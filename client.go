package main

import (
	"encoding/json"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// WriteWait is the time allowed to write a message to the peer
	writeWait = 10 * time.Second
	// PongWait is the time allowed to read the next pong message from the peer
	pongWait = 60 * time.Second
	// PingPeriod is the interval at which pings are sent (must be less than pongWait)
	pingPeriod = (pongWait * 9) / 10
	// MaxMessageSize is the maximum message size allowed from peer
	maxMessageSize = 512 * 1024 // 512KB
)

type ClientList map[*Client]bool

type Client struct {
	conn       *websocket.Conn
	manager    *Manager
	egress     chan Event
	deviceID   string
	deviceType string // "mac" or "watch"
	room       *Room
	closeOnce  sync.Once
	mu         sync.RWMutex
	done       chan struct{}
}

func NewClient(conn *websocket.Conn, m *Manager) *Client {
	// Set connection limits and timeouts
	conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetReadLimit(maxMessageSize)
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	return &Client{
		conn:    conn,
		manager: m,
		egress:  make(chan Event, 64), // Larger buffer for better performance
		done:    make(chan struct{}),
	}
}

func (c *Client) readMessages() {
	defer func() {
		// Recover from any panics
		if r := recover(); r != nil {
			log.Printf("PANIC recovered in readMessages for device %s: %v", c.deviceID, r)
		}
		c.manager.removeClient(c)
		c.closeConn()
	}()

	for {
		_, payload, err := c.conn.ReadMessage()
		if err != nil {
			// Check for close errors
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				// Normal closure - client closed gracefully
				log.Printf("Device %s (%s) disconnected normally", c.deviceID, c.deviceType)
			} else if websocket.IsCloseError(err, websocket.CloseAbnormalClosure) {
				// Abnormal closure (1006) - connection closed without proper handshake
				// This is common when client crashes, network issues, or client closes abruptly
				// Don't log as error, just as info
				log.Printf("Device %s (%s) disconnected abnormally (connection closed without handshake)", c.deviceID, c.deviceType)
			} else if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				// Other unexpected close errors
				log.Printf("WebSocket error for device %s (%s): %v", c.deviceID, c.deviceType, err)
			} else {
				// Other read errors (timeout, etc.)
				log.Printf("Device %s (%s) disconnected: %v", c.deviceID, c.deviceType, err)
			}
			break
		}

		var event Event
		if err := json.Unmarshal(payload, &event); err != nil {
			log.Printf("Error unmarshaling event from %s: %v", c.deviceID, err)
			c.sendError("", "invalid_json", "Failed to parse event JSON")
			continue
		}

		event.Timestamp = time.Now()
		if err := c.manager.routeEvent(event, c); err != nil {
			log.Printf("Error routing event %s from %s: %v", event.Type, c.deviceID, err)
			c.sendError(event.RequestID, "routing_error", err.Error())
		}
	}
}

func (c *Client) writeMessages() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		if r := recover(); r != nil {
			log.Printf("PANIC recovered in writeMessages for device %s: %v", c.deviceID, r)
		}
		ticker.Stop()
		c.closeConn()
	}()

	for {
		select {
		case message, ok := <-c.egress:
			if c.conn == nil {
				return
			}
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// Channel closed, send close message
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := c.conn.WriteJSON(message); err != nil {
				// Suppress expected errors when client disconnects
				if websocket.IsCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) ||
					websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					return
				}
				// Check for network errors
				if netErr, ok := err.(*net.OpError); ok {
					if netErr.Err != nil {
						errStr := netErr.Err.Error()
						if strings.Contains(errStr, "broken pipe") ||
							strings.Contains(errStr, "connection reset") ||
							strings.Contains(errStr, "use of closed network connection") {
							return
						}
					}
				}
				// Check error string
				errStr := err.Error()
				if strings.Contains(errStr, "broken pipe") ||
					strings.Contains(errStr, "connection reset") ||
					strings.Contains(errStr, "use of closed network connection") {
					return
				}
				// Log unexpected errors
				log.Printf("Error writing message to %s (%s): %v", c.deviceID, c.deviceType, err)
				return
			}

		case <-ticker.C:
			// Send ping to keep connection alive
			if c.conn == nil {
				return
			}
			// Check if client is already closed
			select {
			case <-c.done:
				return
			default:
			}
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				// Suppress expected errors when client disconnects
				// Check for WebSocket close errors
				if websocket.IsCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) ||
					websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					return
				}
				// Check for network errors (broken pipe, connection reset, etc.)
				if netErr, ok := err.(*net.OpError); ok {
					if netErr.Err != nil {
						errStr := netErr.Err.Error()
						if strings.Contains(errStr, "broken pipe") ||
							strings.Contains(errStr, "connection reset") ||
							strings.Contains(errStr, "use of closed network connection") {
							return
						}
					}
				}
				// Check error string for broken pipe (some errors don't wrap as OpError)
				errStr := err.Error()
				if strings.Contains(errStr, "broken pipe") ||
					strings.Contains(errStr, "connection reset") ||
					strings.Contains(errStr, "use of closed network connection") {
					return
				}
				// Log unexpected errors
				log.Printf("Error sending ping to %s (%s): %v", c.deviceID, c.deviceType, err)
				return
			}

		case <-c.done:
			return
		}
	}
}

func (c *Client) send(ev Event) {
	// Check if client is already closed
	select {
	case <-c.done:
		return
	default:
	}

	select {
	case c.egress <- ev:
	default:
		log.Printf("Egress channel full for %s (%s), dropping message: %s", c.deviceID, c.deviceType, ev.Type)
	}
}

func (c *Client) sendError(requestID, code, message string) {
	payload := map[string]string{"code": code, "message": message}
	b, _ := json.Marshal(payload)
	c.send(Event{
		Type:      EventError,
		RequestID: requestID,
		Timestamp: time.Now(),
		Payload:   b,
	})
}

func (c *Client) closeConn() {
	c.closeOnce.Do(func() {
		// Close done first so goroutines can stop promptly.
		select {
		case <-c.done:
			// already closed
		default:
			close(c.done)
		}

		// Safely close connection.
		if c.conn != nil {
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			_ = c.conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			_ = c.conn.Close()
		}

		// Close egress after the connection is closed; write loop will exit.
		// `closeOnce` guarantees this runs only once.
		close(c.egress)
	})
}

// startStatusPinger periodically informs the client of connection status.
// For Mac devices:
// - If not in a room: send status_update { in_room:false }
// - If in a room: send status_update { in_room:true, watch_connected: bool }
func (c *Client) startStatusPinger() {
	if c.deviceType != DeviceTypeMac {
		return
	}

	ticker := time.NewTicker(statusInterval)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("PANIC recovered in startStatusPinger for device %s: %v", c.deviceID, r)
			}
			ticker.Stop()
		}()

		for {
			select {
			case <-ticker.C:
				c.mu.RLock()
				room := c.room
				c.mu.RUnlock()

				inRoom := room != nil
				watchConnected := false
				roomID := ""

				if inRoom && room != nil {
					roomID = room.id
					if peer := room.getPeer(DeviceTypeWatch); peer != nil {
						watchConnected = true
					}
				}

				payload := map[string]any{"in_room": inRoom, "watch_connected": watchConnected}
				b, err := json.Marshal(payload)
				if err != nil {
					log.Printf("Error marshaling status update for %s: %v", c.deviceID, err)
					continue
				}

				c.send(Event{
					Type:      EventStatusUpdate,
					RoomID:    roomID,
					Timestamp: time.Now(),
					Payload:   b,
				})

			case <-c.done:
				return
			}
		}
	}()
}
