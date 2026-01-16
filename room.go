package main

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

type Room struct {
	id       string
	mu       sync.RWMutex
	clients  map[string]*Client
	cache    *RoomCache
	pending  map[string]chan Event
	macID    string
	isActive bool
}

func NewRoom(id, macID string) *Room {
	return &Room{
		id:       id,
		clients:  make(map[string]*Client),
		cache:    NewRoomCache(),
		pending:  make(map[string]chan Event),
		macID:    macID,
		isActive: true,
	}
}
func (r *Room) addClient(c *Client) {
	if c == nil {
		return
	}

	var prior *Client

	r.mu.Lock()
	prior = r.clients[c.deviceID]
	// Replace any existing connection for the same device ID.
	// This prevents stale disconnect handlers from deleting the new connection.
	r.clients[c.deviceID] = c
	c.mu.Lock()
	c.room = r
	c.mu.Unlock()
	r.mu.Unlock()

	if prior != nil && prior != c {
		prior.closeConn()
	}

	// Notify other clients about the new peer
	r.broadcastExcept(c.deviceID, Event{
		Type:      EventPeerConnected,
		RoomID:    r.id,
		DeviceID:  c.deviceID,
		Timestamp: time.Now(),
		Payload:   []byte(fmt.Sprintf(`{"device_type":"%s"}`, c.deviceType)),
	})
}

func (r *Room) removeClient(c *Client) {
	if c == nil {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Only remove if this is still the active connection for this device.
	// A stale connection (e.g., after a quick reconnect) must not delete the new one.
	existing, exists := r.clients[c.deviceID]
	if !exists || existing != c {
		return
	}

	deviceType := c.deviceType
	deviceID := c.deviceID
	isMac := deviceID == r.macID

	// Remove client from room
	delete(r.clients, deviceID)

	// Clear client's room reference safely
	c.mu.Lock()
	c.room = nil
	c.mu.Unlock()

	// Clean up pending requests from this client only
	// We need to track which requests belong to which client
	// For now, we'll clean up all pending requests if Mac disconnects
	// Otherwise, we'll let them timeout naturally
	if isMac {
		// Mac disconnected - clean up all pending requests
		for reqID, ch := range r.pending {
			select {
			case <-ch:
				// Channel already closed or received
			default:
				close(ch)
			}
			delete(r.pending, reqID)
		}
	}

	// Notify remaining clients about the disconnection
	remainingClients := make([]*Client, 0, len(r.clients))
	for _, client := range r.clients {
		if client != nil {
			remainingClients = append(remainingClients, client)
		}
	}

	// If mac leaves, deactivate room and notify all watches
	if isMac {
		r.isActive = false
		statusPayload, _ := json.Marshal(map[string]any{"in_room": false, "mac_disconnected": true})
		statusEvent := Event{
			Type:      EventStatusUpdate,
			RoomID:    r.id,
			Timestamp: time.Now(),
			Payload:   statusPayload,
		}

		// Notify all remaining clients (watches)
		for _, client := range remainingClients {
			if client != nil {
				client.send(Event{
					Type:      EventPeerDisconnected,
					RoomID:    r.id,
					DeviceID:  deviceID,
					Timestamp: time.Now(),
					Payload:   []byte(fmt.Sprintf(`{"device_type":"%s"}`, deviceType)),
				})
				client.send(statusEvent)
			}
		}
	} else {
		// Watch disconnected - notify remaining clients
		disconnectEvent := Event{
			Type:      EventPeerDisconnected,
			RoomID:    r.id,
			DeviceID:  deviceID,
			Timestamp: time.Now(),
			Payload:   []byte(fmt.Sprintf(`{"device_type":"%s"}`, deviceType)),
		}

		for _, client := range remainingClients {
			if client != nil {
				client.send(disconnectEvent)
			}
		}

		// Notify Mac about watch disconnection
		if mac := r.getPeer(DeviceTypeMac); mac != nil {
			watchConnected := false
			for _, client := range remainingClients {
				if client != nil && client.deviceType == DeviceTypeWatch {
					watchConnected = true
					break
				}
			}
			statusPayload, _ := json.Marshal(map[string]any{
				"in_room":         true,
				"watch_connected": watchConnected,
			})
			mac.send(Event{
				Type:      EventStatusUpdate,
				RoomID:    r.id,
				Timestamp: time.Now(),
				Payload:   statusPayload,
			})
		}
	}
}

func (r *Room) getPeer(deviceType string) *Client {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, client := range r.clients {
		if client != nil && client.deviceType == deviceType {
			return client
		}
	}
	return nil
}

func (r *Room) broadcastExcept(excludeDeviceID string, ev Event) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	r.broadcastExceptLocked(excludeDeviceID, ev)
}

func (r *Room) broadcastExceptLocked(excludeDeviceID string, ev Event) {
	for deviceID, client := range r.clients {
		if deviceID != excludeDeviceID && client != nil {
			client.send(ev)
		}
	}
}

func (r *Room) waitForResponse(requestID string) <-chan Event {
	r.mu.Lock()
	defer r.mu.Unlock()

	ch := make(chan Event, 1)
	r.pending[requestID] = ch
	return ch
}

func (r *Room) fulfillResponse(ev Event) bool {
	if ev.RequestID == "" {
		return false
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	ch, exists := r.pending[ev.RequestID]
	if !exists {
		return false
	}

	delete(r.pending, ev.RequestID)

	// Try to send response, but don't block
	select {
	case ch <- ev:
		close(ch)
		return true
	default:
		// Channel might be full or closed, close it anyway
		close(ch)
		return true
	}
}
