package main

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi"
	httprouter "github.com/rohitvpatil0810/tunnelway-server/internal/api/http-router"
	"github.com/rohitvpatil0810/tunnelway-server/internal/api/ws"
	"github.com/rohitvpatil0810/tunnelway-server/internal/tunnel"
	"github.com/rohitvpatil0810/tunnelway-server/pkg/logger"
)

func main() {
	var log = logger.Logger()
	var r *chi.Mux = chi.NewRouter()
	manager := tunnel.NewManager()

	httprouter.RegisterRoutes(r, manager)

	r.HandleFunc("/_ws/agent", func(w http.ResponseWriter, r *http.Request) {
		ws.RegisterAgent(w, r, manager)
	})

	fmt.Println("Starting Tunnelway Server...")

	err := http.ListenAndServe("localhost:8080", r)

	if err != nil {
		log.Error(err.Error())
	}
}
