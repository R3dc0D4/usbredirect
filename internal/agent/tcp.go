package agent

import (
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/r3dc0d4/usbredirect/internal/serial"
	"github.com/r3dc0d4/usbredirect/internal/virtual"
)

// runServerTCP starts a raw TCP server that bridges serial port data.
// Supports multiple clients — serial data is broadcast to all clients,
// and data from any client is written to serial.
func (a *Agent) runServerTCP(serialPort *serial.Port) error {
	listener, err := net.Listen("tcp", a.cfg.Network.Listen)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", a.cfg.Network.Listen, err)
	}
	defer listener.Close()

	a.logger.Info("TCP server listening", "addr", listener.Addr())

	// Channel for serial data
	serialData := make(chan []byte, 256)
	clientConns := make(map[net.Conn]bool)
	clientAdd := make(chan net.Conn)
	clientRemove := make(chan net.Conn)

	// Read from serial port in a goroutine
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := serialPort.Read(buf)
			if err != nil {
				a.logger.Error("Serial read error", "error", err)
				return
			}
			if n > 0 {
				data := make([]byte, n)
				copy(data, buf[:n])
				serialData <- data
			}
		}
	}()

	// Accept connections
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				a.logger.Error("Accept error", "error", err)
				return
			}
			a.logger.Info("Client connected", "addr", conn.RemoteAddr())
			clientAdd <- conn
		}
	}()

	a.logger.Info("USB redirect server ready",
		"serial", serialPort.Name(),
		"listen", a.cfg.Network.Listen,
		"hint", "Connect with: usbredirect agent --mode client --remote <server-ip>"+a.cfg.Network.Listen,
	)

	for {
		select {
		case data := <-serialData:
			// Broadcast serial data to all clients
			for conn := range clientConns {
				if _, err := conn.Write(data); err != nil {
					a.logger.Warn("Write to client failed", "addr", conn.RemoteAddr(), "error", err)
					conn.Close()
					delete(clientConns, conn)
				}
			}

		case conn := <-clientAdd:
			clientConns[conn] = true
			// Read from this client in a goroutine
			go func(c net.Conn) {
				buf := make([]byte, 4096)
				for {
					n, err := c.Read(buf)
					if err != nil {
						if err != io.EOF {
							a.logger.Warn("Client read error", "addr", c.RemoteAddr(), "error", err)
						}
						a.logger.Info("Client disconnected", "addr", c.RemoteAddr())
						clientRemove <- c
						return
					}
					if n > 0 {
						// Write client data to serial port
						if _, err := serialPort.Write(buf[:n]); err != nil {
							a.logger.Error("Serial write error", "error", err)
						}
					}
				}
			}(conn)

		case conn := <-clientRemove:
			delete(clientConns, conn)
			conn.Close()
		}
	}
}

// runClientTCP connects to a remote TCP server and creates a virtual serial port.
func (a *Agent) runClientTCP() error {
	a.logger.Info("Connecting to remote server", "addr", a.cfg.Network.Remote)

	// Connect with retry
	var conn net.Conn
	var err error
	maxRetries := 10
	retryDelay := a.cfg.Server.Reconnect.Initial

	for i := 0; i < maxRetries; i++ {
		conn, err = net.Dial("tcp", a.cfg.Network.Remote)
		if err == nil {
			break
		}
		a.logger.Warn("Connection failed, retrying",
			"attempt", i+1,
			"error", err,
			"delay", retryDelay,
		)
		time.Sleep(retryDelay)
		if retryDelay < a.cfg.Server.Reconnect.Max {
			retryDelay = time.Duration(float64(retryDelay) * a.cfg.Server.Reconnect.Multiplier)
			if retryDelay > a.cfg.Server.Reconnect.Max {
				retryDelay = a.cfg.Server.Reconnect.Max
			}
		}
	}
	if err != nil {
		return fmt.Errorf("failed to connect after %d retries: %w", maxRetries, err)
	}
	defer conn.Close()

	a.logger.Info("Connected to remote server", "addr", conn.RemoteAddr())

	// Create virtual COM port
	virtualPort := a.cfg.Virtual.Port
	if virtualPort == "" {
		// Auto-generate a PTY (Linux/macOS) or named pipe (Windows)
		virtualPort = "" // Let Create() decide
	}

	vp, err := virtual.Create(virtualPort)
	if err != nil {
		return fmt.Errorf("failed to create virtual port: %w", err)
	}
	defer vp.Close()

	a.logger.Info("Virtual serial port created", "port", vp.PortName())
	fmt.Printf("\n╔══════════════════════════════════════════════════╗\n")
	fmt.Printf("║  USB Redirect Client - CONNECTED                ║\n")
	fmt.Printf("║  Remote: %-38s  ║\n", a.cfg.Network.Remote)
	fmt.Printf("║  Virtual Port: %-32s  ║\n", vp.PortName())
	fmt.Printf("║  Connect your software to this port.              ║\n")
	fmt.Printf("║  Press Ctrl+C to disconnect.                    ║\n")
	fmt.Printf("╚══════════════════════════════════════════════════╝\n\n")

	// Bridge: Virtual port <-> TCP connection
	done := make(chan error, 2)

	// Virtual port (PTY master) -> TCP
	// Note: PTY Read() returns EIO when no slave is open; we retry.
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := vp.Read(buf)
			if err != nil {
				if err == io.EOF {
					a.logger.Info("Virtual port closed (EOF)")
					done <- nil
					return
				}
				// EIO (input/output error) happens when no slave has PTY open yet
				// Retry with backoff until an application opens the virtual port
				if isRetryableError(err) {
					a.logger.Debug("Waiting for application to open virtual port...")
					time.Sleep(100 * time.Millisecond)
					continue
				}
				a.logger.Error("Virtual port read error", "error", err)
				done <- fmt.Errorf("virtual read: %w", err)
				return
			}
			if n > 0 {
				if _, err := conn.Write(buf[:n]); err != nil {
					done <- fmt.Errorf("tcp write: %w", err)
					return
				}
				a.logger.Info("Virtual→TCP", "bytes", n)
			}
		}
	}()

	// TCP -> Virtual port (PTY master)
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := conn.Read(buf)
			if err != nil {
				if err == io.EOF {
					a.logger.Info("Connection closed by remote")
					done <- nil
					return
				}
				a.logger.Error("TCP read error", "error", err)
				done <- fmt.Errorf("tcp read: %w", err)
				return
			}
			if n > 0 {
				if _, err := vp.Write(buf[:n]); err != nil {
					done <- fmt.Errorf("virtual write: %w", err)
					return
				}
				a.logger.Info("TCP→Virtual", "bytes", n)
			}
		}
	}()

	// Wait for completion or signal
	select {
	case err := <-done:
		if err != nil {
			a.logger.Error("Bridge error", "error", err)
		}
		return err
	case sig := <-waitForSignalChan():
		a.logger.Info("Received signal, shutting down", "signal", sig)
		return nil
	}
}

// waitForSignalChan returns a channel that receives OS signals.
func waitForSignalChan() <-chan os.Signal {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt)
	return ch
}

// Placeholder for future Tether integration
// serial and config packages are used in other files in this package
var _ = io.EOF

// isRetryableError checks if the error is a temporary condition that can be retried.
// On Linux, PTY Read() returns EIO when no slave is open.
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}
	// Check for "input/output error" (EIO) which happens when
	// no application has opened the PTY slave yet
	return err.Error() == "read /dev/ptmx: input/output error" ||
		strings.Contains(err.Error(), "input/output error") ||
		strings.Contains(err.Error(), "I/O error")
}