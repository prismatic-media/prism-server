package handler

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/gorilla/websocket"

	"github.com/prismatic-media/prism-server/pkg/events"
)

// EventsHandler serves the global real-time event WebSocket.
type EventsHandler struct {
	bus *events.Bus
}

func NewEventsHandler(bus *events.Bus) *EventsHandler {
	return &EventsHandler{bus: bus}
}

// ServeEvents handles GET /api/v1/ws/events.
// It upgrades the connection, subscribes to the global event bus, and streams
// all events until the client disconnects or the server shuts down.
// Authentication is enforced at the router level by the Authenticate middleware
// before this handler is called.
func (h *EventsHandler) ServeEvents(w http.ResponseWriter, r *http.Request) {
	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("Failed to upgrade websocket", "error", err)
		return
	}
	defer func() { _ = conn.Close() }()

	subID, ch := h.bus.Subscribe()
	defer h.bus.Unsubscribe(subID)

	// Ping the client every 30 s to keep the connection alive through proxies.
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	// Drain incoming messages so the connection's read pump doesn't stall.
	go func() {
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	for {
		select {
		case evt, ok := <-ch:
			if !ok {
				slog.Info("channel closed")
				return
			}
			conn.SetWriteDeadline(time.Now().Add(10 * time.Second)) //nolint:errcheck
			if err := conn.WriteJSON(evt); err != nil {
				slog.Error("Failed to write event", "error", err)
				return
			}
		case <-ticker.C:
			conn.SetWriteDeadline(time.Now().Add(10 * time.Second)) //nolint:errcheck
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				slog.Error("Failed to write ping", "error", err)
				return
			}
		case <-r.Context().Done():
			slog.Info("request context done")
			return
		}
	}
}
