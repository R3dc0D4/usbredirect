package agent

import (
	"fmt"
	"io"
	"net"
	"os"

	"github.com/r3dc0d4/usbredirect/internal/serial"
)

// runServerTCP starts a raw TCP server that bridges serial port data.
// Multiple clients can connect; data from serial is broadcast to all clients,
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

	a.logger.Info("USB redirect server ready", "serial", serialPort.Name(), "listen", a.cfg.Network.Listen)

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

// runClientTCP connects to a remote TCP server and creates a virtual serial port (PTY on Linux/macOS).
func (a *Agent) runClientTCP() error {
	a.logger.Info("Connecting to remote server", "addr", a.cfg.Network.Remote)

	conn, err := net.Dial("tcp", a.cfg.Network.Remote)
	if err != nil {
		return fmt.Errorf("failed to connect to %s: %w", a.cfg.Network.Remote, err)
	}
	defer conn.Close()

	a.logger.Info("Connected to remote server", "addr", conn.RemoteAddr())

	// TODO: Create virtual COM port
	// On Linux/macOS: PTY
	// On Windows: Named pipe or kernel driver
	// For MVP: just bridge stdin/stdout or create a PTY

	a.logger.Info("Bridging network to virtual port (MVP: using PTY/stdout)")

	// Simple bidirectional copy for MVP
	done := make(chan error, 2)

	// Network → Serial (stdout for MVP)
	go func() {
		_, err := io.Copy(osStdout{}, conn)
		done <- err
	}()

	// Serial (stdin for MVP) → Network
	go func() {
		_, err := io.Copy(conn, osStdin{})
		done <- err
	}()

	err = <-done
	a.logger.Info("Connection closed", "error", err)
	return nil
}

// osStdout wraps os.Stdout to implement io.Writer
type osStdout struct{}

func (osStdout) Write(p []byte) (int, error) {
	return os.Stdout.Write(p)
}

// osStdin wraps os.Stdin to implement io.Reader
type osStdin struct{}

func (osStdin) Read(p []byte) (int, error) {
	return os.Stdin.Read(p)
}