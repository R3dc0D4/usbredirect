package serial

import (
	"fmt"
	"log/slog"

	go_serial "go.bug.st/serial"
)

// Port wraps a serial port with configuration.
type Port struct {
	port     go_serial.Port
	portName string
	config   *Config
	logger   *slog.Logger
}

// Config holds serial port configuration.
type Config struct {
	Port     string
	Baud     int
	DataBits int
	Parity   go_serial.Parity
	StopBits go_serial.StopBits
}

// Open opens a serial port with the given configuration.
func Open(cfg *Config) (*Port, error) {
	mode := &go_serial.Mode{
		BaudRate: cfg.Baud,
		Parity:   cfg.Parity,
		DataBits: cfg.DataBits,
		StopBits: cfg.StopBits,
	}

	port, err := go_serial.Open(cfg.Port, mode)
	if err != nil {
		return nil, fmt.Errorf("failed to open serial port %s: %w", cfg.Port, err)
	}

	logger := slog.Default().With("component", "serial", "port", cfg.Port)

	logger.Info("Serial port opened",
		"baud", cfg.Baud,
		"databits", cfg.DataBits,
		"parity", fmt.Sprintf("%v", cfg.Parity),
		"stopbits", fmt.Sprintf("%v", cfg.StopBits),
	)

	return &Port{
		port:     port,
		portName: cfg.Port,
		config:   cfg,
		logger:   logger,
	}, nil
}

// Read reads data from the serial port.
func (p *Port) Read(buf []byte) (int, error) {
	return p.port.Read(buf)
}

// Write writes data to the serial port.
func (p *Port) Write(data []byte) (int, error) {
	return p.port.Write(data)
}

// Close closes the serial port.
func (p *Port) Close() error {
	if p.port != nil {
		p.logger.Info("Serial port closed")
		return p.port.Close()
	}
	return nil
}

// Name returns the port name.
func (p *Port) Name() string {
	return p.portName
}

// Reconfigure changes the serial port settings (for RFC 2217).
func (p *Port) Reconfigure(baud int, dataBits int, parity go_serial.Parity, stopBits go_serial.StopBits) error {
	mode := &go_serial.Mode{
		BaudRate: baud,
		Parity:   parity,
		DataBits: dataBits,
		StopBits: stopBits,
	}
	if err := p.port.SetMode(mode); err != nil {
		return fmt.Errorf("failed to reconfigure serial port: %w", err)
	}
	p.logger.Info("Serial port reconfigured", "baud", baud, "databits", dataBits, "parity", parity, "stopbits", stopBits)
	return nil
}

// ListPorts returns available serial ports.
func ListPorts() ([]string, error) {
	return go_serial.GetPortsList()
}

// ParseDataBits validates data bits value (5, 6, 7, or 8).
func ParseDataBits(bits int) (int, error) {
	switch bits {
	case 5, 6, 7, 8:
		return bits, nil
	default:
		return 0, fmt.Errorf("invalid data bits: %d (must be 5, 6, 7, or 8)", bits)
	}
}

// ParseParity converts string to serial.Parity.
func ParseParity(p string) (go_serial.Parity, error) {
	switch p {
	case "none":
		return go_serial.NoParity, nil
	case "odd":
		return go_serial.OddParity, nil
	case "even":
		return go_serial.EvenParity, nil
	case "mark":
		return go_serial.MarkParity, nil
	case "space":
		return go_serial.SpaceParity, nil
	default:
		return 0, fmt.Errorf("invalid parity: %s (must be none, odd, even, mark, or space)", p)
	}
}

// ParseStopBits converts int to serial.StopBits.
func ParseStopBits(bits int) (go_serial.StopBits, error) {
	switch bits {
	case 1:
		return go_serial.OneStopBit, nil
	case 2:
		return go_serial.TwoStopBits, nil
	default:
		return 0, fmt.Errorf("invalid stop bits: %d (must be 1 or 2)", bits)
	}
}