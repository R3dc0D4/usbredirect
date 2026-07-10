package agent

import (
	"context"
	"fmt"
	"time"

	go_serial "go.bug.st/serial"

	"github.com/r3dc0d4/usbredirect/internal/serial"
	"github.com/r3dc0d4/usbredirect/internal/tether"
	"github.com/r3dc0d4/usbredirect/internal/virtual"
)

// runServerTether connects to Tether server in server mode (physical COM → WebSocket).
func (a *Agent) runServerTether(serialPort *serial.Port) error {
	a.logger.Info("Starting Tether server mode",
		"port", a.cfg.Serial.Port,
		"server_url", a.cfg.Server.URL,
	)

	// Create Tether client
	tc := tether.NewClient(a.cfg.Server.URL, a.cfg.Server.Token)
	tc.OnData(func(data []byte) {
		// Data from Tether → write to serial port
		if _, err := serialPort.Write(data); err != nil {
			a.logger.Error("Serial write error", "error", err)
		} else {
			a.logger.Debug("Tether→Serial", "bytes", len(data))
		}
	})

	tc.OnControl(func(msg *tether.Message) {
		// RFC 2217 control message → reconfigure serial port
		switch msg.SubType {
		case "set_baud":
			if baud, ok := msg.Value.(float64); ok {
				a.logger.Info("RFC 2217: Reconfiguring baud rate", "baud", int(baud))
				parity, _ := serial.ParseParity(a.cfg.Serial.Parity)
				dataBits, _ := serial.ParseDataBits(a.cfg.Serial.DataBits)
				stopBits, _ := serial.ParseStopBits(a.cfg.Serial.StopBits)
				if err := serialPort.Reconfigure(int(baud), dataBits, parity, stopBits); err != nil {
					a.logger.Error("RFC 2217: Failed to reconfigure baud", "error", err)
				}
			}
		case "set_parity":
			if parity, ok := msg.Value.(string); ok {
				a.logger.Info("RFC 2217: Reconfiguring parity", "parity", parity)
				p, _ := serial.ParseParity(parity)
				d, _ := serial.ParseDataBits(a.cfg.Serial.DataBits)
				s, _ := serial.ParseStopBits(a.cfg.Serial.StopBits)
				if err := serialPort.Reconfigure(a.cfg.Serial.Baud, d, p, s); err != nil {
					a.logger.Error("RFC 2217: Failed to reconfigure parity", "error", err)
				}
			}
		case "set_databits":
			if bits, ok := msg.Value.(float64); ok {
				a.logger.Info("RFC 2217: Reconfiguring data bits", "bits", int(bits))
				p, _ := serial.ParseParity(a.cfg.Serial.Parity)
				s, _ := serial.ParseStopBits(a.cfg.Serial.StopBits)
				if err := serialPort.Reconfigure(a.cfg.Serial.Baud, int(bits), p, s); err != nil {
					a.logger.Error("RFC 2217: Failed to reconfigure data bits", "error", err)
				}
			}
		case "set_stopbits":
			if bits, ok := msg.Value.(float64); ok {
				a.logger.Info("RFC 2217: Reconfiguring stop bits", "bits", int(bits))
				p, _ := serial.ParseParity(a.cfg.Serial.Parity)
				d, _ := serial.ParseDataBits(a.cfg.Serial.DataBits)
				if err := serialPort.Reconfigure(a.cfg.Serial.Baud, d, p, go_serial.StopBits(int(bits))); err != nil {
					a.logger.Error("RFC 2217: Failed to reconfigure stop bits", "error", err)
				}
			}
		default:
			a.logger.Info("RFC 2217 control message", "subType", msg.SubType, "value", msg.Value)
		}
	})

	tc.OnPaired(func(msg *tether.Message) {
		a.logger.Info("Paired with client",
			"pairId", msg.PairID,
			"partnerId", msg.PartnerID,
			"partnerPort", msg.PartnerPort,
		)
	})

	tc.OnUnpaired(func(reason string) {
		a.logger.Warn("Pair broken", "reason", reason)
	})

	tc.OnError(func(err string) {
		a.logger.Error("Tether serial error", "error", err)
	})

	// Connect with retry
	if err := a.connectWithRetry(tc); err != nil {
		return err
	}

	// Register as server agent
	if err := tc.Register("server", a.cfg.Serial.Port, a.cfg.Serial.Baud, a.cfg.Serial.Parity, a.cfg.Serial.DataBits, a.cfg.Serial.StopBits); err != nil {
		return fmt.Errorf("failed to register as server: %w", err)
	}

	// If target server specified, auto-connect
	if a.cfg.Server.RemoteClient != "" {
		a.logger.Info("Auto-connecting to client", "target", a.cfg.Server.RemoteClient)
		if err := tc.ConnectToServer(a.cfg.Server.RemoteClient); err != nil {
			a.logger.Warn("Auto-connect failed", "error", err)
		}
	}

	fmt.Printf("\n╔══════════════════════════════════════════════════╗\n")
	fmt.Printf("║  USB Redirect Server - TETHER MODE             ║\n")
	fmt.Printf("║  Serial Port: %-34s  ║\n", a.cfg.Serial.Port)
	fmt.Printf("║  Tether: %-38s  ║\n", a.cfg.Server.URL)
	fmt.Printf("║  Mode: Server (physical COM)                    ║\n")
	fmt.Printf("║  Press Ctrl+C to disconnect.                    ║\n")
	fmt.Printf("╚══════════════════════════════════════════════════╝\n\n")

	// Read from serial port and send to Tether
	done := make(chan error, 2)

	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := serialPort.Read(buf)
			if err != nil {
				done <- fmt.Errorf("serial read: %w", err)
				return
			}
			if n > 0 {
				if err := tc.SendData(buf[:n]); err != nil {
					done <- fmt.Errorf("tether send: %w", err)
					return
				}
				a.logger.Debug("Serial→Tether", "bytes", n)
			}
		}
	}()

	// Heartbeat
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if tc.IsConnected() {
					tc.SendHeartbeat()
				}
			}
		}
	}()

	select {
	case err := <-done:
		return err
	case sig := <-waitForSignalChan():
		a.logger.Info("Received signal, shutting down", "signal", sig)
		tc.Close()
		return nil
	}
}

// runClientTether connects to Tether server in client mode (WebSocket → virtual COM).
func (a *Agent) runClientTether() error {
	a.logger.Info("Starting Tether client mode",
		"server_url", a.cfg.Server.URL,
		"virtual", a.cfg.Virtual.Port,
	)

	// Connect to Tether server
	tc := tether.NewClient(a.cfg.Server.URL, a.cfg.Server.Token)

	// Create virtual COM port
	vp, err := virtual.Create(a.cfg.Virtual.Port)
	if err != nil {
		return fmt.Errorf("failed to create virtual port: %w", err)
	}
	defer vp.Close()

	// Data from Tether → write to virtual port
	tc.OnData(func(data []byte) {
		if _, err := vp.Write(data); err != nil {
			a.logger.Error("Virtual port write error", "error", err)
		} else {
			a.logger.Debug("Tether→Virtual", "bytes", len(data))
		}
	})

	tc.OnControl(func(msg *tether.Message) {
		a.logger.Info("Serial control message", "subType", msg.SubType, "value", msg.Value)
		// TODO: RFC 2217 control → reconfigure virtual port
	})

	tc.OnPaired(func(msg *tether.Message) {
		a.logger.Info("Paired with server",
			"pairId", msg.PairID,
			"partnerId", msg.PartnerID,
			"partnerPort", msg.PartnerPort,
		)
	})

	tc.OnUnpaired(func(reason string) {
		a.logger.Warn("Pair broken", "reason", reason)
	})

	tc.OnError(func(err string) {
		a.logger.Error("Tether serial error", "error", err)
	})

	// Connect with retry
	if err := a.connectWithRetry(tc); err != nil {
		return err
	}

	// Register as client agent
	if err := tc.Register("client", vp.PortName(), a.cfg.Serial.Baud, a.cfg.Serial.Parity, a.cfg.Serial.DataBits, a.cfg.Serial.StopBits); err != nil {
		return fmt.Errorf("failed to register as client: %w", err)
	}

	// If target server specified, connect to it
	if a.cfg.Server.RemoteClient != "" {
		a.logger.Info("Connecting to server agent", "target", a.cfg.Server.RemoteClient)
		if err := tc.ConnectToServer(a.cfg.Server.RemoteClient); err != nil {
			a.logger.Warn("Connect to server failed", "error", err)
		}
	}

	fmt.Printf("\n╔══════════════════════════════════════════════════╗\n")
	fmt.Printf("║  USB Redirect Client - TETHER MODE             ║\n")
	fmt.Printf("║  Tether: %-38s  ║\n", a.cfg.Server.URL)
	fmt.Printf("║  Virtual Port: %-32s  ║\n", vp.PortName())
	fmt.Printf("║  Mode: Client (virtual COM)                      ║\n")
	fmt.Printf("║  Connect your software to this port.              ║\n")
	fmt.Printf("║  Press Ctrl+C to disconnect.                    ║\n")
	fmt.Printf("╚══════════════════════════════════════════════════╝\n\n")

	// Read from virtual port and send to Tether
	done := make(chan error, 2)

	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := vp.Read(buf)
			if err != nil {
				// EIO can happen when no application has the PTY slave open
				if isRetryableError(err) {
					a.logger.Debug("Waiting for application to open virtual port...")
					time.Sleep(100 * time.Millisecond)
					continue
				}
				if err.Error() == "EOF" {
					a.logger.Info("Virtual port closed (EOF)")
					done <- nil
					return
				}
				done <- fmt.Errorf("virtual read: %w", err)
				return
			}
			if n > 0 {
				if err := tc.SendData(buf[:n]); err != nil {
					done <- fmt.Errorf("tether send: %w", err)
					return
				}
				a.logger.Debug("Virtual→Tether", "bytes", n)
			}
		}
	}()

	// Heartbeat
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if tc.IsConnected() {
					tc.SendHeartbeat()
				}
			}
		}
	}()

	select {
	case err := <-done:
		return err
	case sig := <-waitForSignalChan():
		a.logger.Info("Received signal, shutting down", "signal", sig)
		tc.Close()
		return nil
	}
}

// connectWithRetry connects to Tether server with exponential backoff retry.
func (a *Agent) connectWithRetry(tc *tether.Client) error {
	retryDelay := a.cfg.Server.Reconnect.Initial
	maxRetries := 30 // ~5 minutes with exponential backoff

	for i := 0; i < maxRetries; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		err := tc.Connect(ctx)
		cancel()

		if err == nil {
			a.logger.Info("Connected to Tether server")
			return nil
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

	return fmt.Errorf("failed to connect after %d retries", maxRetries)
}