package game

import "time"

// A game room is split in two. The Room (room.go) is game-agnostic: it owns the
// code, the seats, the reconnect tokens, the per-client send queues and the one
// goroutine that serialises all state. A Game is the rules for one kind of game.
// The Room drives the Game and hands it an Outbox to emit frames through.
//
// Because every Game method runs on the Room's single goroutine, a Game needs no
// locks. Bomberman is the first Game (bomberman.go); the turn-based games slot in
// beside it. See docs/games-architecture.md.

// Outbox lets a Game send frames. The Room implements it; the sockets and the
// set of connected seats belong to the Room. Called only from Game methods,
// which the Room runs on its own goroutine, so touching Room state here is safe.
type Outbox interface {
	Send(seat int, v any) // to one seat — the basis of per-seat PRIVATE views
	Broadcast(v any)      // to every connected seat
	BroadcastRoster()     // re-send the roster (Room folds in host + "gone" flags)
	Seats() []int         // connected seats, ascending
	Host() int            // current host seat, or -1
}

// WelcomeExtra is the game-specific payload merged into the welcome frame. Turn
// -based games leave it zero; Bomberman fills the arena and bomb fuse.
type WelcomeExtra struct {
	Fuse  int
	Arena ArenaDTO
}

// Game is the rules of one room. One instance per room, built by a gameFactory.
type Game interface {
	// AddPlayer seats a new player; meta is a free per-game string (Bomberman:
	// the totem slug). ok is false when the game is full.
	AddPlayer(name, meta string) (seat int, ok bool)
	// RemovePlayer drops a player for good (their held seat's grace lapsed).
	RemovePlayer(seat int)
	// Roster is the slow-changing identity list; the Room adds host + gone.
	Roster() []RosterEntry

	// WelcomeExtra is merged into this seat's welcome frame.
	WelcomeExtra() WelcomeExtra
	// Joined pushes a freshly-seated seat its initial view (via the Outbox).
	Joined(seat int)

	// Command applies one client frame. isHost says whether the seat currently
	// hosts (host-only actions like "start" check it). The game re-parses raw.
	Command(seat int, isHost bool, raw []byte)
	// Dropped is called when a seat's socket drops but the seat is being held
	// (Bomberman stops that player moving; turn games may ignore it).
	Dropped(seat int)
	// ToLobby resets to the waiting state once nobody is left to come back.
	ToLobby()

	// TickHz is the simulation rate: 0 for an event-driven game (no clock, the
	// Room only pushes on change), or >0 for a realtime one (Bomberman: 60).
	TickHz() int
	// Tick advances a realtime game by one tick; never called when TickHz()==0.
	Tick(now time.Time)
}

// gameFactory builds a Game bound to its Room's Outbox. seed makes any
// randomness deterministic for tests.
type gameFactory func(out Outbox, seed uint64) Game

// gameFactories maps the ?game= slug to its implementation. New games register
// here; the slug is what POST /api/v1/games/rooms selects.
var gameFactories = map[string]gameFactory{
	"bomberman": newBombermanGame,
	"codenames": newCodenamesGame,
	"the-mind":  newTheMindGame,
	"the-game":  newTheGameGame,
}
