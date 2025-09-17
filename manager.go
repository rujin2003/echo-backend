package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type Manager struct {
	mu       sync.RWMutex
	rooms    map[string]*Room // roomID -> room
	upgrader websocket.Upgrader
}

func NewManager() *Manager {
	return &Manager{
		rooms: make(map[string]*Room),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}
}

func (m *Manager) createRoom(roomID, macID string) *Room {
	m.mu.Lock()
	defer m.mu.Unlock()

	room := NewRoom(roomID, macID)
	m.rooms[roomID] = room
	return room
}

func (m *Manager) getRoom(roomID string) (*Room, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	room, exists := m.rooms[roomID]
	if exists && !room.isActive {
		return nil, false
	}
	return room, exists
}

func (m *Manager) removeClient(c *Client) {
	if c.room != nil {
		c.room.removeClient(c)

		// Clean up empty rooms
		m.mu.Lock()
		if len(c.room.clients) == 0 || !c.room.isActive {
			delete(m.rooms, c.room.id)
		}
		m.mu.Unlock()
	}
}

// Event handlers
func (m *Manager) handleCreateRoom(ev Event, c *Client) error {
	var payload struct {
		RoomID string `json:"room_id"`
	}

	if err := json.Unmarshal(ev.Payload, &payload); err != nil {
		return fmt.Errorf("invalid payload: %w", err)
	}

	if c.deviceType != DeviceTypeMac {
		return errors.New("only Mac devices can create rooms")
	}

	// Check if room already exists
	if _, exists := m.getRoom(payload.RoomID); exists {
		return errors.New("room already exists")
	}

	room := m.createRoom(payload.RoomID, c.deviceID)
	room.addClient(c)

	c.send(Event{
		Type:      EventRoomJoined,
		RoomID:    room.id,
		Timestamp: time.Now(),
		Payload:   []byte(fmt.Sprintf(`{"status":"created","role":"host"}`)),
	})

	return nil
}

func (m *Manager) handleJoinRoom(ev Event, c *Client) error {
	var payload struct {
		RoomID string `json:"room_id"`
	}

	if err := json.Unmarshal(ev.Payload, &payload); err != nil {
		return fmt.Errorf("invalid payload: %w", err)
	}

	room, exists := m.getRoom(payload.RoomID)
	if !exists {
		return errors.New("room not found or inactive")
	}

	// Check device limits (1 Mac, multiple watches possible but typically 1)
	if c.deviceType == DeviceTypeMac && room.getPeer(DeviceTypeMac) != nil {
		return errors.New("room already has a Mac device")
	}

	room.addClient(c)

	// Send cached data to new client if available
	m.sendCachedData(c, room)

	c.send(Event{
		Type:      EventRoomJoined,
		RoomID:    room.id,
		Timestamp: time.Now(),
		Payload:   []byte(fmt.Sprintf(`{"status":"joined","role":"client"}`)),
	})

	return nil
}

func (m *Manager) sendCachedData(c *Client, room *Room) {

	if c.deviceType == DeviceTypeWatch {

		if data, ok := room.cache.Get("device_info"); ok {
			c.send(Event{Type: EventDeviceInfo, RoomID: room.id, Timestamp: time.Now(), Payload: data})
		}
		if data, ok := room.cache.Get("battery"); ok {
			c.send(Event{Type: EventBatteryUpdate, RoomID: room.id, Timestamp: time.Now(), Payload: data})
		}
		if data, ok := room.cache.Get("storage"); ok {
			c.send(Event{Type: EventStorageUpdate, RoomID: room.id, Timestamp: time.Now(), Payload: data})
		}
		if data, ok := room.cache.Get("downloads"); ok {
			c.send(Event{Type: EventDownloadsUpdate, RoomID: room.id, Timestamp: time.Now(), Payload: data})
		}
	}
}

func (m *Manager) handleDeviceInfo(ev Event, c *Client) error {
	if c.room == nil {
		return errors.New("not in a room")
	}

	// Only Mac can send device info
	if c.deviceType != DeviceTypeMac {
		return errors.New("only Mac devices can send device info")
	}

	// Cache with long TTL (static data)
	c.room.cache.Set("device_info", ev.Payload, 24*time.Hour)

	// Broadcast to watches
	c.room.broadcastExcept(c.deviceID, Event{
		Type:      EventDeviceInfo,
		RoomID:    c.room.id,
		DeviceID:  c.deviceID,
		Timestamp: time.Now(),
		Payload:   ev.Payload,
	})

	return nil
}

func (m *Manager) handleBatteryUpdate(ev Event, c *Client) error {
	if c.room == nil {
		return errors.New("not in a room")
	}

	// Cache with short TTL (dynamic data)
	c.room.cache.Set("battery", ev.Payload, batteryTTL)

	c.room.broadcastExcept(c.deviceID, Event{
		Type:      EventBatteryUpdate,
		RoomID:    c.room.id,
		DeviceID:  c.deviceID,
		Timestamp: time.Now(),
		Payload:   ev.Payload,
	})

	return nil
}

func (m *Manager) handleStorageUpdate(ev Event, c *Client) error {
	if c.room == nil {
		return errors.New("not in a room")
	}

	// Cache with medium TTL (semi-dynamic data)
	c.room.cache.Set("storage", ev.Payload, cacheTTL)

	c.room.broadcastExcept(c.deviceID, Event{
		Type:      EventStorageUpdate,
		RoomID:    c.room.id,
		DeviceID:  c.deviceID,
		Timestamp: time.Now(),
		Payload:   ev.Payload,
	})

	return nil
}

func (m *Manager) handleDownloadsUpdate(ev Event, c *Client) error {
	if c.room == nil {
		return errors.New("not in a room")
	}

	// Cache with short TTL (dynamic data)
	c.room.cache.Set("downloads", ev.Payload, downloadsTTL)

	c.room.broadcastExcept(c.deviceID, Event{
		Type:      EventDownloadsUpdate,
		RoomID:    c.room.id,
		DeviceID:  c.deviceID,
		Timestamp: time.Now(),
		Payload:   ev.Payload,
	})

	return nil
}
func (m *Manager) handleMediaAction(ev Event, c *Client) error {
	if c.room == nil {
		return errors.New("not in a room")
	}

	// Only Watch can request media actions
	if c.deviceType != DeviceTypeWatch {
		return errors.New("only Watch devices can request media actions")
	}
	var payload struct {
		Action string `json:"action"`
	}
	if err := json.Unmarshal(ev.Payload, &payload); err != nil {
		return fmt.Errorf("invalid payload: %w", err)
	}
	if payload.Action != "play" && payload.Action != "pause" && payload.Action != "volup" && payload.Action != "volDown" && payload.Action != "next" && payload.Action != "prev" {
		return errors.New("invalid media action")
	}
	mac := c.room.getPeer(DeviceTypeMac)
	if mac == nil {
		c.sendError(ev.RequestID, "mac_unavailable", "Mac device not connected")
		return nil
	}
	mac.send(Event{
		Type:      EventMediaAction,
		RoomID:    c.room.id,
		DeviceID:  c.deviceID,
		RequestID: ev.RequestID,
		Timestamp: time.Now(),
		Payload:   ev.Payload,
	})
	return nil
}
func (m *Manager) handleActionRequest(ev Event, c *Client) error {
	if c.room == nil {
		return errors.New("not in a room")
	}

	// Only Watch can request actions
	if c.deviceType != DeviceTypeWatch {
		return errors.New("only Watch devices can request actions")
	}

	var payload struct {
		Action string `json:"action"`
	}

	if err := json.Unmarshal(ev.Payload, &payload); err != nil {
		return fmt.Errorf("invalid payload: %w", err)
	}

	// Validate action
	if payload.Action != "shutdown" && payload.Action != "sleep" {
		return errors.New("invalid action")
	}

	// Forward to Mac
	mac := c.room.getPeer(DeviceTypeMac)
	if mac == nil {
		c.sendError(ev.RequestID, "mac_unavailable", "Mac device not connected")
		return nil
	}

	// Create response waiter if request ID provided
	if ev.RequestID != "" {
		respCh := c.room.waitForResponse(ev.RequestID)

		// Forward to Mac
		mac.send(Event{
			Type:      EventActionRequest,
			RoomID:    c.room.id,
			DeviceID:  c.deviceID,
			RequestID: ev.RequestID,
			Timestamp: time.Now(),
			Payload:   ev.Payload,
		})

		// Wait for response
		go func() {
			select {
			case resp := <-respCh:
				c.send(Event{
					Type:      EventActionResult,
					RequestID: resp.RequestID,
					RoomID:    c.room.id,
					Timestamp: time.Now(),
					Payload:   resp.Payload,
				})
			case <-time.After(requestTimeout):
				c.sendError(ev.RequestID, "timeout", "Mac did not respond in time")
			}
		}()
	} else {
		// Fire and forget
		mac.send(Event{
			Type:      EventActionRequest,
			RoomID:    c.room.id,
			DeviceID:  c.deviceID,
			Timestamp: time.Now(),
			Payload:   ev.Payload,
		})
	}

	return nil
}

func (m *Manager) handleActionResult(ev Event, c *Client) error {
	if c.room == nil {
		return errors.New("not in a room")
	}

	// Only Mac can send action results
	if c.deviceType != DeviceTypeMac {
		return errors.New("only Mac devices can send action results")
	}

	// Fulfill pending response
	c.room.fulfillResponse(ev)
	return nil
}

func (m *Manager) handleRoomStatus(ev Event, c *Client) error {

	if c.room == nil {
		c.send(Event{
			Type:      EventResponse,
			Timestamp: time.Now(),
			Payload:   []byte(`{"status":"false"}`),
		})
	} else {
		c.send(Event{
			Type:      EventResponse,
			Timestamp: time.Now(),
			Payload:   []byte(`{"status":"true"}`),
		})
	}

	return nil

}
func (m *Manager) handleGenericRequest(ev Event, c *Client) error {
	if c.room == nil {
		return errors.New("not in a room")
	}

	if ev.RequestID == "" {
		return errors.New("missing request_id")
	}

	var payload struct {
		Action string `json:"action"`
	}
	json.Unmarshal(ev.Payload, &payload)

	// Try cache first for certain requests
	switch payload.Action {
	case "get_device_info":
		if data, ok := c.room.cache.Get("device_info"); ok {
			c.send(Event{
				Type:      EventResponse,
				RequestID: ev.RequestID,
				RoomID:    c.room.id,
				Timestamp: time.Now(),
				Payload:   data,
			})
			return nil
		}
	case "get_battery":
		if data, ok := c.room.cache.Get("battery"); ok {
			c.send(Event{
				Type:      EventResponse,
				RequestID: ev.RequestID,
				RoomID:    c.room.id,
				Timestamp: time.Now(),
				Payload:   data,
			})
			return nil
		}
	}

	// Forward to appropriate peer
	var target *Client
	if c.deviceType == DeviceTypeWatch {
		target = c.room.getPeer(DeviceTypeMac)
	} else {
		target = c.room.getPeer(DeviceTypeWatch)
	}

	if target == nil {
		c.sendError(ev.RequestID, "peer_unavailable", "Target device not connected")
		return nil
	}

	respCh := c.room.waitForResponse(ev.RequestID)
	target.send(ev)

	go func() {
		select {
		case resp := <-respCh:
			c.send(Event{
				Type:      EventResponse,
				RequestID: resp.RequestID,
				RoomID:    c.room.id,
				Timestamp: time.Now(),
				Payload:   resp.Payload,
			})
		case <-time.After(requestTimeout):
			c.sendError(ev.RequestID, "timeout", "Peer did not respond in time")
		}
	}()

	return nil
}

func (m *Manager) handleResponse(ev Event, c *Client) error {
	if c.room == nil {
		return errors.New("not in a room")
	}

	c.room.fulfillResponse(ev)
	return nil
}

func (m *Manager) routeEvent(ev Event, c *Client) error {
	switch ev.Type {
	case EventRoomStatus:
		return m.handleRoomStatus(ev, c)
	case EventCreateRoom:
		return m.handleCreateRoom(ev, c)
	case EventJoinRoom:
		return m.handleJoinRoom(ev, c)
	case EventDeviceInfo:
		return m.handleDeviceInfo(ev, c)
	case EventBatteryUpdate:
		return m.handleBatteryUpdate(ev, c)
	case EventStorageUpdate:
		return m.handleStorageUpdate(ev, c)
	case EventDownloadsUpdate:
		return m.handleDownloadsUpdate(ev, c)
	case EventActionRequest:
		return m.handleActionRequest(ev, c)
	case EventMediaAction:
		return m.handleMediaAction(ev, c)
	case EventActionResult:
		return m.handleActionResult(ev, c)
	case EventRequest:
		return m.handleGenericRequest(ev, c)
	case EventResponse:
		return m.handleResponse(ev, c)
	default:
		return fmt.Errorf("unknown event type: %s", ev.Type)
	}
}

func (m *Manager) serveWs(w http.ResponseWriter, r *http.Request) {
	// JWT validation
	token := r.URL.Query().Get("token")
	claims, err := validateJWT(token)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Extract device info from JWT
	deviceID, ok := claims["device_id"].(string)
	if !ok {
		http.Error(w, "Missing device_id in token", http.StatusBadRequest)
		return
	}

	deviceType, ok := claims["device_type"].(string)
	if !ok || (deviceType != DeviceTypeMac && deviceType != DeviceTypeWatch) {
		http.Error(w, "Invalid device_type in token", http.StatusBadRequest)
		return
	}

	conn, err := m.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Failed to upgrade connection: %v", err)
		return
	}

	client := NewClient(conn, m)
	client.deviceID = deviceID
	client.deviceType = deviceType

	log.Printf("Device %s (%s) connected", deviceID, deviceType)

	go client.readMessages()
	go client.writeMessages()
}
