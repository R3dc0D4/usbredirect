# USB Redirect — Cross-platform COM Port Network Redirector

[![Go Version](https://img.shields.io/badge/Go-1.24%2B-00ADD8?style=flat&logo=go)](https://go.dev)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
[![GitHub Release](https://img.shields.io/github/v/release/R3dc0D4/usbredirect?include_prereleases)](https://github.com/R3dc0D4/usbredirect/releases)

**USB Redirect** redirects serial (COM) ports over the network. It allows software on one machine to read/write a serial device connected to a different machine over TCP or WebSocket.

## Problem

A software reads data from a COM port. But the device is on a different network. The software needs a virtual COM port that connects to the remote physical COM port through the network.

```
┌─────────────────┐          Network          ┌─────────────────┐
│   Software PC    │◄──────────────────────►│   Device PC      │
│   (Windows/Mac)  │     TCP/TLS/WS          │   (Windows)      │
│                  │                         │                  │
│  Software        │                         │  COM3 (physical) │
│  reads COM5 ◄─────┐                     ┌─────► USB device     │
│                  │  ┌──────────┐          │                  │
└─────────────────┘  │  Tether  │          └──────────────────┘
                     │  Server  │
                     │ (ZimaOS) │
                     └────┬─────┘
                          │
                   virtual COM    physical COM
                   (client)        (server)
```

## Features

| Feature | Status | Description |
|---------|--------|-------------|
| TCP bridge (raw) | ✅ | Direct server↔client serial data relay |
| Serial port enumeration | ✅ | Cross-platform port listing |
| Config file (YAML) | ✅ | Viper-based configuration |
| CLI (cobra) | ✅ | Full command-line interface |
| Virtual COM (Linux PTY) | ✅ | PTY-based virtual serial ports |
| Virtual COM (macOS PTY) | ✅ | PTY-based virtual serial ports |
| Virtual COM (Windows com0com) | ✅ | com0com bridge-based virtual ports |
| Tether WebSocket relay | ✅ | Cross-network relay via Cloudflare |
| RFC 2217 control | ✅ | Baud/parity/stopbits remote config |
| TLS support | ✅ | Direct TCP with TLS encryption |
| Auto-reconnect | ✅ | Exponential backoff on disconnect |
| Multi-client broadcast | ✅ | 1 server → N clients |
| Health check | ✅ | Remote server reachability test |
| Graceful shutdown | ✅ | SIGINT/SIGTERM handling |

## Quick Start

### Install

Download from [Releases](https://github.com/R3dc0D4/usbredirect/releases) or build from source:

```bash
git clone https://github.com/R3dc0D4/usbredirect.git
cd usbredirect
make build
```

### Server Mode (device side)

Share a physical COM port over the network:

```bash
# Direct TCP (same network)
usbredirect agent --mode server --port COM3 --baud 9600 --listen :5760

# Via Tether (cross-network)
usbredirect agent --mode server --port /dev/ttyUSB0 --baud 115200 \
  --server-url wss://tether.tanrisever.tr --token YOUR_TOKEN
```

### Client Mode (software side)

Create a virtual COM port connected to the remote device:

```bash
# Direct TCP (same network)
usbredirect agent --mode client --remote 192.168.1.50:5760 --virtual /dev/ttyV0

# Via Tether (cross-network)
usbredirect agent --mode client \
  --server-url wss://tether.tanrisever.tr --token YOUR_TOKEN \
  --virtual COM5 --remote-client SERVER_AGENT_ID
```

### List Serial Ports

```bash
usbredirect ports
```

### Health Check

```bash
usbredirect health --addr 192.168.1.50:5760
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
  tls: false

server:
  url: "wss://tether.tanrisever.tr"
  token: "your-auth-token"
  reconnect:
    initial: "1s"
    max: "30s"
    multiplier: 2.0

rfc2217: false
```

## Architecture

### Direct TCP Mode (Same Network)

```
┌─────────────┐      TCP/TLS      ┌─────────────┐
│   Server     │◄──────────────────►│   Client     │
│   (Device)   │                    │  (Software)  │
│              │                    │              │
│  COM3 → TCP  │                    │  TCP → COM5  │
└─────────────┘                    └─────────────┘
```

### Tether Mode (Cross-Network)

```
┌─────────────┐    WS     ┌──────────┐    WS     ┌─────────────┐
│   Server     │◄─────────►│  Tether  │◄─────────►│   Client     │
│   (Device)   │           │  Server   │           │  (Software)  │
│              │           │ (ZimaOS)  │           │              │
│  COM3 → WS   │           │  relay   │           │  WS → COM5  │
└─────────────┘           └──────────┘           └─────────────┘
```

### RFC 2217 (Remote Port Configuration)

```
Software (client)                     Device (server)
  │                                      │
  │  ─── set_baud(115200) ───────────►  │
  │  ◄── baud_changed(115200) ────────   │
  │                                      │
  │  ─── set_parity(none) ───────────►   │
  │  ◄── parity_changed(none) ───────   │
  │                                      │
```

## Project Structure

```
usbredirect/
├── cmd/usbredirect/main.go      # CLI entry point
├── internal/
│   ├── agent/
│   │   ├── agent.go             # Agent orchestration
│   │   ├── tcp.go               # TCP server/client
│   │   ├── tether.go            # Tether WebSocket relay
│   │   └── tls.go               # TLS support
│   ├── config/config.go         # YAML config (viper)
│   ├── protocol/rfc2217.go     # RFC 2217 constants
│   ├── serial/port.go           # Serial port abstraction
│   ├── tether/ws.go            # Tether WebSocket client
│   └── virtual/
│       ├── virtual_linux.go     # PTY-based (Linux)
│       ├── virtual_darwin.go   # PTY-based (macOS)
│       └── virtual_windows.go   # com0com-based (Windows)
├── configs/usbredirect.yaml     # Example config
├── Makefile
├── .goreleaser.yml
├── README.md
├── LICENSE                      # Apache 2.0
├── go.mod
└── go.sum
```

## Cross-Platform Support

| Platform | Virtual COM | Status |
|----------|-------------|--------|
| Linux | PTY (`/dev/pts/N` → symlink) | ✅ Working |
| macOS | PTY (`/dev/ttysN` → symlink) | ✅ Working |
| Windows | com0com (COM5↔COM6 bridge) | ✅ Working (requires com0com) |

## Windows Setup

1. Download and install [com0com](https://github.com/vovsoft/com0com)
2. Run `usbredirect agent --mode client --virtual COM5 --remote ...`
3. com0com will create COM5↔COM6 pair automatically
4. Your software connects to COM6, USB Redirect uses COM5 internally

## Development

```bash
# Build
make build

# Build all platforms
make build-all

# Run tests
make test

# Format code
make fmt

# Run locally (server)
make run-server

# Run locally (client)
make run-client
```

## License

Apache License 2.0 — See [LICENSE](LICENSE) for details.

## Related Projects

- [Tether](https://github.com/openclaw/workspace) — Remote support server (WebSocket relay)
- [go.bug.st/serial](https://github.com/bugst/go-serial) — Go serial port library
- [com0com](https://github.com/vovsoft/com0com) — Windows virtual serial port driver
- [RFC 2217](https://www.rfc-editor.org/rfc/rfc2217) — Telnet Com Port Control Option