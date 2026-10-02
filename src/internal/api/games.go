package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/coder/websocket"

	"github.com/Niklasvdm/TotemAppIT/internal/game"
)

const (
	// maxGameFrame caps a client frame. Client messages are a direction and two
	// flags, so anything larger is malformed or hostile.
	maxGameFrame = 512

	// maxClientMsgRate is the per-second frame budget for one connection.
	// Well-behaved clients send one input per tick; the headroom absorbs bursts
	// after a stall without letting a bad actor spin the room goroutine.
	maxClientMsgRate = game.TickHz * 4

	gameWriteTimeout = 5 * time.Second
)

// createRoom opens a game room and returns its join code.
func (s *Server) createRoom(w http.ResponseWriter, _ *http.Request) {
	room, err := s.games.Create()
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, "no free game rooms right now")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"code": room.Code()})
}

// gameWS upgrades to a WebSocket and pumps one player's frames to and from
// their room for the life of the connection.
func (s *Server) gameWS(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	room, ok := s.games.Get(game.NormalizeCode(q.Get("code")))
	if !ok {
		writeErr(w, http.StatusNotFound, "unknown room code")
		return
	}
	name, ok := cleanName(q.Get("name"))
	if !ok {
		writeErr(w, http.StatusBadRequest, "invalid player name")
		return
	}
	// The animal is a catalog slug the client renders a sprite for; an
	// unrecognised shape is dropped rather than refused.
	animal := q.Get("animal")
	if !slugRe.MatchString(animal) {
		animal = ""
	}

	// A WebSocket long outlives the 10s read/write timeouts http.Server put on
	// this connection, and hijacking does not clear them — so clear them here,
	// before the upgrade, instead of weakening the timeouts for the whole API.
	rc := http.NewResponseController(w)
	if err := rc.SetReadDeadline(time.Time{}); err != nil {
		writeErr(w, http.StatusInternalServerError, "websockets unavailable")
		return
	}
	if err := rc.SetWriteDeadline(time.Time{}); err != nil {
		writeErr(w, http.StatusInternalServerError, "websockets unavailable")
		return
	}

	// AcceptOptions is left empty on purpose: the library's default Origin check
	// (Origin host must equal Host) is the protection against a hostile page
	// driving someone's game session, and nothing here should relax it.
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return // Accept has already written the error response
	}
	defer conn.CloseNow()
	conn.SetReadLimit(maxGameFrame)

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	seat, welcome, err := room.Join(name, animal)
	if err != nil {
		_ = writeWSJSON(ctx, conn, game.ErrorMsg{T: game.MsgError, Err: err.Error()})
		_ = conn.Close(websocket.StatusTryAgainLater, "room unavailable")
		return
	}
	defer room.Leave(seat.Slot())

	if err := writeWSJSON(ctx, conn, welcome); err != nil {
		return
	}

	// One pump per direction. Either one ending cancels ctx, which unblocks the
	// other, so a dead socket never leaves a goroutine parked on a channel.
	go func() {
		defer cancel()
		gameWritePump(ctx, conn, seat)
	}()
	gameReadPump(ctx, conn, room, seat.Slot())
}

// gameWritePump forwards the room's frames to the socket until the room hangs
// up or the socket fails.
func gameWritePump(ctx context.Context, conn *websocket.Conn, seat *game.Conn) {
	for {
		select {
		case <-ctx.Done():
			return
		case b, ok := <-seat.Out():
			if !ok {
				// The room closed; tell the client so it can stop reconnecting.
				_ = conn.Close(websocket.StatusGoingAway, "room closed")
				return
			}
			wctx, cancel := context.WithTimeout(ctx, gameWriteTimeout)
			err := conn.Write(wctx, websocket.MessageText, b)
			cancel()
			if err != nil {
				return
			}
		}
	}
}

// gameReadPump applies client intent to the room. Unparseable frames are
// skipped; sustained flooding closes the socket.
func gameReadPump(ctx context.Context, conn *websocket.Conn, room *game.Room, slot int) {
	var lim msgLimiter
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			return
		}
		if !lim.allow(time.Now(), maxClientMsgRate) {
			_ = conn.Close(websocket.StatusPolicyViolation, "message rate exceeded")
			return
		}

		var msg game.ClientMsg
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}
		switch msg.T {
		case game.MsgInput:
			room.Input(slot, game.Input{DX: msg.DX, DY: msg.DY, Bomb: msg.Bomb})
		case game.MsgStart, game.MsgRestart:
			room.Begin(slot) // the room enforces that only the host may start
		}
	}
}

func writeWSJSON(ctx context.Context, conn *websocket.Conn, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	wctx, cancel := context.WithTimeout(ctx, gameWriteTimeout)
	defer cancel()
	return conn.Write(wctx, websocket.MessageText, b)
}

// msgLimiter is a fixed-window frame counter for one connection. It is touched
// only by that connection's read pump, so it needs no lock.
type msgLimiter struct {
	since time.Time
	count int
}

func (l *msgLimiter) allow(now time.Time, limit int) bool {
	if now.Sub(l.since) >= time.Second {
		l.since, l.count = now, 0
	}
	l.count++
	return l.count <= limit
}
