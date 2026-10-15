package game

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

const frameWait = 2 * time.Second

// pushSeq mimics a client's monotonically rising input counter.
var roomSeq uint32

func pushSeq() uint32 { roomSeq++; return roomSeq }

func testRegistry(t *testing.T) *Registry {
	t.Helper()
	reg := NewRegistry()
	t.Cleanup(reg.Close)
	return reg
}

func mustCreate(t *testing.T, reg *Registry) *Room {
	t.Helper()
	r, err := reg.Create("bomberman")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	return r
}

// begin and sendInput drive the room through the generic Command path (the old
// r.Begin / r.Input helpers are gone now that Room is game-agnostic).
func begin(r *Room, c *Conn) { r.Command(c, []byte(`{"t":"`+MsgStart+`"}`)) }

func sendInput(r *Room, c *Conn, in Input) {
	b, _ := json.Marshal(ClientMsg{T: MsgInput, DX: in.DX, DY: in.DY, Bomb: in.Bomb, Seq: pushSeq()})
	r.Command(c, b)
}

// bomb returns the room's Bomberman match for tests that poke at player state.
func bomb(r *Room) *Match { return r.game.(*bombermanGame).match }

func mustJoin(t *testing.T, r *Room, name string) (*Conn, WelcomeMsg) {
	t.Helper()
	c, wm, err := r.Join(name, "vos", "")
	if err != nil {
		t.Fatalf("Join(%s): %v", name, err)
	}
	return c, wm
}

// recvTyped drains frames until one of the wanted type arrives. Rooms also emit
// roster frames, so a test waiting for a snapshot has to skip past them.
func recvTyped(t *testing.T, c *Conn, want string) map[string]any {
	t.Helper()
	deadline := time.After(frameWait)
	for {
		select {
		case b, ok := <-c.Out():
			if !ok {
				t.Fatalf("connection closed while waiting for %q", want)
			}
			var m map[string]any
			if err := json.Unmarshal(b, &m); err != nil {
				t.Fatalf("bad frame %q: %v", b, err)
			}
			if m["t"] == want {
				return m
			}
		case <-deadline:
			t.Fatalf("timed out waiting for a %q frame", want)
		}
	}
}

// recvPlaying drains frames until a snapshot from a running round arrives.
// Joining now delivers a lobby snapshot first, which these tests must skip past.
func recvPlaying(t *testing.T, c *Conn) map[string]any {
	t.Helper()
	deadline := time.After(frameWait)
	for {
		frame := recvTyped(t, c, MsgState)
		if frame["ph"] == string(PhasePlay) {
			return frame
		}
		select {
		case <-deadline:
			t.Fatal("no snapshot from a running round arrived")
		default:
		}
	}
}

func TestRoomCodesAreWellFormedAndUnique(t *testing.T) {
	reg := testRegistry(t)

	seen := map[string]bool{}
	for i := 0; i < 40; i++ {
		code := mustCreate(t, reg).Code()
		if len(code) != CodeLen {
			t.Fatalf("code %q has length %d, want %d", code, len(code), CodeLen)
		}
		for _, ch := range code {
			if !strings.ContainsRune(codeAlphabet, ch) {
				t.Fatalf("code %q contains %q, which is outside the alphabet", code, ch)
			}
		}
		if seen[code] {
			t.Fatalf("duplicate room code %q", code)
		}
		seen[code] = true
	}
}

func TestRegistryGet(t *testing.T) {
	reg := testRegistry(t)
	r := mustCreate(t, reg)

	got, ok := reg.Get(r.Code())
	if !ok || got != r {
		t.Fatal("Get did not return the created room")
	}
	if _, ok := reg.Get("ZZZZ"); ok {
		t.Fatal("Get returned a room for a code that was never created")
	}
	// Codes are read aloud, so lowercase typing has to resolve.
	if _, ok := reg.Get(NormalizeCode(strings.ToLower(r.Code()))); !ok {
		t.Fatal("NormalizeCode did not round-trip a lowercased code")
	}
}

func TestJoinSeatsPlayersAndNamesAHost(t *testing.T) {
	r := mustCreate(t, testRegistry(t))

	c0, w0 := mustJoin(t, r, "aap")
	if w0.You != 0 || w0.Host != 0 {
		t.Fatalf("first joiner: You=%d Host=%d, want 0 and 0", w0.You, w0.Host)
	}
	if w0.Game != "bomberman" {
		t.Fatalf("welcome should carry the room's game slug, got %q", w0.Game)
	}
	if w0.Arena.W != ArenaW || len(w0.Arena.Solid) != ArenaW*ArenaH {
		t.Fatalf("welcome carried a malformed arena: %+v", w0.Arena)
	}
	if w0.Hz != TickHz {
		t.Fatalf("welcome Hz = %d, want %d", w0.Hz, TickHz)
	}

	_, w1 := mustJoin(t, r, "beer")
	if w1.You != 1 {
		t.Fatalf("second joiner got slot %d, want 1", w1.You)
	}
	if w1.Host != 0 {
		t.Fatalf("second joiner sees host %d, want 0", w1.Host)
	}

	// The first player is told about the second. c0 also received a roster for
	// its own join, so wait for the one that reflects both.
	deadline := time.After(frameWait)
	for {
		roster := recvTyped(t, c0, MsgRoster)
		if len(roster["roster"].([]any)) == 2 {
			return
		}
		select {
		case <-deadline:
			t.Fatal("never received a roster listing both players")
		default:
		}
	}
}

func TestJoinRefusedWhenFull(t *testing.T) {
	r := mustCreate(t, testRegistry(t))
	for i := 0; i < MaxPlayers; i++ {
		mustJoin(t, r, "p")
	}
	if _, _, err := r.Join("late", "vos", ""); err != ErrRoomFull {
		t.Fatalf("Join on a full room: err = %v, want %v", err, ErrRoomFull)
	}
}

func TestLeaveReassignsHostAndHoldsTheSeat(t *testing.T) {
	r := mustCreate(t, testRegistry(t))
	c0, _ := mustJoin(t, r, "aap")
	_, _ = mustJoin(t, r, "beer")

	r.Leave(c0)

	// The remaining player takes over as host, and the vacated seat is NOT
	// handed to a stranger: it is being kept warm for whoever just dropped.
	_, w := mustJoin(t, r, "cavia")
	if w.Host != 1 {
		t.Fatalf("host = %d after the host left, want 1", w.Host)
	}
	if w.You == c0.Slot() {
		t.Fatalf("a new player was given slot %d, which is being held", w.You)
	}
}

func TestDroppedPlayerReclaimsTheirSeat(t *testing.T) {
	r := mustCreate(t, testRegistry(t))
	c0, w0 := mustJoin(t, r, "aap")
	if w0.Token == "" {
		t.Fatal("welcome carried no resume token")
	}
	// Win tallies and totem live on the Player, so they must survive the drop.
	r.do(func() { bomb(r).Players[c0.Slot()].Wins = 3 })

	r.Leave(c0)

	conn, w1, err := r.Join("aap", "vos", w0.Token)
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	if conn.Slot() != c0.Slot() {
		t.Fatalf("resumed into slot %d, want the original %d", conn.Slot(), c0.Slot())
	}
	var wins int
	r.do(func() { wins = bomb(r).Players[conn.Slot()].Wins })
	if wins != 3 {
		t.Fatalf("win tally = %d after reconnect, want 3", wins)
	}
	if w1.Token == w0.Token {
		t.Fatal("the resume token was reused; each seating should mint a fresh one")
	}
}

func TestWrongTokenDoesNotStealASeat(t *testing.T) {
	r := mustCreate(t, testRegistry(t))
	c0, _ := mustJoin(t, r, "aap")

	// A garbage token is simply a normal join into a fresh seat; it never resumes
	// someone else's. (A CORRECT token DOES reclaim its slot even while that seat is
	// still live — that is the reconnect-supersede path, covered by
	// TestReconnectSupersedesTheStaleConnection. The token is the secret that
	// distinguishes the two.)
	other, _, err := r.Join("stranger", "vos", "not-a-real-token")
	if err != nil {
		t.Fatalf("join: %v", err)
	}
	if other.Slot() == c0.Slot() {
		t.Fatal("an invalid token resumed someone else's seat")
	}
}

func TestOnlyHostCanStartTheRound(t *testing.T) {
	r := mustCreate(t, testRegistry(t))
	c0, _ := mustJoin(t, r, "aap")
	c1, _ := mustJoin(t, r, "beer")

	// A non-host pressing start must not begin the match.
	begin(r, c1)
	select {
	case b, ok := <-c1.Out():
		var m map[string]any
		if ok {
			_ = json.Unmarshal(b, &m)
			if m["t"] == MsgState {
				t.Fatal("a non-host started the round")
			}
		}
	case <-time.After(150 * time.Millisecond):
		// Nothing came, which is the expected outcome.
	}

	begin(r, c0)
	state := recvPlaying(t, c0)
	if state["c"] == nil {
		t.Fatal("first snapshot of a round omitted the crate layer")
	}
}

func TestRoomTicksAndAppliesInput(t *testing.T) {
	r := mustCreate(t, testRegistry(t))
	c0, _ := mustJoin(t, r, "aap")
	begin(r, c0)

	first := recvTyped(t, c0, MsgState)
	startX := playerX(t, first, 0)

	// Hold right for a while; the room's own tick loop should move the player.
	deadline := time.After(frameWait)
	for {
		sendInput(r, c0, Input{DX: 1})
		select {
		case <-deadline:
			t.Fatal("player never moved under held input")
		case <-time.After(20 * time.Millisecond):
		}
		if playerX(t, recvTyped(t, c0, MsgState), 0) > startX+0.2 {
			return
		}
	}
}

// The crate layer rides along only on the ticks where it changed, so a player
// who joins mid-round has never seen one. Their first snapshot has to carry the
// full layer, or they render an arena with no crates in it at all.
func TestMidRoundJoinerGetsTheCrateLayer(t *testing.T) {
	r := mustCreate(t, testRegistry(t))
	c0, _ := mustJoin(t, r, "aap")
	begin(r, c0)

	// Let the round settle past the first snapshot, so the layer counts as
	// "unchanged" by the time the second player arrives.
	for i := 0; i < 3; i++ {
		recvTyped(t, c0, MsgState)
	}

	c1, _ := mustJoin(t, r, "beer")
	if frame := recvTyped(t, c1, MsgState); frame["c"] == nil {
		t.Fatal("mid-round joiner's first snapshot carried no crate layer")
	}
}

// The flip side of the joiner case: an established client must not be sent the
// crate layer 30 times a second when nothing has blown up.
func TestTickSnapshotsOmitUnchangedCrateLayer(t *testing.T) {
	r := mustCreate(t, testRegistry(t))
	c0, _ := mustJoin(t, r, "aap")
	begin(r, c0)

	if first := recvPlaying(t, c0); first["c"] == nil {
		t.Fatal("first snapshot of a round omitted the crate layer")
	}
	for i := 0; i < 3; i++ {
		if f := recvTyped(t, c0, MsgState); f["c"] != nil {
			t.Fatal("unchanged crate layer was resent on a later tick")
		}
	}
}

func TestLobbyTicksOverSlowlyRatherThanStreaming(t *testing.T) {
	r := mustCreate(t, testRegistry(t))
	c0, _ := mustJoin(t, r, "aap")
	recvTyped(t, c0, MsgRoster)
	recvTyped(t, c0, MsgState) // the join frame

	// The lobby keeps a slow pulse so a client can tell a quiet connection from
	// a dead one, but it must not push a frame every tick at an idle room.
	time.Sleep(1200 * time.Millisecond)
	n := len(c0.Out())
	if n == 0 {
		t.Fatal("lobby went completely silent; a dead link would be undetectable")
	}
	if n > 4 {
		t.Fatalf("lobby emitted %d frames in 1.2s, want a slow pulse not a stream", n)
	}
}

func TestLobbyJoinCarriesTheBoard(t *testing.T) {
	r := mustCreate(t, testRegistry(t))
	c0, _ := mustJoin(t, r, "aap")

	// Without this frame the client has no crate layer and draws an empty box
	// behind the "waiting for players" overlay.
	frame := recvTyped(t, c0, MsgState)
	if frame["c"] == nil {
		t.Fatal("lobby snapshot carried no crate layer")
	}
	if players, _ := frame["p"].([]any); len(players) != 1 {
		t.Fatalf("lobby snapshot listed %d players, want 1", len(players))
	}
}

func TestRegistryCloseHangsUpClients(t *testing.T) {
	reg := NewRegistry()
	r := mustCreate(t, reg)
	c0, _ := mustJoin(t, r, "aap")

	reg.Close()

	deadline := time.After(frameWait)
	for {
		select {
		case _, ok := <-c0.Out():
			if !ok {
				return // channel closed, which is the hang-up
			}
		case <-deadline:
			t.Fatal("client was not hung up after the registry closed")
		}
	}
}

func TestJoinAfterCloseIsRefused(t *testing.T) {
	reg := NewRegistry()
	r := mustCreate(t, reg)
	reg.Close()

	if _, _, err := r.Join("aap", "vos", ""); err != ErrRoomClosed {
		t.Fatalf("Join on a closed room: err = %v, want %v", err, ErrRoomClosed)
	}
}

// playerX digs a player's X out of a decoded snapshot frame.
func playerX(t *testing.T, frame map[string]any, slot int) float64 {
	t.Helper()
	players, ok := frame["p"].([]any)
	if !ok {
		t.Fatalf("snapshot has no players: %v", frame)
	}
	for _, raw := range players {
		p := raw.(map[string]any)
		if int(p["s"].(float64)) == slot {
			return p["x"].(float64)
		}
	}
	t.Fatalf("slot %d missing from snapshot", slot)
	return 0
}

// drainClosed reads a connection's queue until it is closed, failing if it stays open.
// A superseded connection's send channel is closed by the room, which is how its pumps
// unwind and it stops controlling the seat.
func drainClosed(t *testing.T, c *Conn) {
	t.Helper()
	deadline := time.After(frameWait)
	for {
		select {
		case _, ok := <-c.Out():
			if !ok {
				return
			}
		case <-deadline:
			t.Fatal("the superseded connection's queue was never closed")
		}
	}
}

func TestReconnectSupersedesTheStaleConnection(t *testing.T) {
	// The reconnect bug: a client that reconnects before the server has noticed its old
	// socket die (macOS App Nap, a backgrounded tab, a lid-sleep) must reclaim its SAME
	// seat and hero, not be re-seated as a brand-new player (whose crawler hero would
	// spawn at the start tile: the "teleport halfway the map"). The correct resume token
	// reclaims the live seat and supersedes the stale connection.
	r := mustCreate(t, testRegistry(t))
	c0, w0 := mustJoin(t, r, "aap")
	r.do(func() { bomb(r).Players[c0.Slot()].Wins = 2 }) // seat state must survive

	c1, w1, err := r.Join("aap", "vos", w0.Token) // reconnect while c0 is still live
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	if c1.Slot() != c0.Slot() {
		t.Fatalf("reconnect landed in slot %d, want the original %d (a new seat is the teleport bug)", c1.Slot(), c0.Slot())
	}
	if w1.Token == w0.Token {
		t.Fatal("a reseat should mint a fresh resume token")
	}
	drainClosed(t, c0) // the stale connection is superseded (queue closed)

	var wins int
	r.do(func() { wins = bomb(r).Players[c1.Slot()].Wins })
	if wins != 2 {
		t.Fatalf("win tally = %d after supersede, want 2 (the same hero was kept)", wins)
	}
}

func TestSupersededConnectionCannotCommand(t *testing.T) {
	// A superseded connection must not mutate the game: a late/in-flight command from the
	// old socket (e.g. a move or portal that was in flight when it dropped) is ignored.
	r := mustCreate(t, testRegistry(t))
	c0, w0 := mustJoin(t, r, "aap") // host, slot 0
	c1, _, err := r.Join("aap", "vos", w0.Token)
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	drainClosed(t, c0)

	begin(r, c0)    // the superseded (old) host connection presses Start: must be ignored
	r.do(func() {}) // flush: a prior act would have run by the time this returns
	var ph Phase
	r.do(func() { ph = bomb(r).Phase })
	if ph == PhasePlay {
		t.Fatal("a superseded connection started the round")
	}

	begin(r, c1) // the live connection drives the game as normal
	r.do(func() {})
	r.do(func() { ph = bomb(r).Phase })
	if ph != PhasePlay {
		t.Fatalf("the live host could not start the round; phase %v", ph)
	}
}

func TestStaleTeardownDoesNotEvictNewConnection(t *testing.T) {
	// When the old socket finally tears down, its Leave/Quit must not touch the seat the
	// reconnect has since taken over, or it would kick the client that just reconnected.
	r := mustCreate(t, testRegistry(t))
	c0, w0 := mustJoin(t, r, "aap")
	c1, _, err := r.Join("aap", "vos", w0.Token)
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	drainClosed(t, c0)

	r.Leave(c0) // the stale connection's delayed teardown

	var cur *Conn
	r.do(func() { cur = r.conns[c1.Slot()] })
	if cur != c1 {
		t.Fatal("the stale teardown evicted the new connection from the seat")
	}
	// And the new connection is still live: a roster broadcast reaches it.
	r.BroadcastRoster()
	recvTyped(t, c1, MsgRoster)
}
