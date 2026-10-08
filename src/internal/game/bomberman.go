package game

import (
	"encoding/json"
	"time"
)

// bombermanGame is the Bomberman rules as a Game. It is the realtime, 60Hz case:
// everything in here used to live inside Room, and the behaviour (snapshot
// cadence, crate-layer dedup, lobby keepalive, host-only start) is unchanged —
// it just talks to the Room through the Outbox now instead of reaching into it.
type bombermanGame struct {
	out   Outbox
	match *Match
	round uint64

	// lastCrates is the crate layer the clients have already been sent. The layer
	// rides along only on the ticks where it changed; a mid-round joiner is handed
	// the full layer directly in Joined.
	lastCrates string
}

func newBombermanGame(out Outbox, seed uint64) Game {
	return &bombermanGame{out: out, match: NewMatch(seed)}
}

func (g *bombermanGame) AddPlayer(name, meta string) (int, bool) {
	return g.match.AddPlayer(name, meta) // meta is the totem slug
}

func (g *bombermanGame) RemovePlayer(seat int) {
	g.match.RemovePlayer(seat)
	// Losing a player can settle a round (last one standing); the Room broadcasts
	// the roster right after this call.
	if g.match.Phase == PhasePlay {
		g.match.checkRoundEnd()
	}
}

func (g *bombermanGame) Roster() []RosterEntry { return g.match.Roster() }

func (g *bombermanGame) WelcomeExtra() WelcomeExtra {
	return WelcomeExtra{Fuse: fuseTicks, Arena: g.match.Grid.ArenaDTO()}
}

func (g *bombermanGame) TickHz() int { return TickHz }

// Joined hands this seat one full snapshot, crate layer included: it has seen
// nothing, so it needs the complete state (in the lobby too, so it has a board
// to draw behind the "waiting" overlay rather than an empty box).
func (g *bombermanGame) Joined(seat int) {
	g.out.Send(seat, g.match.Snapshot())
}

func (g *bombermanGame) Command(seat int, isHost bool, raw []byte) {
	var msg ClientMsg
	if err := json.Unmarshal(raw, &msg); err != nil {
		return
	}
	switch msg.T {
	case MsgInput:
		g.match.QueueInput(seat, msg.Seq, Input{DX: msg.DX, DY: msg.DY, Bomb: msg.Bomb})
	case MsgStart, MsgRestart:
		if !isHost { // the room enforces that only the host may start
			return
		}
		if g.match.Phase == PhasePlay || (g.match.Phase == PhaseOver && !g.match.CanRestart()) {
			return
		}
		g.round++
		g.match.Start(g.round)
		g.lastCrates = "" // fresh grid: make the next snapshot carry it
		g.out.BroadcastRoster()
		g.broadcastSnapshot()
	}
}

func (g *bombermanGame) Dropped(seat int) {
	// The player stays in the match, standing still, until their grace lapses.
	if p, ok := g.match.Players[seat]; ok {
		p.in = Input{}
		p.dropPending()
	}
}

func (g *bombermanGame) ToLobby() { g.match.Phase = PhaseLobby }

func (g *bombermanGame) Tick(_ time.Time) {
	// The lobby has nothing in motion, so it ticks over at a crawl rather than
	// pushing a frame every tick. It is not silent though: a dead socket fires no
	// event, so clients need a slow pulse to miss.
	if g.match.Phase == PhaseLobby {
		if g.match.Tick++; g.match.Tick%lobbyKeepaliveTicks != 0 {
			return
		}
		g.broadcastSnapshot()
		return
	}
	was := g.match.Phase
	g.match.Step()
	g.broadcastSnapshot()
	if was == PhasePlay && g.match.Phase == PhaseOver {
		g.out.BroadcastRoster() // the winner's tally changed
	}
}

// broadcastSnapshot sends the per-tick state, omitting the crate layer when it
// has not changed: delivery is ordered, so clients hold the last value they saw.
func (g *bombermanGame) broadcastSnapshot() {
	s := g.match.Snapshot()
	if s.C == g.lastCrates {
		s.C = ""
	} else {
		g.lastCrates = s.C
	}
	g.out.Broadcast(s)
}
