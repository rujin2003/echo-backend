package main

import (
	"fmt"
	"net/http"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{}

type Manager struct {
	clients ClientList
	handler map[string]EventHandler
}

func NewManager() *Manager {
	m := &Manager{
		clients: make(ClientList),
		handler: make(map[string]EventHandler),
	}
	m.setupEventHandlers()
	return m
}


func (m *Manager) setupEventHandlers() {
	m.handler[EventDeviceInfo] = handleDeviceInfo
	m.handler[EventBatteryUpdate] = handleBatteryUpdate
	m.handler[EventAction] = handleActions
	m.handler[EventDownloadsUpdate] = handleDownloads
	m.handler[EventSubscribe] = handleSubscribe
}


func (m *Manager) routeEvent(event Event, c *Client) error {
	if handler, ok := m.handler[event.Type]; ok {
		return handler(event, c)
	}
	return fmt.Errorf("no handler for event type: %s", event.Type)
}

func (m *Manager) serveWs(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		http.Error(w, "Failed to upgrade connection", http.StatusInternalServerError)
		return
	}
	client := NewClient(conn, m)

	m.addClient(client)
	go client.readMessages()
	go client.writeMessages()
}

func (m *Manager) addClient(client *Client) {
	m.clients[client] = true
}

func (m *Manager) removeClient(client *Client) {
	if _, ok := m.clients[client]; ok {
		delete(m.clients, client)
	}

}

