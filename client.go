package main

import (
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"
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
	return &Client{
		conn:    conn,
		manager: m,
		egress:  make(chan Event, 64), // Larger buffer for better performance
		done:    make(chan struct{}),
	}
}

func (c *Client) readMessages() {
	defer func() {
		c.manager.removeClient(c)
		c.closeConn()
	}()

	for {
		_, payload, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error for device %s: %v", c.deviceID, err)
			}
			break
		}

		var event Event

		if err := json.Unmarshal(payload, &event); err != nil {
			log.Printf("Error unmarshaling event from %s: %v", c.deviceID, err)
			continue
		}

		event.Timestamp = time.Now()
		if err := c.manager.routeEvent(event, c); err != nil {
			log.Printf("Error routing event from %s: %v", c.deviceID, err)
			c.sendError("", "routing_error", err.Error())
		}
	}
}

func (c *Client) writeMessages() {
	defer c.closeConn()

	for {
		select {
		case message, ok := <-c.egress:
			if !ok {
				return
			}

			if err := c.conn.WriteJSON(message); err != nil {
				log.Printf("Error writing message to %s: %v", c.deviceID, err)
				return
			}
		case <-c.done:
			return
		}
	}
}

func (c *Client) send(ev Event) {
	select {
	case c.egress <- ev:
	default:
		log.Printf("Egress full for %s, dropping message: %s", c.deviceID, ev.Type)
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
		c.conn.Close()
		close(c.egress)
		close(c.done)
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
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				inRoom := c.room != nil
				watchConnected := false
				if inRoom {
					if peer := c.room.getPeer(DeviceTypeWatch); peer != nil {
						watchConnected = true
					}
				}
				payload := map[string]any{"in_room": inRoom, "watch_connected": watchConnected}
				b, _ := json.Marshal(payload)
				c.send(Event{Type: EventStatusUpdate, RoomID: func() string {
					if c.room != nil {
						return c.room.id
					}
					return ""
				}(), Timestamp: time.Now(), Payload: b})
			case <-c.done:
				return
			}
		}
	}()
}
