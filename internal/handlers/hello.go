package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/rohitvpatil0810/tunnelway-server/pkg/logger"
)

type HelloMessage struct {
	Message string
}

func SayHello(w http.ResponseWriter, r *http.Request) {
	var log = logger.Logger()

	log.Info("First Api Hit: \"/\" (SayHello)")
	var helloMessage = HelloMessage{
		Message: "Hello From Tunnelway Server...",
	}
	w.Header().Set("Content-Type", "application/json")
	err := json.NewEncoder(w).Encode(helloMessage)
	if err != nil {
		log.Error(err.Error())
		return
	}

}
