package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/r3dc0d4/usbredirect/internal/agent"
	"github.com/r3dc0d4/usbredirect/internal/config"
	"github.com/spf13/cobra"
)

var (
	version = "0.4.0"
	cfgFile string
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "usbredirect",
		Short: "USB Redirect — Cross-platform COM Port Network Redirector",
		Long: `USB Redirect redirects serial (COM) ports over the network.

It allows software on one machine to read/write a serial device
connected to a different machine over TCP or WebSocket (via Tether).

Modes:
  server   — Physical COM port → Network (device side)
  client   — Network → Virtual COM port (software side)

Direct TCP (same network):
  usbredirect agent --mode server --port COM3 --baud 9600 --listen :5760
  usbredirect agent --mode client --remote 192.168.1.50:5760 --virtual COM5

Via Tether (cross-network, through Cloudflare Tunnel):
  usbredirect agent --mode server --port COM3 --server-url wss://tether.example.com
  usbredirect agent --mode client --server-url wss://tether.example.com --virtual COM5`,
		Version: version,
	}

	// Global flags
	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", "config file (default: ./usbredirect.yaml)")
	rootCmd.PersistentFlags().BoolP("verbose", "v", false, "verbose logging")
	rootCmd.PersistentFlags().BoolP("quiet", "q", false, "quiet mode (errors only)")

	// Agent command
	agentCmd := &cobra.Command{
		Use:   "agent",
		Short: "Start USB redirect agent",
		Long:  "Start the USB redirect agent in server or client mode.",
		RunE:  runAgent,
	}

	agentCmd.Flags().String("mode", "server", "Agent mode: server or client")
	agentCmd.Flags().String("port", "", "Serial port (server mode): COM3, /dev/ttyUSB0, etc.")
	agentCmd.Flags().Int("baud", 9600, "Baud rate (server mode)")
	agentCmd.Flags().Int("databits", 8, "Data bits: 5, 6, 7, or 8")
	agentCmd.Flags().String("parity", "none", "Parity: none, odd, even, mark, space")
	agentCmd.Flags().Int("stopbits", 1, "Stop bits: 1 or 2")
	agentCmd.Flags().String("listen", ":5760", "Listen address (server mode, raw TCP)")
	agentCmd.Flags().String("remote", "", "Remote address (client mode): host:port")
	agentCmd.Flags().String("virtual", "", "Virtual COM port name (client mode): COM5, /dev/ttyV0, etc.")
	agentCmd.Flags().String("server-url", "", "Tether server URL (client/server mode): ws://host:port")
	agentCmd.Flags().String("token", "", "Tether server authentication token")
	agentCmd.Flags().String("remote-client", "", "Remote client ID to connect to (client mode)")
	agentCmd.Flags().Bool("tls", false, "Enable TLS for direct TCP connections")
	agentCmd.Flags().Bool("insecure", false, "Skip TLS certificate verification")
	agentCmd.Flags().Bool("rfc2217", false, "Enable RFC 2217 protocol support")

	rootCmd.AddCommand(agentCmd)

	// Ports command
	portsCmd := &cobra.Command{
		Use:   "ports",
		Short: "List available serial ports",
		RunE: func(cmd *cobra.Command, args []string) error {
			return agent.ListPorts()
		},
	}
	rootCmd.AddCommand(portsCmd)

	// Version command
	rootCmd.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("usbredirect v%s\n", version)
		},
	})

	// Health check command
	healthCmd := &cobra.Command{
		Use:   "health",
		Short: "Check if a remote server is reachable",
		RunE:  runHealthCheck,
	}
	healthCmd.Flags().String("addr", "", "Remote address to check (host:port)")
	healthCmd.Flags().Bool("tls", false, "Use TLS for the check")
	rootCmd.AddCommand(healthCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func runAgent(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(cfgFile, cmd.Flags())
	if err != nil {
		return fmt.Errorf("config error: %w", err)
	}

	// Setup signal handling for graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	agt, err := agent.New(cfg)
	if err != nil {
		return fmt.Errorf("agent init error: %w", err)
	}

	// Run agent in a goroutine
	errCh := make(chan error, 1)
	go func() {
		errCh <- agt.Run()
	}()

	select {
	case err := <-errCh:
		return err
	case sig := <-sigCh:
		fmt.Printf("\nReceived signal %v, shutting down gracefully...\n", sig)
		return nil
	}
}

func runHealthCheck(cmd *cobra.Command, args []string) error {
	addr, _ := cmd.Flags().GetString("addr")
	if addr == "" {
		return fmt.Errorf("--addr is required")
	}
	fmt.Printf("Checking %s...\n", addr)
	if err := agent.HealthCheck(nil, addr); err != nil {
		fmt.Printf("UNREACHABLE: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("OK")
	return nil
}