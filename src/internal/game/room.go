package game

import (
	"encoding/json"
	"errors"
	"sync"
	"time"
)

// sendQueue is how many snapshots may back up for one client before the room
// starts dropping them. Snapshots supersede each other, so a short queue is
// better than a long one: it bounds how stale a slow client's view can get.
const sendQueue = 8

var (
	// ErrRoomFull is returned when all MaxPlayers slots are taken.
	ErrRoomFull = errors.New("room is full")
	// ErrRoomClosed is returned once a room has been reaped.
	ErrRoomClosed = errors.New("room is closed")
)

// Conn is the room's handle on one connected client. The transport reads
// frames to send from Out and pushes client intent back through the Room.
type Conn struct {
	slot int
	send chan []byte
}

// Slot is the player slot this connection controls.
func (c *Conn) Slot() int { return c.slot }

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

	acts      chan func()
	quit      chan struct{}
	closeOnce sync.Once
}

func newRoom(code string, seed uint64) *Room {
	r := &Room{
		code:    code,
		match:   NewMatch(seed),
		conns:   map[int]*Conn{},
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
		case <-tick.C:
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
	// The lobby has nothing in motion; its state travels in roster frames, so
	// idle rooms stay silent instead of pushing 30 snapshots a second.
	if r.match.Phase == PhaseLobby {
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

func (r *Room) rosterMsg() RosterMsg {
	return RosterMsg{T: MsgRoster, Host: r.host, Roster: r.match.Roster()}
}

// Join seats a player and returns their connection plus the welcome frame.
func (r *Room) Join(name, animal string) (*Conn, WelcomeMsg, error) {
	var (
		conn *Conn
		wm   WelcomeMsg
		err  error
	)
	if !r.do(func() {
		slot, ok := r.match.AddPlayer(name, animal)
		if !ok {
			err = ErrRoomFull
			return
		}
		conn = &Conn{slot: slot, send: make(chan []byte, sendQueue)}
		r.conns[slot] = conn
		r.emptyAt = time.Time{}
		if r.host < 0 {
			r.host = slot
		}
		wm = WelcomeMsg{
			T: MsgWelcome, You: slot, Code: r.code, Host: r.host, Hz: TickHz,
			Arena: r.match.Grid.ArenaDTO(), Roster: r.match.Roster(),
		}
		r.broadcast(r.rosterMsg())
		if r.match.Phase != PhaseLobby {
			// Mid-round joiner has seen nothing, so they get the complete
			// state — crate layer included — addressed only to them.
			r.sendTo(conn, r.match.Snapshot())
		}
	}) {
		return nil, WelcomeMsg{}, ErrRoomClosed
	}
	return conn, wm, err
}

// Leave frees a slot and reassigns the host if it was theirs.
func (r *Room) Leave(slot int) {
	r.do(func() {
		if c, ok := r.conns[slot]; ok {
			close(c.send)
			delete(r.conns, slot)
		}
		r.match.RemovePlayer(slot)

		if r.host == slot {
			r.host = -1
			for s := 0; s < MaxPlayers; s++ {
				if _, ok := r.conns[s]; ok {
					r.host = s
					break
				}
			}
		}
		if len(r.conns) == 0 {
			r.emptyAt = time.Now()
			// Nobody left to play it out; park the match back in the lobby so a
			// rejoin doesn't resume a round with no players.
			r.match.Phase = PhaseLobby
			return
		}
		r.broadcast(r.rosterMsg())
		// A departure can leave one player standing, which ends the round.
		if r.match.Phase == PhasePlay {
			r.match.checkRoundEnd()
		}
	})
}

// Input records a player's intent for the next tick.
func (r *Room) Input(slot int, in Input) {
	// Fire-and-forget: a dropped input costs one tick of movement, where
	// blocking the connection goroutine would cost much more.
	select {
	case r.acts <- func() { r.match.SetInput(slot, in) }:
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

// close stops the room goroutine, which hangs up on every client as it exits.
func (r *Room) close() {
	r.closeOnce.Do(func() { close(r.quit) })
}
