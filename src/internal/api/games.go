package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

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
	gamePingInterval = 20 * time.Second // keepalive so idle turn-based sockets survive proxy idle timeouts

	// wsCloseRoomGone is an application close code (the 4000-4999 private range, which
	// browsers expose on the close event) meaning "this room no longer exists, stop
	// reconnecting". A plain HTTP 404 on the upgrade is indistinguishable from a network
	// blip (both surface as close code 1006), so a mobile client that re-opens a dead
	// room would reconnect-spam it. We accept the upgrade and close with this instead.
	wsCloseRoomGone = websocket.StatusCode(4404)
)

// createRoom opens a game room and returns its join code. The ?game= slug picks
// which game (default "bomberman").
func (s *Server) createRoom(w http.ResponseWriter, r *http.Request) {
	slug := r.URL.Query().Get("game")
	if slug == "" {
		slug = "bomberman"
	}
	room, err := s.games.Create(slug)
	if errors.Is(err, game.ErrTooManyRooms) {
		writeErr(w, http.StatusServiceUnavailable, "no free game rooms right now")
		return
	}
	if err != nil {
		writeErr(w, http.StatusBadRequest, "unknown game")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"code": room.Code()})
}

// gameWS upgrades to a WebSocket and pumps one player's frames to and from
// their room for the life of the connection.
func (s *Server) gameWS(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

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

	// The Origin check is what stops a hostile page from driving someone's game
	// session. Same-origin always passes; OriginPatterns only adds the explicit
	// extras from config (empty in production).
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: s.gameOrigins,
	})
	if err != nil {
		return // Accept has already written the error response
	}
	defer conn.CloseNow()
	conn.SetReadLimit(maxGameFrame)

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	// Validate over the open socket (not as a pre-upgrade HTTP error) so the client
	// learns WHY it was refused from the close code and can act on it. A missing room
	// gets wsCloseRoomGone so the client stops retrying a room that no longer exists.
	room, ok := s.games.Get(game.NormalizeCode(q.Get("code")))
	if !ok {
		game.VerboseLogger().Warn("join refused: room not found",
			"event", "join_fail", "code", game.NormalizeCode(q.Get("code")), "reason", "room not found")
		_ = writeWSJSON(ctx, conn, game.ErrorMsg{T: game.MsgError, Err: "room not found"})
		_ = conn.Close(wsCloseRoomGone, "room not found")
		return
	}
	name, ok := cleanNick(q.Get("name"))
	if !ok {
		_ = writeWSJSON(ctx, conn, game.ErrorMsg{T: game.MsgError, Err: "invalid player name"})
		_ = conn.Close(websocket.StatusPolicyViolation, "invalid player name")
		return
	}
	// The animal is a catalog slug the client renders a sprite for; an
	// unrecognised shape is dropped rather than refused.
	animal := q.Get("animal")
	if !slugRe.MatchString(animal) {
		animal = ""
	}

	// A resume token reclaims the seat this client held before its socket
	// dropped. It is the player's own secret, issued in their last welcome.
	seat, welcome, err := room.Join(name, animal, q.Get("resume"))
	if err != nil {
		game.VerboseLogger().Warn("join refused",
			"event", "join_fail", "code", game.NormalizeCode(q.Get("code")), "reason", err.Error())
		_ = writeWSJSON(ctx, conn, game.ErrorMsg{T: game.MsgError, Err: err.Error()})
		_ = conn.Close(websocket.StatusTryAgainLater, "room unavailable")
		return
	}
	// Only an explicit goodbye (the Leave button -> a "bye" frame) releases the seat and
	// lets an emptied room be deleted. Every other exit is a dropped socket: HOLD the seat
	// for the reconnect grace. This includes a connection that dropped before it ever sent
	// a frame (an idle reconnect that flaps), which must NOT be treated as a deliberate
	// leave and must never delete the room (the resume + supersede flow reclaims the seat).
	var deliberate bool
	defer func() {
		if deliberate {
			room.Quit(seat) // bye: release the seat; delete the room if that empties it
			return
		}
		room.Leave(seat) // drop: hold the seat for a reconnect
	}()

	if err := writeWSJSON(ctx, conn, welcome); err != nil {
		return
	}

	// One pump per direction. Either one ending cancels ctx, which unblocks the
	// other, so a dead socket never leaves a goroutine parked on a channel.
	go func() {
		defer cancel()
		gameWritePump(ctx, conn, seat)
	}()
	// Keep the socket from going fully idle between turns. A turn-based game sends
	// nothing while nobody acts, so a proxy's idle read timeout (NGINX/WAF, a dev
	// proxy) would reap the connection and the client would appear to "time out"
	// just for sitting still. A periodic WebSocket ping (protocol-level, invisible
	// to the client) keeps intermediaries from dropping it. Bomberman never idles.
	go gamePingPump(ctx, conn)
	deliberate = gameReadPump(ctx, conn, room, seat)
}

// gamePingPump sends a WebSocket keepalive ping every gamePingInterval so an idle
// turn-based socket stays alive through any proxy. A failed ping (dead link) cancels
// the context via the read/write pumps, so this just returns.
func gamePingPump(ctx context.Context, conn *websocket.Conn) {
	t := time.NewTicker(gamePingInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			pctx, cancel := context.WithTimeout(ctx, gameWriteTimeout)
			err := conn.Ping(pctx)
			cancel()
			if err != nil {
				return
			}
		}
	}
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
				// The room closed (game over / everyone left); tell the client with the
				// terminal code so it stops reconnecting instead of spamming a dead room.
				_ = conn.Close(wsCloseRoomGone, "room closed")
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
// skipped; sustained flooding closes the socket. It reports whether the client
// left DELIBERATELY (a "bye" from the Leave button) — that alone decides whether
// the seat is released (and an emptied room deleted). A plain socket drop, even
// one before the client ever sent a frame (e.g. an idle reconnect), is NOT
// deliberate: the seat is held for a reconnect.
func gameReadPump(
	ctx context.Context,
	conn *websocket.Conn,
	room *game.Room,
	seat *game.Conn,
) (deliberate bool) {
	var lim msgLimiter
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			return false
		}
		if !lim.allow(time.Now(), maxClientMsgRate) {
			_ = conn.Close(websocket.StatusPolicyViolation, "message rate exceeded")
			return false
		}

		// Peek only at the type: a deliberate "bye" releases the seat, everything
		// else is handed to the game to parse and apply.
		var msg struct {
			T string `json:"t"`
		}
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}
		if msg.T == game.MsgBye {
			return true
		}
		room.Command(seat, data)
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

// NickMaxLen bounds a player nickname. It is drawn as a label above the
// player's sprite, so a long one would cover the board.
const NickMaxLen = 16

// cleanNick validates a game nickname. Unlike cleanName (which vets submitted
// animal names) digits are allowed, because "Niklas2" is a perfectly ordinary
// thing to call yourself. Everything else is kept tight: the nickname is
// broadcast to every other player in the room.
func cleanNick(s string) (string, bool) {
	s = strings.TrimSpace(s)
	n := utf8.RuneCountInString(s)
	if n < 1 || n > NickMaxLen {
		return "", false
	}
	for _, r := range s {
		switch {
		case unicode.IsLetter(r), unicode.IsMark(r), unicode.IsDigit(r), r == ' ':
		case r == '-' || r == '_' || r == '\'' || r == '’' || r == '.':
		default:
			return "", false
		}
	}
	return s, true
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
