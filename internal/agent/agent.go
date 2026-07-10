package agent

import (
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/r3dc0d4/usbredirect/internal/config"
	"github.com/r3dc0d4/usbredirect/internal/serial"
	"github.com/r3dc0d4/usbredirect/internal/virtual"
)

// Agent is the main USB redirect agent.
type Agent struct {
	cfg    *config.Config
	logger *slog.Logger
}

// New creates a new agent with the given configuration.
func New(cfg *config.Config) (*Agent, error) {
	logger := slog.Default().With("mode", cfg.Mode)
	return &Agent{
		cfg:    cfg,
		logger: logger,
	}, nil
}

// Run starts the agent based on its mode.
func (a *Agent) Run() error {
	switch a.cfg.Mode {
	case "server":
		return a.runServer()
	case "client":
		return a.runClient()
	default:
		return fmt.Errorf("unknown mode: %s", a.cfg.Mode)
	}
}

// runServer starts the agent in server mode (serial port → network).
func (a *Agent) runServer() error {
	a.logger.Info("Starting USB redirect server",
		"port", a.cfg.Serial.Port,
		"baud", a.cfg.Serial.Baud,
		"listen", a.cfg.Network.Listen,
	)

	// Parse serial config
	parity, err := serial.ParseParity(a.cfg.Serial.Parity)
	if err != nil {
		return err
	}
	dataBits, err := serial.ParseDataBits(a.cfg.Serial.DataBits)
	if err != nil {
		return err
	}
	stopBits, err := serial.ParseStopBits(a.cfg.Serial.StopBits)
	if err != nil {
		return err
	}

	serialCfg := &serial.Config{
		Port:     a.cfg.Serial.Port,
		Baud:     a.cfg.Serial.Baud,
		DataBits: dataBits,
		Parity:   parity,
		StopBits: stopBits,
	}

	serialPort, err := serial.Open(serialCfg)
	if err != nil {
		return fmt.Errorf("failed to open serial port: %w", err)
	}
	defer serialPort.Close()

	// Start TCP server or Tether client based on config
	if a.cfg.Server.URL != "" {
		return a.runServerTether(serialPort)
	}
	return a.runServerTCP(serialPort)
}

// runClient starts the agent in client mode (network → virtual COM).
func (a *Agent) runClient() error {
	a.logger.Info("Starting USB redirect client",
		"remote", a.cfg.Network.Remote,
		"virtual", a.cfg.Virtual.Port,
		"server_url", a.cfg.Server.URL,
	)

	// Connect via Tether or direct TCP
	if a.cfg.Server.URL != "" {
		return a.runClientTether()
	}
	return a.runClientTCP()
}

// ListPorts lists available serial ports.
func ListPorts() error {
	ports, err := serial.ListPorts()
	if err != nil {
		return fmt.Errorf("failed to list ports: %w", err)
	}

	if len(ports) == 0 {
		fmt.Println("No serial ports found.")
		return nil
	}

	fmt.Println("Available serial ports:")
	for _, p := range ports {
		fmt.Printf("  %s\n", p)
	}
	return nil
}

// bridgeSerialTCP bridges a serial port and a TCP connection bidirectionally.
func bridgeSerialTCP(serialPort *serial.Port, conn net.Conn, logger *slog.Logger, done chan error) {
	// Serial → TCP
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := serialPort.Read(buf)
			if err != nil {
				logger.Error("Serial read error", "error", err)
				done <- fmt.Errorf("serial read: %w", err)
				return
			}
			if n > 0 {
				if _, err := conn.Write(buf[:n]); err != nil {
					logger.Error("TCP write error", "error", err)
					done <- fmt.Errorf("tcp write: %w", err)
					return
				}
				logger.Debug("Serial→TCP", "bytes", n)
			}
		}
	}()

	// TCP → Serial
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := conn.Read(buf)
			if err != nil {
				if err != io.EOF {
					logger.Error("TCP read error", "error", err)
				}
				done <- fmt.Errorf("tcp read: %w", err)
				return
			}
			if n > 0 {
				if _, err := serialPort.Write(buf[:n]); err != nil {
					logger.Error("Serial write error", "error", err)
					done <- fmt.Errorf("serial write: %w", err)
					return
				}
				logger.Debug("TCP→Serial", "bytes", n)
			}
		}
	}()
}

// bridgeVirtualTCP bridges a virtual port and a TCP connection bidirectionally.
// The virtual port uses PTY master; data written to the PTY master appears on
// the PTY slave (which is the virtual COM port that applications connect to),
// and vice versa.
func bridgeVirtualTCP(vPort *virtual.VirtualPort, conn net.Conn, logger *slog.Logger, done chan error) {
	// Virtual port (PTY master) → TCP
	// When an application opens the PTY slave and writes data,
	// it becomes readable on the PTY master.
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := vPort.Read(buf)
			if err != nil {
				// EIO is expected when no slave has the PTY open
				if err == io.EOF {
					logger.Info("Virtual port closed (EOF)")
					done <- nil
					return
				}
				logger.Error("Virtual port read error", "error", err)
				done <- fmt.Errorf("virtual read: %w", err)
				return
			}
			if n > 0 {
				if _, err := conn.Write(buf[:n]); err != nil {
					logger.Error("TCP write error", "error", err)
					done <- fmt.Errorf("tcp write: %w", err)
					return
				}
				logger.Debug("Virtual→TCP", "bytes", n)
			}
		}
	}()

	// TCP → Virtual port (PTY master)
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := conn.Read(buf)
			if err != nil {
				if err == io.EOF {
					logger.Info("Connection closed by remote")
					done <- nil
					return
				}
				logger.Error("TCP read error", "error", err)
				done <- fmt.Errorf("tcp read: %w", err)
				return
			}
			if n > 0 {
				if _, err := vPort.Write(buf[:n]); err != nil {
					logger.Error("Virtual port write error", "error", err)
					done <- fmt.Errorf("virtual write: %w", err)
					return
				}
				logger.Debug("TCP→Virtual", "bytes", n)
			}
		}
	}()
}

// waitForSignal blocks until a termination signal is received.
func waitForSignal() os.Signal {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	return <-sigCh
}