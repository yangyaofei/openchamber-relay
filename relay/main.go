// Package main implements the OpenChamber relay: a small stateless WebSocket
// relay that lets remote OpenChamber clients (mobile apps, browsers on other
// machines) reach a self-hosted OpenChamber server behind NAT or a firewall.
//
// The relay never inspects application traffic. Clients and the host server
// exchange end-to-end encrypted frames (ECDH + AES-GCM, negotiated out of
// band); the relay only pairs sockets and forwards opaque binary messages.
//
// Connection model (all endpoints accept the /ws upgrade with query params):
//
//	role=host-control  one per serverId; carries control messages
//	                   (client connected/disconnected/sync)
//	role=host-data     one per client connectionId; carries that client's
//	                   paired data frames
//	role=client        remote client entry point; parked until the host opens
//	                   the matching host-data socket
//
// Run with:
//
//	PORT=8080 RELAY_VERIFY_AUTH=false go run .
package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	verifyAuth := os.Getenv("RELAY_VERIFY_AUTH") == "true"
	serviceName := os.Getenv("RELAY_SERVICE_NAME")
	if serviceName == "" {
		serviceName = "openchamber-relay"
	}

	r := newRelay(verifyAuth)
	r.startSweeper()

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", r.handleWebSocket)
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"ok":      "true",
			"service": serviceName,
		})
	})

	addr := ":" + port
	log.Printf("[relay] starting on %s (verifyAuth=%v, maxMsg=%dMB, maxClients=%d)",
		addr, verifyAuth, maxMessageSize/(1024*1024), maxClientsPerHost)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("[relay] failed to start: %v", err)
	}
}
