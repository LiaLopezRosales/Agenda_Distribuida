// websocket.go
package agendadistribuida

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// =====================
// Configuración WS
// =====================

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 512
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	// En producción: restringir orígenes
	CheckOrigin: func(r *http.Request) bool { return true },
}

// =====================
// WS Manager & Client
// =====================

// WSClient representa una conexión WebSocket activa de un usuario
type WSClient struct {
	manager *WSManager
	conn    *websocket.Conn
	send    chan []byte
	userID  int64
}

// WSManager mantiene conexiones activas agrupadas por usuario
type WSManager struct {
	conns      map[int64]map[*WSClient]bool
	mux        sync.RWMutex
	register   chan *WSClient
	unregister chan *WSClient
	closed     chan struct{}
}

func NewWSManager() *WSManager {
	return &WSManager{
		conns:      make(map[int64]map[*WSClient]bool),
		register:   make(chan *WSClient),
		unregister: make(chan *WSClient),
		closed:     make(chan struct{}),
	}
}

func (m *WSManager) Run() {
	for {
		select {
		case c := <-m.register:
			m.mux.Lock()
			if _, ok := m.conns[c.userID]; !ok {
				m.conns[c.userID] = make(map[*WSClient]bool)
			}
			m.conns[c.userID][c] = true
			m.mux.Unlock()
			log.Printf("ws: user %d connected", c.userID)
		case c := <-m.unregister:
			m.mux.Lock()
			if set, ok := m.conns[c.userID]; ok {
				if _, exists := set[c]; exists {
					delete(set, c)
					close(c.send)
					if len(set) == 0 {
						delete(m.conns, c.userID)
					}
				}
			}
			m.mux.Unlock()
			log.Printf("ws: user %d disconnected", c.userID)
		case <-m.closed:
			m.mux.Lock()
			for _, set := range m.conns {
				for cl := range set {
					cl.conn.Close()
					close(cl.send)
				}
			}
			m.conns = make(map[int64]map[*WSClient]bool)
			m.mux.Unlock()
			return
		}
	}
}

func (m *WSManager) Stop() { close(m.closed) }

// =====================
// Broadcast helpers
// =====================

func (m *WSManager) BroadcastToUser(userID int64, msg interface{}) {
	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("ws: marshal error: %v", err)
		return
	}

	m.mux.RLock()
	defer m.mux.RUnlock()

	if set, ok := m.conns[userID]; ok {
		for c := range set {
			select {
			case c.send <- data:
			default:
				// canal lleno -> desconectar
				go func(cl *WSClient) {
					m.unregister <- cl
					cl.conn.Close()
				}(c)
			}
		}
	}
}

func (m *WSManager) BroadcastToUsers(userIDs []int64, msg interface{}) {
	for _, uid := range userIDs {
		m.BroadcastToUser(uid, msg)
	}
}

// =====================
// Pumps
// =====================

func (c *WSClient) readPump() {
	defer func() {
		c.manager.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadLimit(maxMessageSize)
	_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, _, err := c.conn.ReadMessage()
		if err != nil {
			break // close on error or disconnect
		}
	}
}

func (c *WSClient) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case msg, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			_, _ = w.Write(msg)

			// agrupar mensajes pendientes
			n := len(c.send)
			for i := 0; i < n; i++ {
				_, _ = w.Write([]byte{'\n'})
				_, _ = w.Write(<-c.send)
			}
			if err := w.Close(); err != nil {
				return
			}
		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// =====================
// ServeWS
// =====================

// extrae token de Authorization o query param
func extractTokenFromRequest(r *http.Request) (string, error) {
	auth := r.Header.Get("Authorization")
	if auth != "" {
		parts := strings.SplitN(auth, " ", 2)
		if len(parts) == 2 && strings.ToLower(parts[0]) == "bearer" {
			return parts[1], nil
		}
	}
	if q := r.URL.Query().Get("token"); q != "" {
		return q, nil
	}
	return "", errors.New("no token provided")
}

// ServeWS valida token, registra conexión y envía notificaciones pendientes
func ServeWS(storage *Storage, manager *WSManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tokenStr, err := extractTokenFromRequest(r)
		if err != nil {
			http.Error(w, "missing token", http.StatusUnauthorized)
			return
		}

		claims, err := ParseToken(tokenStr)
		if err != nil {
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}

		user, err := storage.GetUserByUsername(claims.Username)
		if err != nil {
			http.Error(w, "user not found", http.StatusUnauthorized)
			return
		}

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("ws upgrade error: %v", err)
			return
		}

		client := &WSClient{
			manager: manager,
			conn:    conn,
			send:    make(chan []byte, 256),
			userID:  user.ID,
		}
		manager.register <- client

		// Enviar notificaciones pendientes al conectar
		notes, err := storage.GetUserNotifications(user.ID)
		if err == nil {
			for _, n := range notes {
				payload := map[string]interface{}{
					"type":    n.Type,
					"payload": n.Payload,
					"id":      n.ID,
					"created": n.CreatedAt,
				}
				manager.BroadcastToUser(user.ID, payload)
			}
		}

		go client.writePump()
		go client.readPump()
	}
}
