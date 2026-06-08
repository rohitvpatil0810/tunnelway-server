package tunnel

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type Session struct {
	ID        string
	Subdomain string

	PendingMu sync.Mutex
	Pending   map[string]*ResponseStream

	LastSeen time.Time

	stateMu sync.RWMutex
	state   *connectionState
}

type connectionState struct {
	conn      *websocket.Conn
	send      chan []byte
	closed    chan struct{}
	stop      chan struct{}
	closeOnce sync.Once
}

type FrameType int

const (
	FrameRequestStart FrameType = iota
	FrameRequestBodyChunk
	FrameRequestBodyEnd
	FrameResponseStart
	FrameResponseBodyChunk
	FrameResponseBodyEnd
)

type Frame struct {
	Type      FrameType
	RequestID string
	Data      []byte
}

type ResponseStream struct {
	responseStart chan ResponseStart

	PipeReader *io.PipeReader
	PipeWriter *io.PipeWriter
}

type ResponseStart struct {
	StatusCode int
	Headers    http.Header
}

func NewSession(slug string, mainDomain string, conn *websocket.Conn) *Session {
	session := &Session{
		ID:        "agent-" + slug,
		Subdomain: slug + "." + mainDomain,
		Pending:   make(map[string]*ResponseStream),
		LastSeen:  time.Now(),
	}

	state := &connectionState{
		conn:   conn,
		send:   make(chan []byte, 128),
		closed: make(chan struct{}),
		stop:   make(chan struct{}),
	}
	session.state = state

	conn.WriteJSON(map[string]string{"subdomain": session.Subdomain})
	go session.StartWriteLoop(state)
	go session.StartReadLoop(state)

	return session
}

func (session *Session) currentState() *connectionState {
	session.stateMu.RLock()
	defer session.stateMu.RUnlock()
	return session.state
}

func (session *Session) setState(state *connectionState) {
	session.stateMu.Lock()
	session.state = state
	session.stateMu.Unlock()
}

func (session *Session) signalClosed(state *connectionState) {
	state.closeOnce.Do(func() {
		close(state.stop)
		close(state.closed)
		_ = state.conn.Close()
	})
}

func (session *Session) Reconnect(conn *websocket.Conn) {
	previous := session.currentState()
	if previous != nil {
		session.signalClosed(previous)
	}

	newState := &connectionState{
		conn:   conn,
		send:   make(chan []byte, 128),
		closed: make(chan struct{}),
		stop:   make(chan struct{}),
	}
	session.setState(newState)
	session.LastSeen = time.Now()

	go session.StartWriteLoop(newState)
	go session.StartReadLoop(newState)
}

func (session *Session) SendFrame(frame *Frame) (<-chan struct{}, error) {
	state := session.currentState()
	if state == nil {
		return nil, errors.New("Session has no active connection")
	}

	encoded, err := encodeFrame(frame)
	if err != nil {
		return nil, err
	}

	select {
	case <-state.closed:
		return nil, errors.New("Session connection is closed")
	case state.send <- encoded:
		return state.closed, nil
	}
}

func encodeFrame(frame *Frame) ([]byte, error) {
	buf := bytes.NewBuffer(nil)

	// 1 byte for frame type
	buf.WriteByte(byte(frame.Type))

	// request ID length + value
	idBytes := []byte(frame.RequestID)
	if len(idBytes) > 255 {
		return nil, errors.New("Request ID too long")
	}
	buf.WriteByte(byte(len(idBytes)))
	buf.Write(idBytes)

	// payload length (4 bytes)
	binary.Write(buf, binary.BigEndian, uint32(len(frame.Data)))
	buf.Write(frame.Data)

	return buf.Bytes(), nil
}

func (session *Session) StreamRequestToAgent(req *http.Request, requestID string) error {

	// 1. send start frame with headers and metadata
	startFrameData, _ := json.Marshal(map[string]interface{}{
		"method":  req.Method,
		"url":     req.URL.RequestURI(),
		"headers": req.Header,
	})

	_, err := session.SendFrame(&Frame{
		Type:      FrameRequestStart,
		RequestID: requestID,
		Data:      startFrameData,
	})
	if err != nil {
		return err
	}

	// 2. stream body in chunks
	buf := make([]byte, 1024*32) // 32KB buffer
	for {
		n, err := req.Body.Read(buf)
		if n > 0 {
			chunkData := buf[:n]
			_, err := session.SendFrame(&Frame{
				Type:      FrameRequestBodyChunk,
				RequestID: requestID,
				Data:      append([]byte(nil), chunkData...), // copy to avoid reuse
			})
			if err != nil {
				return err
			}
		}

		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}

	// 3. send end frame
	_, err = session.SendFrame(&Frame{
		Type:      FrameRequestBodyEnd,
		RequestID: requestID,
		Data:      nil,
	})

	return err
}

func (session *Session) StartWriteLoop(state *connectionState) {
	defer session.signalClosed(state)

	for {
		select {
		case <-state.stop:
			return
		case msg := <-state.send:
			if err := state.conn.WriteMessage(websocket.BinaryMessage, msg); err != nil {
				log.Error("write error", slog.String("error", err.Error()))
				return
			}
		}
	}
}

func decodeFrame(data []byte) (*Frame, error) {
	reader := bytes.NewReader(data)

	// 1 byte for frame type
	frameTypeByte, err := reader.ReadByte()
	if err != nil {
		return nil, err
	}

	idLen, err := reader.ReadByte()
	if err != nil {
		return nil, err
	}

	idBytes := make([]byte, idLen)
	if _, err := io.ReadFull(reader, idBytes); err != nil {
		return nil, err
	}

	var payloadLen uint32
	if err := binary.Read(reader, binary.BigEndian, &payloadLen); err != nil {
		return nil, err
	}

	payload := make([]byte, payloadLen)
	if _, err := io.ReadFull(reader, payload); err != nil {
		return nil, err
	}

	return &Frame{
		Type:      FrameType(frameTypeByte),
		RequestID: string(idBytes),
		Data:      payload,
	}, nil
}

func (session *Session) handleResponseFrame(frame *Frame) {
	switch frame.Type {
	case FrameResponseStart:
		var meta struct {
			StatusCode int
			Headers    http.Header
		}

		if err := json.Unmarshal(frame.Data, &meta); err != nil {
			log.Error("Failed to unmarshal response metadata", slog.String("error", err.Error()))
			return
		}

		session.PendingMu.Lock()
		responseStream, exists := session.Pending[frame.RequestID]
		session.PendingMu.Unlock()

		if exists {
			responseStream.responseStart <- ResponseStart{
				StatusCode: meta.StatusCode,
				Headers:    meta.Headers,
			}
		} else {
			log.Debug("Received response start for unknown request ID", slog.String("requestID", frame.RequestID))
		}

	case FrameResponseBodyChunk:
		session.PendingMu.Lock()
		responseStream, exists := session.Pending[frame.RequestID]
		session.PendingMu.Unlock()

		if exists {
			_, err := responseStream.PipeWriter.Write(frame.Data)
			if err != nil {
				log.Error("Failed to write response body chunk", slog.String("error", err.Error()))
			}
		} else {
			log.Debug("Received response body chunk for unknown request ID", slog.String("requestID", frame.RequestID))
		}

	case FrameResponseBodyEnd:
		session.PendingMu.Lock()
		responseStream, exists := session.Pending[frame.RequestID]
		if exists {
			delete(session.Pending, frame.RequestID)
		} else {
			log.Debug("Received response body end for unknown request ID", slog.String("requestID", frame.RequestID))
		}
		session.PendingMu.Unlock()

		if exists {
			responseStream.PipeWriter.Close()
		}
	default:
		log.Error("Received unknown frame type", slog.Int("type", int(frame.Type)))
	}
}

func (session *Session) StartReadLoop(state *connectionState) {
	defer session.signalClosed(state)

	for {
		_, msg, err := state.conn.ReadMessage()
		if err != nil {
			log.Error(err.Error())
			return
		}

		frame, err := decodeFrame(msg)
		if err != nil {
			log.Error("Failed to decode frame", slog.String("error", err.Error()))
			continue
		}

		session.handleResponseFrame(frame)
	}
}
