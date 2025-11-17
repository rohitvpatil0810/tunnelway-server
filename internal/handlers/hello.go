package handlers

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/rohitvpatil0810/tunnelway-server/pkg/logger"
)

type HelloMessage struct {
	Message string
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func SayHello(w http.ResponseWriter, r *http.Request) {
	var log = logger.Logger()

	conn, err := upgrader.Upgrade(w, r, nil)

	if err != nil {
		log.Error("Error upgrading to websocket", slog.String("error", err.Error()))
		return
	}
	defer conn.Close()

	// echo back received messages
	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			log.Error("Error reading message", slog.String("error", err.Error()))
			break
		}

		fmt.Printf("Received: %s\\n", message)

		if err := conn.WriteMessage(websocket.TextMessage, message); err != nil {
			log.Error("Error writing message:", slog.String("error", err.Error()))
			break
		}
	}
}
