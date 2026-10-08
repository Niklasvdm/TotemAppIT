package game

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

const (
	// CodeLen is the length of a room code.
	CodeLen = 4

	// codeAlphabet omits look-alike glyphs (0/O, 1/I) because these codes get
	// read out loud across a room. 32 symbols divide 256 evenly, so drawing
	// bytes modulo the alphabet introduces no bias.
	codeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

	maxRooms        = 200
	roomIdleTimeout = 2 * time.Minute
	reapInterval    = 30 * time.Second
)

// ErrTooManyRooms is returned when the registry is at capacity.
var ErrTooManyRooms = errors.New("too many open rooms")

// Registry maps room codes to live rooms and reaps the ones nobody is in.
type Registry struct {
	mu    sync.Mutex
	rooms map[string]*Room

	stop     chan struct{}
	stopOnce sync.Once
}

// NewRegistry starts a registry and its reaper. Call Close to shut it down.
func NewRegistry() *Registry {
	reg := &Registry{rooms: map[string]*Room{}, stop: make(chan struct{})}
	go reg.reap()
	return reg
}

// Create opens a room running the named game with a fresh code. It errors if the
// game slug is unknown or the registry is at capacity.
func (reg *Registry) Create(gameSlug string) (*Room, error) {
	reg.mu.Lock()
	defer reg.mu.Unlock()

	if _, ok := gameFactories[gameSlug]; !ok {
		return nil, fmt.Errorf("unknown game %q", gameSlug)
	}
	if len(reg.rooms) >= maxRooms {
		return nil, ErrTooManyRooms
	}
	// Codes are random, not sequential, so a collision just means drawing again.
	for attempt := 0; attempt < 20; attempt++ {
		code := newCode()
		if _, taken := reg.rooms[code]; taken {
			continue
		}
		r, err := newRoom(code, gameSlug, randUint64())
		if err != nil {
			return nil, err
		}
		reg.rooms[code] = r
		return r, nil
	}
	return nil, ErrTooManyRooms
}

// Get looks up a room by code.
func (reg *Registry) Get(code string) (*Room, bool) {
	reg.mu.Lock()
	defer reg.mu.Unlock()
	r, ok := reg.rooms[code]
	return r, ok
}

// Close stops the reaper and every open room.
func (reg *Registry) Close() {
	reg.stopOnce.Do(func() { close(reg.stop) })
	reg.mu.Lock()
	defer reg.mu.Unlock()
	for code, r := range reg.rooms {
		r.close()
		delete(reg.rooms, code)
	}
}

// NormalizeCode canonicalises a user-typed code.
func NormalizeCode(s string) string {
	return strings.ToUpper(strings.TrimSpace(s))
}

func (reg *Registry) reap() {
	t := time.NewTicker(reapInterval)
	defer t.Stop()
	for {
		select {
		case <-reg.stop:
			return
		case now := <-t.C:
			reg.sweep(now)
		}
	}
}

func (reg *Registry) sweep(now time.Time) {
	// Snapshot under the lock, then query rooms without it: idleSince waits on
	// each room's own goroutine, and holding the registry lock across that
	// would let one wedged room stall every join in the process.
	reg.mu.Lock()
	rooms := make(map[string]*Room, len(reg.rooms))
	for code, r := range reg.rooms {
		rooms[code] = r
	}
	reg.mu.Unlock()

	var dead []string
	for code, r := range rooms {
		if idle, empty := r.idleSince(now); empty && idle > roomIdleTimeout {
			r.close()
			dead = append(dead, code)
		}
	}
	if len(dead) == 0 {
		return
	}

	reg.mu.Lock()
	for _, code := range dead {
		delete(reg.rooms, code)
	}
	reg.mu.Unlock()
}

// newCode draws a room code from a cryptographic source: codes are the only
// thing guarding a room, so they must not be guessable or enumerable.
func newCode() string {
	b := make([]byte, CodeLen)
	mustRandom(b)
	out := make([]byte, CodeLen)
	for i, v := range b {
		out[i] = codeAlphabet[int(v)%len(codeAlphabet)]
	}
	return string(out)
}

func randUint64() uint64 {
	var b [8]byte
	mustRandom(b[:])
	return binary.LittleEndian.Uint64(b[:])
}

// mustRandom fills b with cryptographic randomness. crypto/rand.Read is
// documented never to fail, so there is no fallback to get wrong.
func mustRandom(b []byte) {
	if _, err := rand.Read(b); err != nil {
		panic("game: crypto/rand failed: " + err.Error())
	}
}
