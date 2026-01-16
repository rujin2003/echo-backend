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
	log.Printf("Room created: id=%s mac_id=%s", roomID, macID)
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
	if c == nil {
		return
	}

	defer func() {
		if r := recover(); r != nil {
			log.Printf("PANIC recovered in removeClient for device %s: %v", c.deviceID, r)
		}
	}()

	c.mu.RLock()
	room := c.room
	c.mu.RUnlock()

	if room != nil {
		room.removeClient(c)

		// Clean up empty or inactive rooms
		m.mu.Lock()
		room.mu.RLock()
		clientCount := len(room.clients)
		isActive := room.isActive
		roomID := room.id
		room.mu.RUnlock()

		if clientCount == 0 || !isActive {
			delete(m.rooms, roomID)
			log.Printf("Room %s cleaned up (clients: %d, active: %v)", roomID, clientCount, isActive)
		}
		m.mu.Unlock()
	}

	log.Printf("Client %s (%s) removed from manager", c.deviceID, c.deviceType)
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
	existingRoom, exists := m.getRoom(payload.RoomID)
	if exists {
		// If room exists and this Mac is the owner, just add the client to the room
		existingRoom.mu.RLock()
		isOwner := existingRoom.macID == c.deviceID
		roomID := existingRoom.id
		existingRoom.mu.RUnlock()
		
		if isOwner {
			log.Printf("Device %s (%s) rejoining existing room %s", c.deviceID, c.deviceType, payload.RoomID)
			
			// Reactivate room if it was deactivated
			existingRoom.mu.Lock()
			existingRoom.isActive = true
			existingRoom.mu.Unlock()
			
			existingRoom.addClient(c)
			
			// Check if watch is already connected
			watchConnected := existingRoom.getPeer(DeviceTypeWatch) != nil
			statusPayload := fmt.Sprintf(`{"in_room":true,"watch_connected":%t}`, watchConnected)
			
			// Immediately inform Mac of status after rejoining
			c.send(Event{Type: EventStatusUpdate, RoomID: roomID, Timestamp: time.Now(), Payload: []byte(statusPayload)})
			
			c.send(Event{
				Type:      EventRoomJoined,
				RoomID:    roomID,
				Timestamp: time.Now(),
				Payload:   []byte(fmt.Sprintf(`{"status":"rejoined","role":"host"}`)),
			})
			return nil
		}
		return errors.New("room already exists")
	}

	// Log which device is creating the room
	log.Printf("Device %s (%s) creating room %s", c.deviceID, c.deviceType, payload.RoomID)

	room := m.createRoom(payload.RoomID, c.deviceID)
	room.addClient(c)

	// Immediately inform Mac of status after room creation
	c.send(Event{Type: EventStatusUpdate, RoomID: room.id, Timestamp: time.Now(), Payload: []byte(`{"in_room":true,"watch_connected":false}`)})

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

	// If a watch joined, notify Mac about watch connection status
	if c.deviceType == DeviceTypeWatch {
		if mac := room.getPeer(DeviceTypeMac); mac != nil {
			mac.send(Event{Type: EventStatusUpdate, RoomID: room.id, Timestamp: time.Now(), Payload: []byte(`{"in_room":true,"watch_connected":true}`)})
		}
	}

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
		// Silently ignore if not in a room - client may send this before joining
		return nil
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
		// Silently ignore if not in a room - client may send this before joining
		return nil
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
		// Silently ignore if not in a room - client may send this before joining
		return nil
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
		// Silently ignore if not in a room - client may send this before joining
		return nil
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
	// Validate media actions (case-insensitive check)
	validActions := map[string]bool{
		"play": true, "pause": true, "volup": true, "voldown": true,
		"next": true, "prev": true, "volumeup": true, "volumedown": true,
	}
	if !validActions[payload.Action] {
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
			defer func() {
				if r := recover(); r != nil {
					log.Printf("PANIC recovered in handleActionRequest goroutine: %v", r)
				}
			}()

			select {
			case resp, ok := <-respCh:
				if ok && c != nil {
					c.send(Event{
						Type:      EventActionResult,
						RequestID: resp.RequestID,
						RoomID:    c.room.id,
						Timestamp: time.Now(),
						Payload:   resp.Payload,
					})
				}
			case <-time.After(requestTimeout):
				if c != nil {
					c.sendError(ev.RequestID, "timeout", "Mac did not respond in time")
				}
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

func (m *Manager) handleRoomStatus(_ Event, c *Client) error {

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
		defer func() {
			if r := recover(); r != nil {
				log.Printf("PANIC recovered in handleGenericRequest goroutine: %v", r)
			}
		}()

		select {
		case resp, ok := <-respCh:
			if ok && c != nil && c.room != nil {
				c.send(Event{
					Type:      EventResponse,
					RequestID: resp.RequestID,
					RoomID:    c.room.id,
					Timestamp: time.Now(),
					Payload:   resp.Payload,
				})
			}
		case <-time.After(requestTimeout):
			if c != nil {
				c.sendError(ev.RequestID, "timeout", "Peer did not respond in time")
			}
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
	// Recover from panics in event handlers
	defer func() {
		if r := recover(); r != nil {
			log.Printf("PANIC recovered in routeEvent for event %s from device %s: %v", ev.Type, c.deviceID, r)
		}
	}()

	if c == nil {
		return errors.New("client is nil")
	}

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
	// Recover from any panics during connection setup
	defer func() {
		if r := recover(); r != nil {
			log.Printf("PANIC recovered in serveWs: %v", r)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
	}()

	// JWT validation
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Error(w, "Missing token", http.StatusUnauthorized)
		return
	}

	claims, err := validateJWT(token)
	if err != nil {
		log.Printf("JWT validation failed: %v", err)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Extract device info from JWT
	deviceID, ok := claims["device_id"].(string)
	if !ok || deviceID == "" {
		http.Error(w, "Missing or invalid device_id in token", http.StatusBadRequest)
		return
	}

	deviceType, ok := claims["device_type"].(string)
	if !ok || (deviceType != DeviceTypeMac && deviceType != DeviceTypeWatch) {
		http.Error(w, "Invalid device_type in token", http.StatusBadRequest)
		return
	}

	conn, err := m.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Failed to upgrade connection for device %s: %v", deviceID, err)
		return
	}

	client := NewClient(conn, m)
	client.deviceID = deviceID
	client.deviceType = deviceType

	log.Printf("Device %s (%s) connected from %s", deviceID, deviceType, r.RemoteAddr)

	// Start write handler first to ensure we can send messages
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("PANIC recovered in writeMessages goroutine for %s: %v", deviceID, r)
			}
		}()
		client.writeMessages()
	}()

	// Start read handler
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("PANIC recovered in readMessages goroutine for %s: %v", deviceID, r)
			}
		}()
		client.readMessages()
	}()

	// On connection, send a basic connect event and start status pinger for Macs
	// Use a small delay to ensure writeMessages goroutine is ready
	go func() {
		time.Sleep(10 * time.Millisecond)
		client.send(Event{Type: EventConnect, Timestamp: time.Now()})
		client.startStatusPinger()
	}()
}
