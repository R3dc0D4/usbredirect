# USB Redirect — COM Port Network Redirector

USB Redirect redirects serial (COM) ports over the network. It allows software on one machine to read/write a serial device connected to a different machine.

## Features

- 🔄 **Raw TCP Bridge** — Redirect serial port data over TCP
- 🌐 **Tether Server Integration** — Relay via WebSocket (planned)
- 📡 **RFC 2217** — Dynamic baud rate/parity changes over network (planned)
- 🔌 **Virtual COM** — Create virtual serial ports (Linux/macOS PTY, Windows driver planned)
- 🔒 **TLS Encryption** — Secure connections (planned)
- 🔄 **Auto-Reconnect** — Exponential backoff on disconnect (planned)
- 🖥️ **Cross-Platform** — Windows, Linux, macOS

## Quick Start

### Server Mode (device side)

Connect a serial device and make it available over TCP:

```bash
# List available serial ports
usbredirect ports

# Start server — share COM3 over TCP port 5760
usbredirect agent --mode server --port COM3 --baud 9600 --listen :5760

# Or with config file
usbredirect agent -c configs/usbredirect.yaml
```

### Client Mode (software side)

Connect to the remote serial port:

```bash
# Connect to remote server (raw TCP, MVP uses stdin/stdout)
usbredirect agent --mode client --remote 192.168.1.50:5760

# With virtual COM port (planned)
usbredirect agent --mode client --remote 192.168.1.50:5760 --virtual COM5
```

## Configuration

See [`configs/usbredirect.yaml`](configs/usbredirect.yaml) for a full example.

```yaml
mode: server
serial:
  port: "/dev/ttyUSB0"
  baud: 9600
  databits: 8
  parity: "none"
  stopbits: 1
network:
  listen: ":5760"
```

## Architecture

```
┌─────────────────┐          TCP          ┌─────────────────┐
│   DEVICE PC      │◄──────────────────────►│   SOFTWARE PC   │
│                  │                         │                  │
│  usbredirect     │                         │  usbredirect     │
│  server          │                         │  client          │
│  (reads COM3)    │                         │  (creates COM5) │
└─────────────────┘                         └─────────────────┘
```

## Development Status

| Feature | Status |
|---------|--------|
| TCP bridge (server/client) | ✅ MVP |
| Serial port enumeration | ✅ |
| Config file (YAML) | ✅ |
| CLI (cobra) | ✅ |
| Virtual COM (Linux/macOS PTY) | 🔄 Planned |
| Tether WebSocket relay | 📋 Planned |
| RFC 2217 support | 📋 Planned |
| Windows kernel driver | 📋 Planned |
| TLS encryption | 📋 Planned |
| Auto-reconnect | 📋 Planned |

## Build

```bash
# Build for current platform
go build -o usbredirect ./cmd/usbredirect

# Cross-compile
GOOS=windows GOARCH=amd64 go build -o usbredirect.exe ./cmd/usbredirect
GOOS=linux GOARCH=amd64 go build -o usbredirect-linux ./cmd/usbredirect
GOOS=darwin GOARCH=arm64 go build -o usbredirect-macos ./cmd/usbredirect
```

## License

Apache License 2.0