package main

import (
	"encoding/json"
)

// const(
//
//	Device
//
// )
const (
	EventDeviceInfo      = "device_info"
	EventBatteryUpdate   = "battery_update"
	EventDownloadsUpdate = "downloads_update"
	EventAction          = "action"
	EventSubscribe       = "subscribe"
)

type Event struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

type EventHandler func(event Event, c *Client) error

type SendMessageEvent struct {
	Message string `json:"message"`
	From    string `json:"from"`
}
