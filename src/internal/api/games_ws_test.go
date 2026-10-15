package api

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// Dialing the game socket for a room that does not exist must close with the terminal
// wsCloseRoomGone code (not a plain upgrade failure), so a client can tell "the room is
// gone, stop reconnecting" apart from a transient network drop (which surfaces as 1006).
func TestGameWSUnknownRoomClosesRoomGone(t *testing.T) {
	srv := httptest.NewServer(New(&fakeCatalog{}, nil, nil).Router)
	defer srv.Close()

	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/api/v1/games/ws?code=ZZZZ&name=bob"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		t.Fatalf("the upgrade should succeed (we refuse over the socket, not the handshake): %v", err)
	}
	defer conn.CloseNow()

	// The server writes an error frame and then closes; read until the close surfaces.
	var closeErr error
	for i := 0; i < 4; i++ {
		if _, _, rerr := conn.Read(ctx); rerr != nil {
			closeErr = rerr
			break
		}
	}
	if got := websocket.CloseStatus(closeErr); got != wsCloseRoomGone {
		t.Fatalf("want close code %d for a missing room, got %d (err %v)", wsCloseRoomGone, got, closeErr)
	}
}
