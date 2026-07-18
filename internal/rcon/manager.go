package rcon

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Manager keeps one persistent, auto-reconnecting RCON connection per
// running instance, matching ARCHITECTURE.md section 5.4 ("패널은
// 인스턴스당 하나의 상시 RCON 소켓 연결을 유지"). Both the REST command
// endpoint and the WebSocket console share the same managed connection
// rather than dialing fresh for every call.
type Manager struct {
	mu    sync.Mutex
	conns map[string]*managedConn
}

type managedConn struct {
	mu     sync.Mutex // serializes Execute calls: RCON has no pipelining
	client *Client    // nil while (re)connecting
	cancel context.CancelFunc
}

func NewManager() *Manager {
	return &Manager{conns: make(map[string]*managedConn)}
}

// StartMaintaining begins (or restarts) a background goroutine that keeps a
// live RCON connection to addr for instanceID, redialing with backoff
// whenever the connection drops. Call this once the instance's process has
// been started; it's safe to call again (e.g. on daemon restart
// reconciliation) since it replaces any prior connection for the same ID.
//
// onConnect, if non-nil, fires every time a connection is (re)established --
// callers use this to know the instance has actually finished booting
// (RCON only comes up once the server reaches its main loop), since nothing
// else signals that transition otherwise. May be called from a different
// goroutine than the caller's.
func (m *Manager) StartMaintaining(instanceID, addr, password string, onConnect func()) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if existing, ok := m.conns[instanceID]; ok {
		existing.cancel()
	}

	ctx, cancel := context.WithCancel(context.Background())
	mc := &managedConn{cancel: cancel}
	m.conns[instanceID] = mc

	go mc.maintain(ctx, addr, password, onConnect)
}

// StopMaintaining closes the connection for instanceID and stops trying to
// reconnect. Call this when the instance stops or is deleted.
func (m *Manager) StopMaintaining(instanceID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if mc, ok := m.conns[instanceID]; ok {
		mc.cancel()
		delete(m.conns, instanceID)
	}
}

// Execute runs command on instanceID's managed connection. Returns an error
// if no connection is currently established (e.g. the server is still
// booting, or RCON just dropped and hasn't reconnected yet) -- callers
// should surface this as "try again in a moment" rather than treating it as
// a hard failure.
func (m *Manager) Execute(instanceID, command string) (string, error) {
	m.mu.Lock()
	mc, ok := m.conns[instanceID]
	m.mu.Unlock()
	if !ok {
		return "", fmt.Errorf("no RCON connection is being maintained for this instance (is it running?)")
	}
	return mc.execute(command)
}

// Connected reports whether instanceID currently has a live RCON client --
// a cheap, in-process signal that the server process is (still) up, without
// spawning a `systemctl` subprocess. Used to detect a graceful shutdown
// finishing instead of polling systemctl is-active in a loop, which was
// racing against systemd tearing down the just-exited transient unit and
// logging a harmless but noisy "Failed to open .../transient/....service:
// No such file or directory" into that same unit's own journal every time
// (confirmed on real hardware -- an operator watching the live console saw
// it repeated right after a clean stop).
func (m *Manager) Connected(instanceID string) bool {
	m.mu.Lock()
	mc, ok := m.conns[instanceID]
	m.mu.Unlock()
	if !ok {
		return false
	}
	mc.mu.Lock()
	defer mc.mu.Unlock()
	return mc.client != nil
}

func (mc *managedConn) execute(command string) (string, error) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	if mc.client == nil {
		return "", fmt.Errorf("rcon not connected yet (server may still be booting)")
	}
	result, err := mc.client.Execute(command)
	if err != nil {
		// The connection is presumably dead; drop it so maintain()'s next
		// loop iteration redials instead of every future call failing
		// against a known-bad socket.
		mc.client.Close()
		mc.client = nil
		return "", fmt.Errorf("rcon command failed, reconnecting: %w", err)
	}
	return result, nil
}

// maintain dials addr in a loop until ctx is cancelled (via StopMaintaining
// or a replacement StartMaintaining call), storing the live client for
// execute() to use and clearing it whenever the connection is found dead.
func (mc *managedConn) maintain(ctx context.Context, addr, password string, onConnect func()) {
	backoff := 1 * time.Second
	const maxBackoff = 15 * time.Second

	for {
		select {
		case <-ctx.Done():
			mc.mu.Lock()
			if mc.client != nil {
				mc.client.Close()
				mc.client = nil
			}
			mc.mu.Unlock()
			return
		default:
		}

		client, err := Dial(addr, password, 30*time.Second)
		if err != nil {
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			if backoff < maxBackoff {
				backoff *= 2
			}
			continue
		}

		backoff = 1 * time.Second
		mc.mu.Lock()
		mc.client = client
		mc.mu.Unlock()

		if onConnect != nil {
			onConnect()
		}

		// Idle until the connection is cleared by a failed execute() call
		// or ctx is cancelled, then loop around to redial.
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(2 * time.Second):
			}
			mc.mu.Lock()
			dead := mc.client == nil
			mc.mu.Unlock()
			if dead {
				break
			}
		}
	}
}
