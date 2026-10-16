package game

import (
	"bytes"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/Niklasvdm/TotemAppIT/internal/buildinfo"
)

// sendQueue is how many frames may back up for one client before the room starts
// dropping them. For a realtime game snapshots supersede each other, so a short
// queue bounds staleness; the room must never block on a slow socket.
const sendQueue = 8

// reconnectGrace is how long a seat is held for a player whose socket dropped. Generous
// (2 minutes) so a phone that dims, locks, or has its tab discarded and reloaded can walk
// back into the same seat; the client restores its session + resume token across the reload.
const reconnectGrace = 2 * time.Minute

var (
	// ErrRoomFull is returned when the game has no free seat.
	ErrRoomFull = errors.New("room is full")
	// ErrRoomClosed is returned once a room has been reaped.
	ErrRoomClosed = errors.New("room is closed")
)

// Conn is the room's handle on one connected client. The transport reads frames
// to send from Out and pushes client intent back through Room.Command.
type Conn struct {
	slot  int
	token string
	send  chan []byte
	// lastView is the last per-seat frame marshalled for this connection, so an
	// identical follow-up is skipped (see sendTo). Only per-seat Sends are deduped;
	// a reconnect gets a fresh Conn (empty lastView) plus a full frame via Joined, so
	// the client never misses state.
	lastView []byte
}

// Slot is the player slot this connection controls.
func (c *Conn) Slot() int { return c.slot }

// Token is the secret that lets this client reclaim its seat after a drop.
func (c *Conn) Token() string { return c.token }

// Out yields frames to write to the socket.
func (c *Conn) Out() <-chan []byte { return c.send }

// heldSeat is a disconnected player's claim on their slot.
type heldSeat struct {
	token string
	until time.Time
}

// Room hosts one Game and serialises every mutation onto a single goroutine, so
// the game needs no locks and stays deterministic. It owns the generic concerns
// (codes, seats, reconnect tokens, send queues, host); the Game owns the rules.
// Room implements Outbox for its Game. All exported methods are safe to call
// from connection goroutines.
type Room struct {
	code string
	slug string // the registered game slug this room was created for (sent in the welcome)
	game Game

	conns   map[int]*Conn
	host    int
	emptyAt time.Time // zero while someone is connected

	// held are seats kept warm for players whose socket dropped, keyed by slot.
	held map[int]heldSeat

	acts      chan func()
	quit      chan struct{}
	closeOnce sync.Once

	// remove asks the registry to delete and close this room. Called when a deliberate
	// leave empties it, so the room goes away at once instead of lingering for the idle
	// timeout. nil in tests that build a Room without a registry.
	remove func()
}

func newRoom(code, gameSlug string, seed uint64, remove func()) (*Room, error) {
	factory, ok := gameFactories[gameSlug]
	if !ok {
		return nil, fmt.Errorf("unknown game %q", gameSlug)
	}
	r := &Room{
		code:    code,
		slug:    gameSlug,
		conns:   map[int]*Conn{},
		held:    map[int]heldSeat{},
		host:    -1,
		emptyAt: time.Now(),
		acts:    make(chan func(), 64),
		quit:    make(chan struct{}),
		remove:  remove,
	}
	r.game = factory(r, seed) // r is the Game's Outbox
	go r.run()
	return r, nil
}

// Code is the room's join code.
func (r *Room) Code() string { return r.code }

func (r *Room) run() {
	// A realtime game drives a clock; an event-driven one (TickHz 0) still wants
	// a slow tick so held seats get reaped.
	hz := r.game.TickHz()
	interval := time.Second
	if hz > 0 {
		interval = time.Second / time.Duration(hz)
	}
	tick := time.NewTicker(interval)
	defer tick.Stop()

	for {
		select {
		case <-r.quit:
			// Hang up here, not in close(): conns belongs to this goroutine, and
			// the reaper calling close() runs on another.
			for _, c := range r.conns {
				close(c.send)
			}
			r.conns = nil
			return
		case fn := <-r.acts:
			r.safely(fn)
		case now := <-tick.C:
			r.safely(func() {
				r.reapHeld(now)
				if hz > 0 {
					r.game.Tick(now)
				}
			})
		}
	}
}

// safely runs fn on the room goroutine, turning a panic in game code into a
// logged recovery instead of taking down the process (and every other room).
func (r *Room) safely(fn func()) {
	defer func() {
		if e := recover(); e != nil {
			slog.Error("room recovered from panic", "event", "panic", "room", r.code, "game", r.slug, "err", e)
		}
	}()
	fn()
}

// do runs fn on the room goroutine and waits for it. It reports false if the
// room closed first. close(done) is deferred so a panic in fn (recovered by
// safely) still unblocks the waiter.
func (r *Room) do(fn func()) bool {
	done := make(chan struct{})
	select {
	case r.acts <- func() { defer close(done); fn() }:
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

// --- Outbox (called by the Game, on the room goroutine) ---------------------

// Send queues v to one seat — the basis of per-seat private views.
func (r *Room) Send(seat int, v any) {
	if c, ok := r.conns[seat]; ok {
		r.sendTo(c, v)
	}
}

// Broadcast queues v to every connected seat.
func (r *Room) Broadcast(v any) { r.broadcast(v) }

// BroadcastRoster re-sends the roster (host + per-seat "gone" flags folded in).
func (r *Room) BroadcastRoster() { r.broadcastRoster() }

// Seats lists the connected seats, ascending.
func (r *Room) Seats() []int {
	out := make([]int, 0, len(r.conns))
	for s := range r.conns {
		out = append(out, s)
	}
	sort.Ints(out)
	return out
}

// Host is the current host seat, or -1.
func (r *Room) Host() int { return r.host }

// --- frame plumbing ---------------------------------------------------------

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
	// Skip a per-seat view identical to the one already sent to this client: the UI
	// renders the latest state, so an unchanged frame carries nothing. This is the
	// safe alternative to a delta protocol (no shared diff state, no reconnect edge
	// cases): correctness is unchanged, only redundant frames are dropped.
	if bytes.Equal(c.lastView, b) {
		return
	}
	// Only record the frame as sent if it actually enqueued: a dropped frame
	// must not advance lastView, or the next identical state would be suppressed
	// as a "duplicate" and the client would sit on stale state until a different
	// frame arrives (turn-based games change state rarely, so that can be a long
	// time). Realtime snapshots supersede each other, so this only matters here.
	if queue(c, b) {
		c.lastView = b
	}
}

// queue hands a frame to one client, dropping it (and returning false) if that
// client's send buffer is backed up.
func queue(c *Conn, b []byte) bool {
	select {
	case c.send <- b:
		return true
	default:
		return false
	}
}

func (r *Room) broadcastRoster() {
	r.broadcast(RosterMsg{T: MsgRoster, Host: r.host, Roster: r.roster()})
}

// roster lists the seated players, marking the ones whose socket has dropped and
// whose seat is merely being held.
func (r *Room) roster() []RosterEntry {
	entries := r.game.Roster()
	for i := range entries {
		_, live := r.conns[entries[i].S]
		entries[i].Gone = !live
	}
	return entries
}

// --- seat / reconnect bookkeeping -------------------------------------------

// resumeSeat matches a token against the held seats and returns the slot it
// reclaims. Tokens are compared in constant time: the token is the only thing
// standing between a stranger and someone else's seat.
func (r *Room) resumeSeat(token string) (int, bool) {
	if token == "" {
		return 0, false
	}
	now := time.Now()
	for slot, h := range r.held {
		// A seat is reclaimable while its grace holds, AND while it is still live:
		// `until` is zero on a connected seat and only set to a deadline on drop, so a
		// live seat is "the same player reconnecting before the server noticed the old
		// socket die". The caller supersedes that stale connection. Only a dropped seat
		// whose grace has run out is off-limits. Matching the token is what authorises
		// the reclaim either way (it is the client's own secret from its last welcome).
		_, live := r.conns[slot]
		if !live && !h.until.IsZero() && now.After(h.until) {
			continue
		}
		if subtle.ConstantTimeCompare([]byte(h.token), []byte(token)) == 1 {
			delete(r.held, slot)
			return slot, true
		}
	}
	return 0, false
}

// reapHeld drops seats whose grace period has run out. Called from the tick loop.
func (r *Room) reapHeld(now time.Time) {
	for slot, h := range r.held {
		if _, live := r.conns[slot]; live {
			continue
		}
		if h.until.IsZero() || now.Before(h.until) {
			continue
		}
		delete(r.held, slot)
		r.game.RemovePlayer(slot)
		vlog.Info("held seat expired, player gone", "event", "drop_expired", "room", r.code, "game", r.slug, "seat", slot)
		r.reassignHost()
		r.broadcastRoster()
		if len(r.conns) == 0 && len(r.held) == 0 {
			r.game.ToLobby() // last hope of a reconnect gone
		}
	}
}

func (r *Room) reassignHost() {
	if _, stillHere := r.conns[r.host]; stillHere {
		return
	}
	// Lowest connected seat becomes host. Scanning r.conns (not a fixed range)
	// keeps this game-agnostic: Codenames seats go past Bomberman's MaxPlayers.
	r.host = -1
	for s := range r.conns {
		if r.host == -1 || s < r.host {
			r.host = s
		}
	}
}

// --- public API (called from connection goroutines) -------------------------

// Join seats a player and returns their connection plus the welcome frame. A
// resume token from a previous Join reclaims that same seat if its grace has not
// expired.
func (r *Room) Join(name, meta, resume string) (*Conn, WelcomeMsg, error) {
	var (
		conn *Conn
		wm   WelcomeMsg
		err  error
	)
	if !r.do(func() {
		slot, ok := r.resumeSeat(resume)
		reconnect := ok
		if !ok {
			if slot, ok = r.game.AddPlayer(name, meta); !ok {
				err = ErrRoomFull
				return
			}
		}
		// If a stale connection still holds this seat (a reconnect that beat the old
		// socket's teardown — common on a Mac where App Nap / a backgrounded tab freezes
		// the socket and the server only learns it died on the next ping ~20s later),
		// supersede it: closing its queue unwinds its write pump, which cancels its
		// context and stops its read pump, so it can no longer apply commands. Its own
		// teardown then no-ops because it is no longer this slot's connection (see Leave).
		// Without this, the reclaim above could not take a live seat and the client would
		// be re-seated as a brand-new player (a fresh hero at the start tile: the "teleport").
		superseded := false
		if old, held := r.conns[slot]; held {
			close(old.send)
			delete(r.conns, slot)
			superseded = true
		}
		conn = &Conn{slot: slot, token: newToken(), send: make(chan []byte, sendQueue)}
		r.conns[slot] = conn
		r.held[slot] = heldSeat{token: conn.token} // claim for the NEXT drop
		r.emptyAt = time.Time{}
		if r.host < 0 {
			r.host = slot
		}
		extra := r.game.WelcomeExtra()
		wm = WelcomeMsg{
			T: MsgWelcome, You: slot, Code: r.code, Game: r.slug, Host: r.host,
			Hz: r.game.TickHz(), Fuse: extra.Fuse, Build: buildinfo.String(),
			Token: conn.token, Arena: extra.Arena, Roster: r.roster(),
		}
		r.broadcastRoster()
		r.game.Joined(slot) // the game pushes this seat its initial view

		// Connection lifecycle: reconnects and supersedes go to the always-on logger so
		// they are visible at the quiet default; a fresh join is level-gated (routine).
		if superseded {
			vlog.Warn("connection superseded", "event", "supersede", "room", r.code, "game", r.slug, "seat", slot)
		}
		if reconnect {
			vlog.Info("player reconnected", "event", "reconnect", "room", r.code, "game", r.slug, "seat", slot, "superseded", superseded)
		} else {
			slog.Debug("player joined", "event", "join", "room", r.code, "game", r.slug, "seat", slot)
		}
	}) {
		return nil, WelcomeMsg{}, ErrRoomClosed
	}
	return conn, wm, err
}

// Quit removes a player who is leaving deliberately, releasing their seat
// immediately. A dropped socket goes through Leave instead, which holds the seat.
// It no-ops for a connection that a reconnect has already superseded, so a stale
// socket's goodbye can never drop the new occupant's held claim.
func (r *Room) Quit(c *Conn) {
	r.do(func() {
		if cur, ok := r.conns[c.slot]; ok && cur != c {
			return
		}
		delete(r.held, c.slot)
	})
	r.Leave(c)
}

// Leave disconnects a player, holding their seat for a grace period so they can
// reconnect into it, and reassigns the host if it was theirs. If a newer connection
// has already taken this seat (we were superseded on a reconnect), this is a no-op:
// our teardown must never close or evict the connection that replaced us.
func (r *Room) Leave(c *Conn) {
	slot := c.slot
	r.do(func() {
		cur, ok := r.conns[slot]
		if ok && cur != c {
			return // superseded: the current connection is someone else's, leave it be
		}
		if ok {
			close(c.send)
			delete(r.conns, slot)
		}
		// Hold the seat rather than dropping the player: a dropped socket is
		// usually a blip, and losing your place to one is miserable.
		if h, ok := r.held[slot]; ok {
			h.until = time.Now().Add(reconnectGrace)
			r.held[slot] = h
			r.game.Dropped(slot)
			vlog.Warn("player dropped, seat held", "event", "drop", "room", r.code, "game", r.slug, "seat", slot, "grace", reconnectGrace.String())
		} else {
			r.game.RemovePlayer(slot)
			vlog.Info("player left", "event", "leave", "room", r.code, "game", r.slug, "seat", slot)
		}

		r.reassignHost()
		if len(r.conns) == 0 {
			r.emptyAt = time.Now()
			// No live and no held seats: the room is empty. Reached via Leave with no held
			// seat only on a deliberate departure (a drop holds the seat), so delete the
			// room now instead of letting it linger for roomIdleTimeout. A drop leaves a
			// held seat, so this does not fire until the seat is reclaimed or expires.
			if len(r.held) == 0 {
				r.game.ToLobby()
				if r.remove != nil {
					r.remove()
				}
			}
			return
		}
		r.broadcastRoster()
	})
}

// Command hands one client frame to the game. Fire-and-forget: a dropped frame
// costs little, where blocking the connection goroutine would cost much more.
// The raw bytes are copied because the game parses them later, on the room
// goroutine, after the transport's read buffer may have been reused.
// maxCommandBytes caps one client frame. Commands are small JSON objects (a few
// fields); anything larger is malformed or hostile, so it is dropped before it reaches
// game code. The WebSocket layer also limits reads; this is the belt-and-braces bound
// at the single trust boundary where every client frame enters the game.
const maxCommandBytes = 8 << 10 // 8 KiB

func (r *Room) Command(c *Conn, raw []byte) {
	if len(raw) > maxCommandBytes {
		return // oversized frame: never hand untrusted bulk to game parsing
	}
	cp := append([]byte(nil), raw...)
	select {
	case r.acts <- func() {
		// Ignore frames from a connection that has been superseded (an old socket that a
		// reconnect took over): it no longer owns the seat, so a late/in-flight command
		// from it must not mutate the live game. Checked on the room goroutine, where
		// r.conns is owned, so the comparison races nothing.
		if r.conns[c.slot] != c {
			return
		}
		r.game.Command(c.slot, c.slot == r.host, cp)
	}:
	case <-r.quit:
	default:
	}
}

// idleSince reports how long the room has been empty, and whether it is empty.
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
