package sdk

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// The event stream must survive every middleware in front of it: a
// response wrapper that hides the connection makes the upgrade fail.
func TestWebSocketUpgradesThroughTheRouter(t *testing.T) {
	ts := setupTestServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, resp, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(ts.URL, "http")+"/api/v1/ws", nil)
	if err != nil {
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}
		t.Fatalf("WebSocket upgrade failed (HTTP %d): %v", status, err)
	}
	_ = conn.Close(websocket.StatusNormalClosure, "")
}
