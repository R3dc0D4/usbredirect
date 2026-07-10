package config

import (
	"fmt"
	"time"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// Config holds all configuration for the USB redirect agent.
type Config struct {
	Mode      string      `mapstructure:"mode"`      // "server" or "client"
	Serial    SerialConfig `mapstructure:"serial"`
	Virtual   VirtualConfig `mapstructure:"virtual"`
	Server    ServerConfig  `mapstructure:"server"`
	Network   NetworkConfig `mapstructure:"network"`
	RFC2217   bool         `mapstructure:"rfc2217"`
}

type SerialConfig struct {
	Port     string `mapstructure:"port"`
	Baud     int    `mapstructure:"baud"`
	DataBits int    `mapstructure:"databits"`
	Parity   string `mapstructure:"parity"`
	StopBits int    `mapstructure:"stopbits"`
}

type VirtualConfig struct {
	Port string `mapstructure:"port"` // COM5, /dev/ttyV0, etc.
}

type ServerConfig struct {
	URL           string        `mapstructure:"url"`             // Tether server URL
	Token         string        `mapstructure:"token"`           // Auth token
	RemoteClient  string        `mapstructure:"remote_client"`  // Target client ID (client mode)
	Reconnect     ReconnectConfig `mapstructure:"reconnect"`
}

type ReconnectConfig struct {
	Initial    time.Duration `mapstructure:"initial"`
	Max        time.Duration `mapstructure:"max"`
	Multiplier float64       `mapstructure:"multiplier"`
}

type NetworkConfig struct {
	Listen string `mapstructure:"listen"` // TCP listen address (server mode)
	Remote string `mapstructure:"remote"` // TCP remote address (client mode)
	TLS    bool   `mapstructure:"tls"`
}

// DefaultConfig returns a config with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		Mode: "server",
		Serial: SerialConfig{
			Baud:     9600,
			DataBits: 8,
			Parity:   "none",
			StopBits: 1,
		},
		Network: NetworkConfig{
			Listen: ":5760",
		},
		Server: ServerConfig{
			Reconnect: ReconnectConfig{
				Initial:    1 * time.Second,
				Max:        30 * time.Second,
				Multiplier: 2.0,
			},
		},
	}
}

// Load reads config from file and flags.
func Load(cfgFile string, flags *pflag.FlagSet) (*Config, error) {
	cfg := DefaultConfig()
	v := viper.New()

	// Config file
	if cfgFile != "" {
		v.SetConfigFile(cfgFile)
	} else {
		v.SetConfigName("usbredirect")
		v.AddConfigPath(".")
		v.AddConfigPath("/etc/usbredirect")
		v.AddConfigPath("$HOME/.config/usbredirect")
	}

	// Bind flags
	if flags != nil {
		v.BindPFlag("mode", flags.Lookup("mode"))
		v.BindPFlag("serial.port", flags.Lookup("port"))
		v.BindPFlag("serial.baud", flags.Lookup("baud"))
		v.BindPFlag("serial.databits", flags.Lookup("databits"))
		v.BindPFlag("serial.parity", flags.Lookup("parity"))
		v.BindPFlag("serial.stopbits", flags.Lookup("stopbits"))
		v.BindPFlag("network.listen", flags.Lookup("listen"))
		v.BindPFlag("network.remote", flags.Lookup("remote"))
		v.BindPFlag("virtual.port", flags.Lookup("virtual"))
		v.BindPFlag("server.url", flags.Lookup("server-url"))
		v.BindPFlag("server.token", flags.Lookup("token"))
		v.BindPFlag("server.remote_client", flags.Lookup("remote-client"))
		v.BindPFlag("network.tls", flags.Lookup("tls"))
		v.BindPFlag("rfc2217", flags.Lookup("rfc2217"))
	}

	// Read config file (optional)
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("config file error: %w", err)
		}
		// Config file not found is OK, use defaults + flags
	}

	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("config unmarshal error: %w", err)
	}

	// Validate
	if cfg.Mode != "server" && cfg.Mode != "client" {
		return nil, fmt.Errorf("mode must be 'server' or 'client', got: %s", cfg.Mode)
	}

	if cfg.Mode == "server" && cfg.Serial.Port == "" {
		return nil, fmt.Errorf("serial port is required in server mode (use --port or config)")
	}

	if cfg.Mode == "client" && cfg.Network.Remote == "" && cfg.Server.URL == "" {
		return nil, fmt.Errorf("remote address or server URL is required in client mode")
	}

	return cfg, nil
}