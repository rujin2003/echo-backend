package main

import (
	"encoding/json"
	"log"

	"github.com/gorilla/websocket"
)

type ClientList map[*Client]bool

type Client struct {
	conn    *websocket.Conn
	manager *Manager
	egress  chan []Event
}

func NewClient(conn *websocket.Conn, manager *Manager) *Client {
	return &Client{
		conn:    conn,
		manager: manager,
		egress:  make(chan []Event),
	}
}

func (c *Client) readMessages() {
	for {
		_, payLoad, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				c.manager.removeClient(c)
				break
			}

		}
		var request Event
		if err := json.Unmarshal(payLoad, &request); err != nil {
			log.Printf("Error unmarshaling event: %v", err)
		}

		if err := c.manager.routeEvent(request, c); err != nil {
			log.Printf("Error routing event: %v", err)
		}

	}
}

func (c *Client) writeMessages() {
	for {
		select {
		case message, ok := <-c.egress:
			if !ok {

				return
			}
			data, err := json.Marshal(message)
			if err != nil {
				log.Println("Error marshaling message:", err)
			}

			if err := c.conn.WriteMessage(websocket.TextMessage, data); err != nil {
				log.Printf("Error writing message to client: %v", err)
				c.manager.removeClient(c)
			}
			log.Printf("Sent message to client: %v", string(data))
		}
	}
}
