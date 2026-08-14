package main

import (
	"log"
	"time"
)

// startSweeper launches a background loop that runs every 30 seconds and
// reclaims stuck resources:
//
//   - clients whose host never opened a host-data socket within
//     dataSocketUnpaired (host never saw the "connected" notification);
//   - paired sockets idle longer than dataSocketIdleMax (dead peers the
//     TCP stack has not noticed yet).
//
// Both cases notify the host with a "disconnected" control message so its
// bookkeeping stays in sync.
func (r *relay) startSweeper() {
	ticker := time.NewTicker(30 * time.Second)
	go func() {
		for range ticker.C {
			r.mu.RLock()
			serverList := make([]string, 0, len(r.servers))
			for sid := range r.servers {
				serverList = append(serverList, sid)
			}
			r.mu.RUnlock()

			for _, serverId := range serverList {
				s := r.getServer(serverId)
				if s == nil {
					continue
				}

				s.mu.Lock()
				for connId, slot := range s.clients {
					slot.mu.Lock()
					idle := time.Since(slot.lastActive)
					noHostData := slot.hostDataWs == nil
					tooOld := time.Since(slot.createdAt)
					slot.mu.Unlock()

					if noHostData && tooOld > dataSocketUnpaired {
						log.Printf("[relay] sweeping unpaired client %s/%s", serverId, connId)
						slot.mu.Lock()
						if slot.clientWs != nil {
							slot.clientWs.Close()
						}
						slot.mu.Unlock()
						slot.finish()
						delete(s.clients, connId)
						s.sendControl(relayMessage{Type: "disconnected", ConnectionId: connId})
						continue
					}

					if idle > dataSocketIdleMax && slot.hostDataWs != nil {
						log.Printf("[relay] sweeping idle client %s/%s", serverId, connId)
						slot.mu.Lock()
						if slot.clientWs != nil {
							slot.clientWs.Close()
						}
						if slot.hostDataWs != nil {
							slot.hostDataWs.Close()
						}
						slot.mu.Unlock()
						slot.finish()
						delete(s.clients, connId)
						s.sendControl(relayMessage{Type: "disconnected", ConnectionId: connId})
					}
				}
				s.mu.Unlock()

				r.maybeRemoveServer(serverId)
			}
		}
	}()
}
