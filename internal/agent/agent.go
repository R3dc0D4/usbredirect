package agent

import (
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/r3dc0d4/usbredirect/internal/config"
	"github.com/r3dc0d4/usbredirect/internal/serial"
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


	// Open serial port
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

// waitForSignal blocks until a termination signal is received.
func waitForSignal() os.Signal {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	return <-sigCh
}