package main

import (
	"encoding/json"
	"time"
)

const (
	// Connection events
	EventConnect    = "connect"
	EventDisconnect = "disconnect"

	// Room events
	EventCreateRoom = "create_room"
	EventJoinRoom   = "join_room"
	EventLeaveRoom  = "leave_room"
	EventRoomJoined = "room_joined"
	EventRoomStatus = "room_status"
	// Data sync events
	EventDeviceInfo      = "device_info"
	EventBatteryUpdate   = "battery_update"
	EventDownloadsUpdate = "downloads_update"
	EventStorageUpdate   = "storage_update"

	// Action events
	EventAction        = "action"
	EventActionRequest = "action_request"
	EventActionResult  = "action_result"

	// Media Action
	EventMediaAction        = "media_action"
	EventMediaActionRequest = "media_action_request"
	EventMediaActionResult  = "media_action_result"

	// Generic request/response
	EventRequest  = "request"
	EventResponse = "response"
	EventError    = "error"

	// Peer events
	EventPeerConnected    = "peer_connected"
	EventPeerDisconnected = "peer_disconnected"

	// Status events
	EventStatusUpdate = "status_update"
)

type Event struct {
	Type      string          `json:"type"`
	RoomID    string          `json:"room_id,omitempty"`
	DeviceID  string          `json:"device_id,omitempty"`
	RequestID string          `json:"request_id,omitempty"`
	Timestamp time.Time       `json:"timestamp"`
	Payload   json.RawMessage `json:"payload,omitempty"`
}
