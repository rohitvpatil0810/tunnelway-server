package main

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi"
	httprouter "github.com/rohitvpatil0810/tunnelway-server/internal/api/http-router"
	"github.com/rohitvpatil0810/tunnelway-server/internal/api/ws"
	"github.com/rohitvpatil0810/tunnelway-server/internal/config"
	"github.com/rohitvpatil0810/tunnelway-server/internal/tunnel"
	"github.com/rohitvpatil0810/tunnelway-server/pkg/logger"
)

func main() {
	var log = logger.Logger()
	cfg := config.Load()
	var r *chi.Mux = chi.NewRouter()
	manager := tunnel.NewManager(cfg.MainDomain)

	httprouter.RegisterRoutes(r, manager)

	// Websocket endpoint for agents to connect
	// also takes an optional query param identify the agent reconnection - ?agent_id=xyze
	r.HandleFunc("/_ws/agent", func(w http.ResponseWriter, r *http.Request) {
		ws.RegisterAgent(w, r, manager)
	})

	fmt.Println("Starting Tunnelway Server...")

	err := http.ListenAndServe(cfg.ListenAddr, r)

	if err != nil {
		log.Error(err.Error())
	}
}
