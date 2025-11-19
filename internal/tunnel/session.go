package tunnel

import "github.com/gorilla/websocket"

type Session struct {
	conn *websocket.Conn
}
