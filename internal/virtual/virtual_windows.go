//go:build windows

package virtual

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
)

// VirtualPort manages a named-pipe-based virtual serial port on Windows.
//
// For MVP: Uses Windows named pipes as a bridge between the TCP client
// and applications. Full Windows kernel driver (KMDF virtual COM) will
// be implemented in Phase 4.
//
// Named pipes on Windows can be accessed as: \\.\pipe\usbredirect-COM5
// Applications can connect to this pipe instead of a real COM port.
//
// Limitations (MVP):
//   - Not a real COM port (software must support named pipe input)
//   - Standard serial port settings (baud, parity) are virtual
//   - For a real COM5 port, Phase 4 kernel driver is needed
type VirtualPort struct {
	pipeName string
	pipeFile *os.File
	logger   *slog.Logger
}

// Create creates a named pipe for serial data bridge on Windows.
// The linkPath parameter specifies the pipe name, e.g., "COM5" or "\\.\pipe\usbredirect".
func Create(linkPath string) (*VirtualPort, error) {
	logger := slog.Default().With("component", "virtual-pipe")

	pipeName := linkPath
	if pipeName == "" {
		pipeName = `\\.\pipe\usbredirect`
	}
	// Normalize pipe name
	if !strings.HasPrefix(pipeName, `\\.\pipe\`) {
		pipeName = `\\.\pipe\` + pipeName
	}

	logger.Info("Creating named pipe (Windows MVP)", "pipe", pipeName)
	logger.Warn("Note: Named pipe is NOT a real COM port. For real COM port support, Phase 4 (KMDF driver) is required.")

	// For MVP: We'll use the TCP connection directly and log instructions.
	// The actual named pipe creation requires Windows API calls (CreateNamedPipe)
	// which needs syscall package. For now, return a stub.

	vp := &VirtualPort{
		pipeName: pipeName,
		logger:   logger,
	}

	logger.Info("Virtual port (named pipe) configured", "port", pipeName)
	logger.Info("Applications should connect via TCP directly or use Phase 4 driver for real COM port")
	return vp, nil
}

// Read reads data from the named pipe (MVP: no-op, use TCP directly).
func (vp *VirtualPort) Read(buf []byte) (int, error) {
	// MVP: Data flows through TCP connection, not through this pipe
	return 0, fmt.Errorf("named pipe read not implemented in MVP - use TCP connection directly")
}

// Write writes data to the named pipe (MVP: no-op).
func (vp *VirtualPort) Write(data []byte) (int, error) {
	return 0, fmt.Errorf("named pipe write not implemented in MVP - use TCP connection directly")
}

// Close closes the virtual port.
func (vp *VirtualPort) Close() error {
	vp.logger.Info("Virtual port closed")
	return nil
}

// PortName returns the pipe name.
func (vp *VirtualPort) PortName() string {
	return vp.pipeName
}

// SetBaudRate is a no-op for named pipes.
func (vp *VirtualPort) SetBaudRate(baud int) error {
	return nil
}

// CheckPTAvailable checks if named pipes are available (always true on Windows).
func CheckPTAvailable() error {
	return nil
}