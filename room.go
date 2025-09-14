package main

import (
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
	r.mu.Lock()
	defer r.mu.Unlock()

	r.clients[c.deviceID] = c
	c.room = r

	// Notify other clients about the new peer
	r.broadcastExceptLocked(c.deviceID, Event{
		Type:      EventPeerConnected,
		RoomID:    r.id,
		DeviceID:  c.deviceID,
		Timestamp: time.Now(),
		Payload:   []byte(fmt.Sprintf(`{"device_type":"%s"}`, c.deviceType)),
	})
}

func (r *Room) removeClient(c *Client) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.clients, c.deviceID)
	c.room = nil

	// Clean up Pending requests from this client
	for reqID := range r.pending {
		close(r.pending[reqID])
		delete(r.pending, reqID)
	}

	// Notify remaining clients
	r.broadcastExceptLocked(c.deviceID, Event{
		Type:      EventPeerDisconnected,
		RoomID:    r.id,
		DeviceID:  c.deviceID,
		Timestamp: time.Now(),
	})

	// If mac leaves, deactivate room
	if c.deviceID == r.macID {
		r.isActive = false
	}
}

func (r *Room) getPeer(deviceType string) *Client {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, client := range r.clients {
		if client.deviceType == deviceType {
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
		if deviceID != excludeDeviceID {
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
	r.mu.Lock()
	defer r.mu.Unlock()

	ch, exists := r.pending[ev.RequestID]
	if exists {
		delete(r.pending, ev.RequestID)
		select {
		case ch <- ev:
		default:
		}
		close(ch)
	}
	return exists
}
