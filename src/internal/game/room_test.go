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
	r, err := reg.Create()
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	return r
}

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

	r.Leave(c0.Slot())

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
	r.do(func() { r.match.Players[c0.Slot()].Wins = 3 })

	r.Leave(c0.Slot())

	conn, w1, err := r.Join("aap", "vos", w0.Token)
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	if conn.Slot() != c0.Slot() {
		t.Fatalf("resumed into slot %d, want the original %d", conn.Slot(), c0.Slot())
	}
	var wins int
	r.do(func() { wins = r.match.Players[conn.Slot()].Wins })
	if wins != 3 {
		t.Fatalf("win tally = %d after reconnect, want 3", wins)
	}
	if w1.Token == w0.Token {
		t.Fatal("the resume token was reused; each seating should mint a fresh one")
	}
}

func TestStaleOrWrongTokenDoesNotStealASeat(t *testing.T) {
	r := mustCreate(t, testRegistry(t))
	c0, w0 := mustJoin(t, r, "aap")

	// While the original holder is still connected, their token must not seat
	// anyone else into that slot.
	conn, _, err := r.Join("imposter", "vos", w0.Token)
	if err != nil {
		t.Fatalf("join: %v", err)
	}
	if conn.Slot() == c0.Slot() {
		t.Fatal("a live player's seat was taken by replaying their token")
	}

	// And a garbage token is simply a normal join.
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
	r.Begin(c1.Slot())
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

	r.Begin(c0.Slot())
	state := recvTyped(t, c0, MsgState)
	if state["ph"] != string(PhasePlay) {
		t.Fatalf("phase = %v after the host started, want %q", state["ph"], PhasePlay)
	}
	if state["c"] == nil {
		t.Fatal("first snapshot of a round omitted the crate layer")
	}
}

func TestRoomTicksAndAppliesInput(t *testing.T) {
	r := mustCreate(t, testRegistry(t))
	c0, _ := mustJoin(t, r, "aap")
	r.Begin(c0.Slot())

	first := recvTyped(t, c0, MsgState)
	startX := playerX(t, first, 0)

	// Hold right for a while; the room's own tick loop should move the player.
	deadline := time.After(frameWait)
	for {
		r.Input(c0.Slot(), pushSeq(), Input{DX: 1})
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
	r.Begin(c0.Slot())

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
	r.Begin(c0.Slot())

	if first := recvTyped(t, c0, MsgState); first["c"] == nil {
		t.Fatal("first snapshot of a round omitted the crate layer")
	}
	for i := 0; i < 3; i++ {
		if f := recvTyped(t, c0, MsgState); f["c"] != nil {
			t.Fatal("unchanged crate layer was resent on a later tick")
		}
	}
}

func TestLobbyDoesNotStreamSnapshots(t *testing.T) {
	r := mustCreate(t, testRegistry(t))
	c0, _ := mustJoin(t, r, "aap")

	// Drain the join roster frame, then confirm the lobby stays quiet rather
	// than pushing 30 snapshots a second at an idle room.
	recvTyped(t, c0, MsgRoster)
	select {
	case b := <-c0.Out():
		t.Fatalf("lobby emitted an unexpected frame: %s", b)
	case <-time.After(300 * time.Millisecond):
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
