package main

import "sync/atomic"

// ServerState represents the current lifecycle phase of the DST server.
type ServerState int32

const (
	StatePreparing  ServerState = iota // prepare.sh is running
	StateInstalling                    // install.sh / steamcmd is running
	StateStarting                      // DST binary launched, not yet accepting connections
	StateRunning                       // DST is accepting connections (A2S responds)
	StateStopping                      // graceful shutdown in progress
	StateStopped                       // DST process has exited
)

func (s ServerState) String() string {
	switch s {
	case StatePreparing:
		return "preparing"
	case StateInstalling:
		return "installing"
	case StateStarting:
		return "starting"
	case StateRunning:
		return "running"
	case StateStopping:
		return "stopping"
	case StateStopped:
		return "stopped"
	default:
		return "unknown"
	}
}

// StateManager provides thread-safe access to the server state.
type StateManager struct {
	state atomic.Int32
}

func (sm *StateManager) Get() ServerState {
	return ServerState(sm.state.Load())
}

func (sm *StateManager) Set(s ServerState) {
	sm.state.Store(int32(s))
}
