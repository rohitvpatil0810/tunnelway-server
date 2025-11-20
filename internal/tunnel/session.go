package tunnel

import (
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
	// TODO: Implement Tunnel Response
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
	return session
}
