package main

import (
	"fmt"
	"os"

	"github.com/r3dc0d4/usbredirect/internal/agent"
	"github.com/r3dc0d4/usbredirect/internal/config"
	"github.com/spf13/cobra"
)

var (
	version = "0.1.0"
	cfgFile string
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "usbredirect",
		Short: "USB Redirect — COM Port Network Redirector",
		Long: `USB Redirect redirects serial (COM) ports over the network.
It allows software on one machine to read/write a serial device
connected to a different machine over TCP/WebSocket.

Usage:
  usbredirect agent --mode server --port COM3 --baud 9600 --listen :5760
  usbredirect agent --mode client --remote 192.168.1.50:5760 --virtual COM5`,
		Version: version,
	}

	// Global flags
	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", "config file (default: ./usbredirect.yaml)")

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
	agentCmd.Flags().Bool("tls", false, "Enable TLS for TCP connections")
	agentCmd.Flags().Bool("rfc2217", false, "Enable RFC 2217 protocol support")

	rootCmd.AddCommand(agentCmd)

	// Version command
	rootCmd.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("usbredirect v%s\n", version)
		},
	})

	// Ports command — list available serial ports
	portsCmd := &cobra.Command{
		Use:   "ports",
		Short: "List available serial ports",
		RunE:  runPorts,
	}
	rootCmd.AddCommand(portsCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func runAgent(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(cfgFile, cmd.Flags())
	if err != nil {
		return fmt.Errorf("config error: %w", err)
	}

	agt, err := agent.New(cfg)
	if err != nil {
		return fmt.Errorf("agent init error: %w", err)
	}

	return agt.Run()
}

func runPorts(cmd *cobra.Command, args []string) error {
	return agent.ListPorts()
}