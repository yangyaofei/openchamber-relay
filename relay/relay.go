package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

const (
	// protocolVersion is the handshake version both peers must send as ?v=.
	protocolVersion = 1

	writeWait    = 120 * time.Second
	pingInterval = 30 * time.Second
	pongWait     = 90 * time.Second

	// hostControlGrace is how long client slots survive after the host
	// control socket drops, to allow the host to reconnect without
	// tearing down every remote client.
	hostControlGrace = 60 * time.Second

	// dataSocketIdleMax is the inactivity timeout for a paired data socket.
	dataSocketIdleMax = 300 * time.Second
	// dataSocketUnpaired is how long a client socket may wait for the
	// host to open its matching host-data socket.
	dataSocketUnpaired = 60 * time.Second

	// maxMessageSize caps a single forwarded frame (64 MB, matching the
	// largest attachment payloads OpenChamber syncs).
	maxMessageSize = 64 * 1024 * 1024
	// maxClientsPerHost caps remote clients per serverId.
	maxClientsPerHost = 128
)

// relayMessage is a control-frame sent to the host control socket.
type relayMessage struct {
	Type          string   `json:"type"`
	ConnectionId  string   `json:"connectionId,omitempty"`
	ConnectionIds []string `json:"connectionIds"`
	Reason        string   `json:"reason,omitempty"`
}

// clientSlot tracks one remote client connection and its paired host data
// socket. A slot is created when the client connects and finished once the
// pair is torn down.
type clientSlot struct {
	clientWs   *websocket.Conn
	hostDataWs *websocket.Conn
	createdAt  time.Time
	lastActive time.Time
	done       chan struct{}
	closeOnce  sync.Once
	mu         sync.Mutex
}

// finish marks the slot as closed; safe to call multiple times.
func (slot *clientSlot) finish() {
	slot.closeOnce.Do(func() { close(slot.done) })
}

// serverState is everything the relay knows about one host serverId.
type serverState struct {
	id        string
	controlWs *websocket.Conn
	controlWg sync.Mutex // serializes writes on controlWs
	clients   map[string]*clientSlot
	hostGen   uint64 // bumped on every host reconnect; guards stale drains
	mu        sync.Mutex
}

// relay is the root state: one serverState per connected host serverId.
type relay struct {
	servers    map[string]*serverState
	mu         sync.RWMutex
	verifyAuth bool
}

func newRelay(verifyAuth bool) *relay {
	return &relay{
		servers:    make(map[string]*serverState),
		verifyAuth: verifyAuth,
	}
}

func (r *relay) getOrCreateServer(serverId string) *serverState {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.servers[serverId]
	if !ok {
		s = &serverState{
			id:      serverId,
			clients: make(map[string]*clientSlot),
		}
		r.servers[serverId] = s
	}
	return s
}

func (r *relay) getServer(serverId string) *serverState {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.servers[serverId]
}

// maybeRemoveServer removes a server entry if it has no host and no clients,
// keeping the map from growing with stale serverIds.
func (r *relay) maybeRemoveServer(serverId string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.servers[serverId]
	if !ok {
		return
	}
	s.mu.Lock()
	empty := s.controlWs == nil && len(s.clients) == 0
	s.mu.Unlock()
	if empty {
		delete(r.servers, serverId)
	}
}

// sendControl writes one control message to the host control socket.
// Multiple goroutines (handleClient, forwardPair, sweeper) call this, so
// writes are serialized under controlWg. Returns false when the host is
// gone. A failed write is fatal for the control socket: it is logged and
// the socket closed, so handleHostControl's reader exits and the grace
// path takes over — never a silent zombie that eats notifications.
func (s *serverState) sendControl(msg relayMessage) bool {
	s.mu.Lock()
	conn := s.controlWs
	s.mu.Unlock()
	if conn == nil {
		return false
	}
	data, _ := json.Marshal(msg)
	s.controlWg.Lock()
	defer s.controlWg.Unlock()
	conn.SetWriteDeadline(time.Now().Add(writeWait))
	if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
		log.Printf("[relay] control write failed serverId=%s type=%s peer=%s: %v",
			s.id, msg.Type, conn.RemoteAddr(), err)
		conn.Close()
		return false
	}
	return true
}

// handleWebSocket upgrades /ws and dispatches on ?role=.
// Required params: v (protocol version), serverId, role.
// Optional auth params (when RELAY_VERIFY_AUTH=true): ts, sig, pk.
func (r *relay) handleWebSocket(w http.ResponseWriter, req *http.Request) {
	q := req.URL.Query()
	version := q.Get("v")
	role := q.Get("role")
	serverId := q.Get("serverId")

	if version != fmt.Sprintf("%d", protocolVersion) || serverId == "" || role == "" {
		http.Error(w, "missing required params", http.StatusBadRequest)
		return
	}

	conn, err := upgrader.Upgrade(w, req, nil)
	if err != nil {
		log.Printf("[relay] upgrade error: %v", err)
		return
	}
	conn.SetReadLimit(maxMessageSize)

	switch role {
	case "host-control":
		if r.verifyAuth && !verifyAuth(serverId, role, "", q.Get("ts"), q.Get("sig"), q.Get("pk")) {
			conn.WriteMessage(websocket.CloseMessage,
				websocket.FormatCloseMessage(4010, "auth failed"))
			conn.Close()
			return
		}
		r.handleHostControl(serverId, conn)

	case "host-data":
		connectionId := q.Get("connectionId")
		if connectionId == "" {
			conn.Close()
			return
		}
		if r.verifyAuth && !verifyAuth(serverId, role, connectionId, q.Get("ts"), q.Get("sig"), q.Get("pk")) {
			conn.WriteMessage(websocket.CloseMessage,
				websocket.FormatCloseMessage(4010, "auth failed"))
			conn.Close()
			return
		}
		r.handleHostData(serverId, connectionId, conn)

	case "client":
		r.handleClient(serverId, conn, q.Get("grant"))

	default:
		conn.Close()
	}
}

// handleHostControl registers the host control socket for serverId.
//
// Behavior:
//   - Replaces any previous control socket (close 4001 "Control replaced").
//   - Sends a "sync" message listing clients that outlived a reconnect.
//   - Keeps the socket alive with WebSocket pings.
//   - On disconnect, waits hostControlGrace for a reconnect before draining
//     every client slot; a host generation bump cancels the drain.
func (r *relay) handleHostControl(serverId string, conn *websocket.Conn) {
	s := r.getOrCreateServer(serverId)

	s.mu.Lock()
	oldControl := s.controlWs
	s.controlWs = conn
	s.hostGen++
	myGen := s.hostGen
	currentClients := make([]string, 0, len(s.clients))
	for cid := range s.clients {
		currentClients = append(currentClients, cid)
	}
	s.mu.Unlock()

	if oldControl != nil {
		// WriteControl is safe to call concurrently with in-flight
		// WriteMessage on the same conn (gorilla contract), unlike
		// WriteMessage which would corrupt the frame stream.
		oldControl.WriteControl(websocket.CloseMessage,
			websocket.FormatCloseMessage(4001, "Control replaced"),
			time.Now().Add(writeWait))
		oldControl.Close()
	}

	s.sendControl(relayMessage{Type: "sync", ConnectionIds: currentClients})

	log.Printf("[relay] host-control connected %s gen=%d peer=%s (%d pending clients)",
		serverId, myGen, conn.RemoteAddr(), len(currentClients))

	conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	done := make(chan struct{})

	// Drain reads; control messages flow host→relay only via this socket's
	// liveness (the host does not send app-level frames here).
	go func() {
		defer close(done)
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				return
			}
		}
	}()

	go func() {
		ticker := time.NewTicker(pingInterval)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				// WriteControl for pings: gorilla guarantees it may run
				// concurrently with the WriteMessage in sendControl. Using
				// WriteMessage here instead raced with control messages and
				// corrupted the outgoing frame stream (interleaved frames the
				// host either dropped the connection on, or worse, silently
				// lost a "connected" notification to).
				if err := conn.WriteControl(websocket.PingMessage, nil,
					time.Now().Add(writeWait)); err != nil {
					return
				}
			}
		}
	}()

	<-done

	s.mu.Lock()
	if s.controlWs == conn {
		s.controlWs = nil
	}
	s.mu.Unlock()

	conn.Close()

	log.Printf("[relay] host-control disconnected %s gen=%d, grace %v", serverId, myGen, hostControlGrace)

	timer := time.NewTimer(hostControlGrace)
	defer timer.Stop()
	<-timer.C

	// Only drain if no new host connected during grace.
	s.mu.Lock()
	if s.controlWs != nil {
		s.mu.Unlock()
		log.Printf("[relay] host reconnected during grace %s, skipping drain", serverId)
		return
	}
	if s.hostGen != myGen {
		s.mu.Unlock()
		log.Printf("[relay] host gen changed during grace %s, skipping drain", serverId)
		return
	}
	for cid, slot := range s.clients {
		log.Printf("[relay] draining client %s/%s (host went away)", serverId, cid)
		slot.mu.Lock()
		if slot.clientWs != nil {
			slot.clientWs.WriteMessage(websocket.CloseMessage,
				websocket.FormatCloseMessage(1012, "Host went away"))
			slot.clientWs.Close()
		}
		if slot.hostDataWs != nil {
			slot.hostDataWs.Close()
		}
		slot.mu.Unlock()
		slot.finish()
	}
	s.clients = make(map[string]*clientSlot)
	s.mu.Unlock()

	r.maybeRemoveServer(serverId)
}

// handleHostData pairs the host-side data socket with an existing client
// slot and starts bidirectional forwarding.
func (r *relay) handleHostData(serverId, connectionId string, conn *websocket.Conn) {
	s := r.getOrCreateServer(serverId)
	s.mu.Lock()
	slot, ok := s.clients[connectionId]
	if !ok {
		s.mu.Unlock()
		log.Printf("[relay] host-data for unknown client %s/%s", serverId, connectionId)
		conn.Close()
		return
	}

	slot.mu.Lock()
	if slot.hostDataWs != nil {
		slot.mu.Unlock()
		s.mu.Unlock()
		log.Printf("[relay] duplicate host-data for %s/%s", serverId, connectionId)
		conn.Close()
		return
	}
	slot.hostDataWs = conn
	slot.lastActive = time.Now()
	clientWs := slot.clientWs
	slot.mu.Unlock()
	s.mu.Unlock()

	if clientWs == nil {
		log.Printf("[relay] host-data but client already gone %s/%s", serverId, connectionId)
		conn.Close()
		return
	}

	log.Printf("[relay] host-data paired %s/%s", serverId, connectionId)

	r.forwardPair(serverId, connectionId, conn, clientWs, slot)
}

// handleClient parks a remote client until the host opens its matching
// host-data socket (forwardPair then takes over). The relay notifies the
// host with a "connected" control message carrying the new connectionId.
func (r *relay) handleClient(serverId string, conn *websocket.Conn, grant string) {
	_ = grant // reserved for future use

	s := r.getServer(serverId)
	if s == nil {
		log.Printf("[relay] no host for serverId=%s, closing client", serverId)
		conn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(4008, "host unavailable"))
		conn.Close()
		return
	}

	s.mu.Lock()

	if s.controlWs == nil {
		clientCount := len(s.clients)
		s.mu.Unlock()
		if clientCount == 0 {
			r.maybeRemoveServer(serverId)
		}
		log.Printf("[relay] no host for serverId=%s, closing client", serverId)
		conn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(4008, "host unavailable"))
		conn.Close()
		return
	}

	if len(s.clients) >= maxClientsPerHost {
		s.mu.Unlock()
		log.Printf("[relay] client limit reached for %s (%d)", serverId, maxClientsPerHost)
		conn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(4029, "limit exceeded"))
		conn.Close()
		return
	}

	connectionId := fmt.Sprintf("c%d", time.Now().UnixNano())
	slot := &clientSlot{
		clientWs:   conn,
		createdAt:  time.Now(),
		lastActive: time.Now(),
		done:       make(chan struct{}),
	}
	s.clients[connectionId] = slot
	s.mu.Unlock()

	s.sendControl(relayMessage{Type: "connected", ConnectionId: connectionId})

	log.Printf("[relay] client connected %s/%s", serverId, connectionId)

	<-slot.done
	conn.Close()
}

// forwardPair copies frames between the host data socket and the client
// socket in both directions until either side errors, then tears the pair
// down and notifies the host with a "disconnected" control message.
func (r *relay) forwardPair(serverId, connectionId string, hostData, client *websocket.Conn, slot *clientSlot) {
	done := make(chan struct{}, 2)

	// host → client
	go func() {
		defer func() { done <- struct{}{} }()
		for {
			msgType, msg, err := hostData.ReadMessage()
			if err != nil {
				return
			}
			slot.mu.Lock()
			slot.lastActive = time.Now()
			slot.mu.Unlock()
			client.SetWriteDeadline(time.Now().Add(writeWait))
			if err := client.WriteMessage(msgType, msg); err != nil {
				return
			}
		}
	}()

	// client → host
	go func() {
		defer func() { done <- struct{}{} }()
		for {
			msgType, msg, err := client.ReadMessage()
			if err != nil {
				return
			}
			slot.mu.Lock()
			slot.lastActive = time.Now()
			slot.mu.Unlock()
			hostData.SetWriteDeadline(time.Now().Add(writeWait))
			if err := hostData.WriteMessage(msgType, msg); err != nil {
				return
			}
		}
	}()

	<-done

	hostData.Close()
	client.Close()
	slot.finish()

	s := r.getServer(serverId)
	if s != nil {
		s.mu.Lock()
		// Only delete if our slot is still the one in the map.
		if cur, ok := s.clients[connectionId]; ok && cur == slot {
			delete(s.clients, connectionId)
		}
		s.mu.Unlock()
		s.sendControl(relayMessage{Type: "disconnected", ConnectionId: connectionId})
	}

	log.Printf("[relay] pair closed %s/%s", serverId, connectionId)

	r.maybeRemoveServer(serverId)
}
