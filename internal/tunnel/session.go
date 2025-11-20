package tunnel

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type Session struct {
	ID        string
	Subdomain string
	conn      *websocket.Conn

	Send chan []byte

	PendingMu sync.Mutex
	Pending   map[string]chan *TunnelResponse

	LastSeen time.Time

	Closed chan struct{}
}

type TunnelResponse struct {
	ID      string
	Status  int
	Headers http.Header
	// Body    []byte `json:"Body"`
	// TODO: change to byte again - for testing changed to string
	Body string `json:"Body"`
}

type TunnelRequest struct {
	ID      string
	Method  string
	Path    string
	Headers http.Header
	Body    []byte
}

func NewSession(slug string, conn *websocket.Conn) *Session {
	session := &Session{
		ID:        "agent-" + slug,
		Subdomain: slug + ".localtest.me",
		conn:      conn,
		// make buffer of 128, to allow up to 128 messages to b queued without blocking the sender
		Send:     make(chan []byte, 128),
		Pending:  make(map[string]chan *TunnelResponse),
		LastSeen: time.Now(),
		Closed:   make(chan struct{}),
	}
	conn.WriteJSON(session.Subdomain)
	return session
}

func (session *Session) StartReadLoop() {
	defer close(session.Closed)

	for {
		_, msg, err := session.conn.ReadMessage()
		if err != nil {
			log.Error(err.Error())
			return
		}
		// log.Info("Type of Message", slog.Int("typeMsg", typeMsg), slog.String("msg", string(msg)))
		responseJson := &TunnelResponse{}
		if err := json.Unmarshal(msg, responseJson); err != nil {
			log.Error("Error unmarshalling json", slog.String("error", err.Error()))
			continue
		}
		session.PendingMu.Lock()
		respCh, exists := session.Pending[responseJson.ID]
		if exists {
			delete(session.Pending, responseJson.ID)
		}
		session.PendingMu.Unlock()

		if exists {
			respCh <- responseJson
		} else {
			log.Error("Received response for unknown request ID", slog.String("id", responseJson.ID))
		}

	}
}

func (session *Session) StartWriteLoop() {
	for msg := range session.Send {
		if err := session.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
			log.Error("write error", slog.String("error", err.Error()))
			return
		}
	}
}
