package httprouter

import (
	"net/http"

	"github.com/go-chi/chi"
	"github.com/rohitvpatil0810/tunnelway-server/internal/tunnel"
)

func RegisterRoutes(r *chi.Mux, m *tunnel.Manager) {
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		m.HandlePublicTunnelRequest(w, r)
	})
}
