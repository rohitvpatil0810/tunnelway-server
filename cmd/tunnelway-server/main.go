package main

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi"
	"github.com/rohitvpatil0810/tunnelway-server/internal/handlers"
	"github.com/rohitvpatil0810/tunnelway-server/pkg/logger"
)

func main() {
	var log = logger.Logger()
	var r *chi.Mux = chi.NewRouter()
	handlers.Handler(r)

	fmt.Println("Starting Tunnelway Server...")

	err := http.ListenAndServe("localhost:8080", r)

	if err != nil {
		log.Error(err.Error())
	}
}
