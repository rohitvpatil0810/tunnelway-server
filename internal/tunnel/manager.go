package tunnel

import (
	"crypto/rand"

	"github.com/gorilla/websocket"
	"github.com/rohitvpatil0810/tunnelway-server/pkg/logger"
)

const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

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
	var session = &Session{conn}
	slug, err := m.generateSlug()
	if err != nil {
		log.Error(err.Error())
		conn.WriteMessage(websocket.TextMessage, []byte("Failed to register"))
		conn.Close()
	}
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
