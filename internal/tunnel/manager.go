package tunnel

import (
	"crypto/rand"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/rohitvpatil0810/tunnelway-server/pkg/logger"
)

const charset = "abcdefghijklmnopqrstuvwxyz0123456789"

type Manager struct {
	tunnels    map[string]*Session
	tunnelsMu  sync.RWMutex
	mainDomain string
}

var manager *Manager
var log = logger.Logger()

func NewManager(mainDomain string) *Manager {
	if strings.TrimSpace(mainDomain) == "" {
		mainDomain = "localtest.me"
	}

	if manager == nil {
		manager = &Manager{
			tunnels:    make(map[string]*Session),
			mainDomain: mainDomain,
		}
	} else {
		manager.mainDomain = mainDomain
	}

	return manager
}

func (m *Manager) RegisterConnection(conn *websocket.Conn, agentID string) {
	// check if agentID is not empty, if yes then its a reconnection attempt, try to find the session and update the connection
	// if session not found create session with same agentId as slug, this will allow users to specify their own subdomain by setting agentID to desired subdomain,
	// if agentID is empty then generate random slug as before
	if agentID != "" {
		m.tunnelsMu.RLock()
		session := m.tunnels[agentID]
		m.tunnelsMu.RUnlock()
		if session != nil {
			log.Info("Found existing session for agentID, updating connection", "agent_id", agentID)
			session.Reconnect(conn)

			conn.WriteJSON(map[string]string{"reconnected": "true", "subdomain": session.Subdomain})
			return
		}
	}

	var slug string
	var err error
	if agentID == "" {
		slug, err = m.generateSlug()
		if err != nil {
			log.Error(err.Error())
			conn.WriteMessage(websocket.CloseMessage, []byte("Failed to register"))
			conn.Close()
			return
		}
	} else {
		slug = agentID
	}
	var session = NewSession(slug, m.mainDomain, conn)

	m.tunnelsMu.Lock()
	m.tunnels[slug] = session
	m.tunnelsMu.Unlock()

	log.Info("New agent connected", "subdomain", session.Subdomain)
}

func (m *Manager) generateSlug() (string, error) {
	for {
		slug, err := generateRandomString(4)
		if err != nil {
			log.Error(err.Error())
			return "", err
		}

		if _, exists := m.tunnels[slug]; !exists {
			return slug, nil
		}
	}
}

func generateRandomString(length int) (string, error) {
	b := make([]byte, length)
	randBytes := make([]byte, length)

	_, err := rand.Read(randBytes)
	if err != nil {
		return "", err
	}

	for i := range b {
		b[i] = charset[int(randBytes[i])%len(charset)]
	}
	return string(b), nil
}

func (m *Manager) HandlePublicTunnelRequest(w http.ResponseWriter, r *http.Request) {
	requestHost := r.Host
	if host, _, err := net.SplitHostPort(r.Host); err == nil {
		requestHost = host
	}

	slug := strings.Split(requestHost, ".")[0]
	if suffix := "." + m.mainDomain; strings.HasSuffix(requestHost, suffix) {
		slug = strings.TrimSuffix(requestHost, suffix)
	}

	m.tunnelsMu.RLock()
	session, exists := m.tunnels[slug]
	m.tunnelsMu.RUnlock()
	if !exists {
		http.Error(w, "Tunnel is not connected.", http.StatusNotFound)
		return
	}

	// read body bytes
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to ready body", http.StatusBadRequest)
		return
	}
	r.Body.Close()

	requestId := uuid.New().String()
	var respChan chan *TunnelResponse = make(chan *TunnelResponse, 1)
	session.PendingMu.Lock()
	session.Pending[requestId] = respChan
	session.PendingMu.Unlock()

	// send request to the send channel
	tunnelRequest := TunnelRequest{
		ID:      requestId,
		Method:  r.Method,
		Path:    r.URL.Path,
		Headers: r.Header,
		Body:    bodyBytes,
	}
	sendReqMsg, _ := json.Marshal(tunnelRequest)
	closed, err := session.SendToAgent(sendReqMsg)
	if err != nil {
		http.Error(w, "Tunnel closed Unexpectedly", http.StatusInternalServerError)
		return
	}

	select {
	case resp := <-respChan:
		for key, values := range resp.Headers {
			for _, value := range values {
				w.Header().Add(key, value)
			}
		}
		w.WriteHeader(resp.Status)
		w.Write([]byte(resp.Body))

	case <-closed:
		http.Error(w, "Tunnel closed Unexpectedly", http.StatusInternalServerError)

	case <-time.After(time.Duration(30 * time.Second)):
		http.Error(w, "Timeout.", http.StatusRequestTimeout)
	}
}
