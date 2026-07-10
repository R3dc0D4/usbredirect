package agent

import (
	"fmt"

	"github.com/r3dc0d4/usbredirect/internal/serial"
)

// runServerTether connects to Tether server and bridges serial port data via WebSocket.
func (a *Agent) runServerTether(serialPort *serial.Port) error {
	// TODO: Implement Tether WebSocket client for server mode
	// 1. Connect to Tether server via WebSocket
	// 2. Register as serial_port channel with mode=server
	// 3. Bridge serial port data to/from Tether server
	return fmt.Errorf("Tether server mode not yet implemented (use TCP mode with --listen)")
}

// runClientTether connects to Tether server and creates a virtual COM port.
func (a *Agent) runClientTether() error {
	// TODO: Implement Tether WebSocket client for client mode
	// 1. Connect to Tether server via WebSocket
	// 2. Register as serial_port channel with mode=client
	// 3. Find server agent by remote_client ID
	// 4. Create virtual COM port
	// 5. Bridge virtual COM data to/from Tether server
	return fmt.Errorf("Tether client mode not yet implemented (use TCP mode with --remote)")
}

// Placeholder for future Tether integration
// The actual implementation will be in internal/tether/ws.go

var _ = serial.ListPorts // ensure serial package is used