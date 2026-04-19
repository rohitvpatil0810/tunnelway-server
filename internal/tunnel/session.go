package tunnel

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type Session struct {
	ID        string
	Subdomain string

	PendingMu sync.Mutex
	Pending   map[string]chan *TunnelResponse

	LastSeen time.Time

	stateMu sync.RWMutex
	state   *connectionState
}

type connectionState struct {
	conn      *websocket.Conn
	send      chan []byte
	closed    chan struct{}
	stop      chan struct{}
	closeOnce sync.Once
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
		Pending:   make(map[string]chan *TunnelResponse),
		LastSeen:  time.Now(),
	}

	state := &connectionState{
		conn:   conn,
		send:   make(chan []byte, 128),
		closed: make(chan struct{}),
		stop:   make(chan struct{}),
	}
	session.state = state

	conn.WriteJSON(map[string]string{"subdomain": session.Subdomain})
	go session.StartWriteLoop(state)
	go session.StartReadLoop(state)

	return session
}

func (session *Session) currentState() *connectionState {
	session.stateMu.RLock()
	defer session.stateMu.RUnlock()
	return session.state
}

func (session *Session) setState(state *connectionState) {
	session.stateMu.Lock()
	session.state = state
	session.stateMu.Unlock()
}

func (session *Session) signalClosed(state *connectionState) {
	state.closeOnce.Do(func() {
		close(state.stop)
		close(state.closed)
		_ = state.conn.Close()
	})
}

func (session *Session) Reconnect(conn *websocket.Conn) {
	previous := session.currentState()
	if previous != nil {
		session.signalClosed(previous)
	}

	newState := &connectionState{
		conn:   conn,
		send:   make(chan []byte, 128),
		closed: make(chan struct{}),
		stop:   make(chan struct{}),
	}
	session.setState(newState)
	session.LastSeen = time.Now()

	go session.StartWriteLoop(newState)
	go session.StartReadLoop(newState)
}

func (session *Session) SendToAgent(msg []byte) (<-chan struct{}, error) {
	state := session.currentState()
	if state == nil {
		return nil, errors.New("session has no active connection")
	}

	select {
	case <-state.closed:
		return nil, errors.New("session connection is closed")
	case state.send <- msg:
		return state.closed, nil
	}
}

func (session *Session) StartReadLoop(state *connectionState) {
	defer session.signalClosed(state)

	for {
		_, msg, err := state.conn.ReadMessage()
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

func (session *Session) StartWriteLoop(state *connectionState) {
	defer session.signalClosed(state)

	for {
		select {
		case <-state.stop:
			return
		case msg := <-state.send:
			if err := state.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				log.Error("write error", slog.String("error", err.Error()))
				return
			}
		}
	}
}
