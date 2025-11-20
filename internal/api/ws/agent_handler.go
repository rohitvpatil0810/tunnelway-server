package ws

import (
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/rohitvpatil0810/tunnelway-server/internal/tunnel"
	"github.com/rohitvpatil0810/tunnelway-server/pkg/logger"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

var log = logger.Logger()

func RegisterAgent(w http.ResponseWriter, r *http.Request, manager *tunnel.Manager) {

	if manager == nil {
		log.Error("tunnel manager is null")
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)

	if err != nil {
		log.Error(err.Error())
		return
	}

	manager.RegisterConnection(conn)
}
