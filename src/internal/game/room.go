package game

import (
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/Niklasvdm/TotemAppIT/internal/buildinfo"
)

// sendQueue is how many snapshots may back up for one client before the room
// starts dropping them. Snapshots supersede each other, so a short queue is
// better than a long one: it bounds how stale a slow client's view can get.
const sendQueue = 8

// reconnectGrace is how long a seat is held for a player whose socket dropped.
// A VPN hiccup or a sleeping laptop should cost a few seconds of standing
// still, not the match: their slot, name, totem and win tally all wait for them.
const reconnectGrace = 30 * time.Second

var (
	// ErrRoomFull is returned when all MaxPlayers slots are taken.
	ErrRoomFull = errors.New("room is full")
	// ErrRoomClosed is returned once a room has been reaped.
	ErrRoomClosed = errors.New("room is closed")
)

// Conn is the room's handle on one connected client. The transport reads
// frames to send from Out and pushes client intent back through the Room.
type Conn struct {
	slot  int
	token string
	send  chan []byte
}

// Slot is the player slot this connection controls.
func (c *Conn) Slot() int { return c.slot }

// Token is the secret that lets this client reclaim its seat after a drop.
func (c *Conn) Token() string { return c.token }

// Out yields frames to write to the socket.
func (c *Conn) Out() <-chan []byte { return c.send }

// Room owns one Match and serialises every mutation onto a single goroutine,
// so the simulation needs no locks and stays deterministic. All exported
// methods are safe to call from connection goroutines.
type Room struct {
	code string

	match   *Match
	conns   map[int]*Conn
	host    int
	round   uint64
	emptyAt time.Time // zero while someone is connected

	// lastCrates is the crate layer this room's clients have already been sent.
	lastCrates string

	// held are seats kept warm for players whose socket dropped, keyed by slot.
	held map[int]heldSeat

	acts      chan func()
	quit      chan struct{}
	closeOnce sync.Once
}

func newRoom(code string, seed uint64) *Room {
	r := &Room{
		code:    code,
		match:   NewMatch(seed),
		conns:   map[int]*Conn{},
		held:    map[int]heldSeat{},
		host:    -1,
		emptyAt: time.Now(),
		acts:    make(chan func(), 64),
		quit:    make(chan struct{}),
	}
	go r.run()
	return r
}

// Code is the room's join code.
func (r *Room) Code() string { return r.code }

func (r *Room) run() {
	tick := time.NewTicker(time.Second / TickHz)
	defer tick.Stop()

	for {
		select {
		case <-r.quit:
			// Hang up here rather than in close(): conns belongs to this
			// goroutine, and the reaper calling close() runs on another.
			for _, c := range r.conns {
				close(c.send)
			}
			r.conns = nil
			return
		case fn := <-r.acts:
			fn()
		case now := <-tick.C:
			r.reapHeld(now)
			r.step()
		}
	}
}

// do runs fn on the room goroutine and waits for it. It reports false if the
// room closed first.
func (r *Room) do(fn func()) bool {
	done := make(chan struct{})
	select {
	case r.acts <- func() { fn(); close(done) }:
	case <-r.quit:
		return false
	}
	select {
	case <-done:
		return true
	case <-r.quit:
		return false
	}
}

func (r *Room) step() {
	// The lobby has nothing in motion, so it ticks over at a crawl rather than
	// pushing a snapshot every frame at a room where nobody is moving. It is not
	// silent, though: a connection that dies without closing (a VPN dropping, a
	// NAT forgetting its mapping) fires no event at all, and the only evidence
	// left is an absence of traffic — so clients need something to miss.
	if r.match.Phase == PhaseLobby {
		if r.match.Tick++; r.match.Tick%lobbyKeepaliveTicks != 0 {
			return
		}
		r.broadcastSnapshot()
		return
	}
	was := r.match.Phase
	r.match.Step()
	r.broadcastSnapshot()
	if was == PhasePlay && r.match.Phase == PhaseOver {
		r.broadcast(r.rosterMsg()) // the winner's tally changed
	}
}

func (r *Room) broadcast(v any) {
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	for _, c := range r.conns {
		queue(c, b)
	}
}

func (r *Room) sendTo(c *Conn, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	queue(c, b)
}

// queue hands a frame to one client, dropping it if that client is backed up.
// Snapshots supersede each other, so the newest is the only one worth having —
// and the room must never block on a stalled socket.
func queue(c *Conn, b []byte) {
	select {
	case c.send <- b:
	default:
	}
}

// broadcastSnapshot sends the per-tick state, omitting the crate layer when it
// has not changed since the last one: delivery is ordered, so clients hold the
// last value they saw. This lives here rather than in Match because only the
// Room knows what its clients have already been sent — which is what lets a
// mid-round joiner be handed the full layer instead of an empty arena.
func (r *Room) broadcastSnapshot() {
	s := r.match.Snapshot()
	if s.C == r.lastCrates {
		s.C = ""
	} else {
		r.lastCrates = s.C
	}
	r.broadcast(s)
}

// resumeSeat matches a token against the seats being held and returns the slot
// it reclaims. Tokens are compared in constant time: the token is the only
// thing standing between a stranger and someone else's seat.
func (r *Room) resumeSeat(token string) (int, bool) {
	if token == "" {
		return 0, false
	}
	now := time.Now()
	for slot, h := range r.held {
		if _, taken := r.conns[slot]; taken {
			continue // that seat is live; this token is stale
		}
		if h.until.IsZero() || now.After(h.until) {
			continue
		}
		if subtle.ConstantTimeCompare([]byte(h.token), []byte(token)) == 1 {
			delete(r.held, slot)
			return slot, true
		}
	}
	return 0, false
}

// heldSeat is a disconnected player's claim on their slot.
type heldSeat struct {
	token string
	until time.Time
}

// reapHeld drops seats whose grace period has run out. Called from the tick
// loop, which is the only place allowed to touch room state.
func (r *Room) reapHeld(now time.Time) {
	for slot, h := range r.held {
		if _, live := r.conns[slot]; live {
			continue // still connected — this claim is for a future drop
		}
		// A zero deadline means the seat was claimed but never dropped.
		if h.until.IsZero() || now.Before(h.until) {
			continue
		}
		delete(r.held, slot)
		r.match.RemovePlayer(slot)
		r.reassignHost()
		r.broadcast(r.rosterMsg())
		if len(r.conns) == 0 && len(r.held) == 0 {
			// The last hope of a reconnect has expired: now the round is over.
			r.match.Phase = PhaseLobby
		} else if r.match.Phase == PhasePlay {
			r.match.checkRoundEnd()
		}
	}
}

func (r *Room) reassignHost() {
	if _, stillHere := r.conns[r.host]; stillHere {
		return
	}
	r.host = -1
	for s := 0; s < MaxPlayers; s++ {
		if _, ok := r.conns[s]; ok {
			r.host = s
			return
		}
	}
}

// roster lists the seated players, marking the ones whose socket has dropped
// and whose seat is merely being held. Without that flag a held seat looks like
// a second, identical player in the list.
func (r *Room) roster() []RosterEntry {
	entries := r.match.Roster()
	for i := range entries {
		_, live := r.conns[entries[i].S]
		entries[i].Gone = !live
	}
	return entries
}

func (r *Room) rosterMsg() RosterMsg {
	return RosterMsg{T: MsgRoster, Host: r.host, Roster: r.roster()}
}

// Join seats a player and returns their connection plus the welcome frame.
// A resume token from a previous Join reclaims that same seat — slot, totem and
// win tally included — provided its grace period has not expired.
func (r *Room) Join(name, animal, resume string) (*Conn, WelcomeMsg, error) {
	var (
		conn *Conn
		wm   WelcomeMsg
		err  error
	)
	if !r.do(func() {
		slot, ok := r.resumeSeat(resume)
		if !ok {
			if slot, ok = r.match.AddPlayer(name, animal); !ok {
				err = ErrRoomFull
				return
			}
		}
		conn = &Conn{slot: slot, token: newToken(), send: make(chan []byte, sendQueue)}
		r.conns[slot] = conn
		r.held[slot] = heldSeat{token: conn.token} // claim for the NEXT drop
		r.emptyAt = time.Time{}
		if r.host < 0 {
			r.host = slot
		}
		wm = WelcomeMsg{
			T: MsgWelcome, You: slot, Code: r.code, Host: r.host, Hz: TickHz, Fuse: fuseTicks,
			Build: buildinfo.String(), Token: conn.token,
			Arena: r.match.Grid.ArenaDTO(), Roster: r.roster(),
		}
		r.broadcast(r.rosterMsg())
		// One snapshot, addressed to this client alone: it has seen nothing, so
		// it needs the complete state including the crate layer. This happens in
		// the lobby too — the lobby streams nothing, and without a first frame
		// the client has no board to draw and shows an empty box while waiting.
		r.sendTo(conn, r.match.Snapshot())
	}) {
		return nil, WelcomeMsg{}, ErrRoomClosed
	}
	return conn, wm, err
}

// Quit removes a player who is leaving deliberately, releasing their seat
// immediately. A dropped socket goes through Leave instead, which keeps the
// seat warm — the difference is whether the client said goodbye.
func (r *Room) Quit(slot int) {
	r.do(func() { delete(r.held, slot) })
	r.Leave(slot)
}

// Leave disconnects a player, holding their seat for a grace period so they can
// reconnect into it, and reassigns the host if it was theirs.
func (r *Room) Leave(slot int) {
	r.do(func() {
		if c, ok := r.conns[slot]; ok {
			close(c.send)
			delete(r.conns, slot)
		}
		// Hold the seat rather than deleting the player: a dropped socket is
		// usually a blip, and losing your totem and score to one is miserable.
		// The player stays in the match, standing still (their input queue runs
		// dry, so takeInput zeroes it), until the grace period lapses.
		if h, ok := r.held[slot]; ok {
			h.until = time.Now().Add(reconnectGrace)
			r.held[slot] = h
			if p, seated := r.match.Players[slot]; seated {
				p.in = Input{}
				p.dropPending()
			}
		} else {
			r.match.RemovePlayer(slot)
		}

		r.reassignHost()
		if len(r.conns) == 0 {
			r.emptyAt = time.Now()
			// Park the match only once nobody is coming back. While a seat is
			// still being held, the round belongs to whoever dropped out of it:
			// resetting to the lobby would hand them back a dead game, which is
			// a strange reward for reconnecting inside the grace period.
			if len(r.held) == 0 {
				r.match.Phase = PhaseLobby
			}
			return
		}
		r.broadcast(r.rosterMsg())
	})
}

// Input queues a player's sequenced intent.
func (r *Room) Input(slot int, seq uint32, in Input) {
	// Fire-and-forget: a dropped input costs one tick of movement, where
	// blocking the connection goroutine would cost much more.
	select {
	case r.acts <- func() { r.match.QueueInput(slot, seq, in) }:
	case <-r.quit:
	default:
	}
}

// Begin starts a round. Only the host may do so, and only from the lobby or
// once the scoreboard has been up long enough.
func (r *Room) Begin(slot int) {
	r.do(func() {
		if slot != r.host {
			return
		}
		if r.match.Phase == PhasePlay || (r.match.Phase == PhaseOver && !r.match.CanRestart()) {
			return
		}
		r.round++
		r.match.Start(r.round)
		r.lastCrates = "" // fresh grid: make the next snapshot carry it
		r.broadcast(r.rosterMsg())
		r.broadcastSnapshot()
	})
}

// idleSince reports how long the room has been empty, and whether it is empty
// at all.
func (r *Room) idleSince(now time.Time) (time.Duration, bool) {
	var (
		d     time.Duration
		empty bool
	)
	if !r.do(func() {
		if r.emptyAt.IsZero() {
			return
		}
		empty, d = true, now.Sub(r.emptyAt)
	}) {
		return 0, false
	}
	return d, empty
}

// newToken mints a seat-resume secret. It must be unguessable: presenting it is
// what proves you are the player whose socket dropped.
func newToken() string {
	b := make([]byte, 16)
	mustRandom(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// close stops the room goroutine, which hangs up on every client as it exits.
func (r *Room) close() {
	r.closeOnce.Do(func() { close(r.quit) })
}
