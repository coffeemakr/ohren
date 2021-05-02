package websocket

import (
	"bytes"
	"encoding/json"
	"github.com/coffeemakr/ohren"
	"github.com/gorilla/websocket"
	"log"
	"net/http"
	"time"
)

type WebsocketClient struct {
	Connection *websocket.Conn
	Send       chan ohren.Record
	OnClose    func(client *WebsocketClient)
}

const (
	// Time allowed to write a Record to the peer.
	writeWait = 10 * time.Second

	// Time allowed to read the next pong Record from the peer.
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10

	// Maximum Record size allowed from peer.
	maxMessageSize = 512
)

func (c *WebsocketClient) readPump() {
	defer func() {
		c.OnClose(c)
		c.Connection.Close()
	}()
	c.Connection.SetReadLimit(maxMessageSize)
	c.Connection.SetReadDeadline(time.Now().Add(pongWait))
	c.Connection.SetPongHandler(func(string) error {
		c.Connection.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})
	for {
		messageType, message, err := c.Connection.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("error: %v", err)
			}
			break
		}
		if messageType == websocket.TextMessage {
			var clientMessage ClientMessage
			decoder := json.NewDecoder(bytes.NewReader(message))
			decoder.Decode(&clientMessage)
		}
	}
}

type jsonDetails struct {
	Type          string    `json:"type"`
	Description   string    `json:"description"`
	Hosts         []string  `json:"hosts"`
	StartTime     time.Time `json:"start_time"`
	RemoteAddress string    `json:"client_address"`
	RemotePort    int       `json:"client_port"`
	LocalAddress  string    `json:"local_address"`
	LocalPort     int       `json:"local_port"`
}

func (c WebsocketClient) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		close(c.Send)
		c.Connection.Close()
	}()
	for {
		select {
		case message, ok := <-c.Send:
			err := c.Connection.SetWriteDeadline(time.Now().Add(writeWait))
			if err != nil {
				return
			}
			if !ok {
				// The hub closed the channel.
				_ = c.Connection.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			details := message.Details
			if details != nil {
				jsonOutput := &jsonDetails{
					RemoteAddress: message.RemoteAddress,
					RemotePort:    message.RemotePort,
					LocalPort:     message.LocalPort,
					LocalAddress:  message.LocalAddress,
					Type:          string(details.Type()),
					Description:   details.Describe(),
					Hosts:         details.Hosts(),
					StartTime:     message.StartTime,
				}
				err = c.Connection.WriteJSON(jsonOutput)
				if err != nil {
					return
				}
			} else {
				log.Println("details are nil")
				continue
			}

		case <-ticker.C:
			if err := c.Connection.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
				return
			}
			if err := c.Connection.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

type websocketHandler struct {
	newClients    chan *WebsocketClient
	closedClients chan *WebsocketClient
	broadcast     chan ohren.Record
	clients       map[*WebsocketClient]bool
}

func NewWebsocketHandler(broadcast chan ohren.Record) *websocketHandler {
	handler := new(websocketHandler)
	handler.broadcast = broadcast
	handler.newClients = make(chan *WebsocketClient)
	handler.closedClients = make(chan *WebsocketClient)
	handler.clients = make(map[*WebsocketClient]bool)
	return handler
}

func (ws websocketHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var upgrader = websocket.Upgrader{}
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Print("upgrade:", err)
		return
	}
	client := new(WebsocketClient)
	client.Send = make(chan ohren.Record)
	client.Connection = c
	client.OnClose = ws.unregisterClient
	ws.newClients <- client
	go client.writePump()
	go client.readPump()
}

func (ws websocketHandler) RunBroadcast() {
	for {
		select {
		case client := <-ws.newClients:
			ws.clients[client] = true
		case client := <-ws.closedClients:
			if _, ok := ws.clients[client]; ok {
				delete(ws.clients, client)
			}
		case message, ok := <-ws.broadcast:
			if !ok {
				log.Println("broadcast channel was closed")
				break
			}
			for client := range ws.clients {
				select {
				case client.Send <- message:
				default:
					close(client.Send)
					delete(ws.clients, client)
				}
			}
		}
	}
}

func (ws websocketHandler) unregisterClient(client *WebsocketClient) {
	ws.closedClients <- client
}
