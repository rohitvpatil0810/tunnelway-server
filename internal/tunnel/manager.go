package tunnel

import (
	"crypto/rand"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/rohitvpatil0810/tunnelway-server/pkg/logger"
)

const charset = "abcdefghijklmnopqrstuvwxyz0123456789"

type Manager struct {
	tunnels map[string]*Session
}

var manager *Manager
var log = logger.Logger()

func NewManager() *Manager {
	if manager == nil {
		manager = &Manager{
			tunnels: make(map[string]*Session),
		}
	}
	return manager
}

func (m *Manager) RegisterConnection(conn *websocket.Conn) string {
	slug, err := m.generateSlug()
	if err != nil {
		log.Error(err.Error())
		conn.WriteMessage(websocket.TextMessage, []byte("Failed to register"))
		conn.Close()
	}
	var session = NewSession(slug, conn)
	go session.StartWriteLoop()
	go session.StartReadLoop()

	m.tunnels[slug] = session

	return slug
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
	slug := strings.Split(requestHost, ".")[0]

	session, exists := m.tunnels[slug]
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
	session.Send <- sendReqMsg

	select {
	case resp := <-respChan:
		// for now just send whatever is received in form of TunnelResponse
		// TODO: convert TunnelResponse to
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(resp)

	case <-session.Closed:
		http.Error(w, "Tunnel closed Unexpectedly", http.StatusInternalServerError)

	case <-time.After(time.Duration(30 * time.Second)):
		http.Error(w, "Timeout.", http.StatusRequestTimeout)
	}
}
