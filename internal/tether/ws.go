package tether

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// MessageType represents the type of message sent to/received from Tether server.
type MessageType string

const (
	// Registration
	TypeSerialRegister   MessageType = "serial_register"
	TypeSerialRegistered MessageType = "serial_registered"

	// Data relay
	TypeSerialData MessageType = "serial_data"

	// Control (RFC 2217)
	TypeSerialControl MessageType = "serial_control"

	// Pairing
	TypeSerialConnect    MessageType = "serial_connect"
	TypeSerialDisconnect MessageType = "serial_disconnect"
	TypeSerialPaired     MessageType = "serial_paired"
	TypeSerialUnpaired   MessageType = "serial_unpaired"
	TypeSerialError      MessageType = "serial_error"

	// General
	TypeHeartbeat    MessageType = "heartbeat"
	TypeHeartbeatAck MessageType = "heartbeat_ack"
	TypeWelcome      MessageType = "welcome"
)

// Message represents a JSON message sent to/received from Tether server.
type Message struct {
	Type       MessageType `json:"type"`
	ClientID   string      `json:"clientId,omitempty"`
	ShortID    string      `json:"shortId,omitempty"`
	Mode       string      `json:"mode,omitempty"`        // "server" or "client"
	PortName   string      `json:"portName,omitempty"`    // e.g., "COM3", "/dev/ttyUSB0"
	Baud       int         `json:"baud,omitempty"`
	Parity     string      `json:"parity,omitempty"`
	DataBits   int         `json:"dataBits,omitempty"`
	StopBits   int         `json:"stopBits,omitempty"`
	TargetServer string    `json:"targetServer,omitempty"` // Client mode: server to connect to
	PairID     string      `json:"pairId,omitempty"`
	Role       string      `json:"role,omitempty"`        // "server" or "client"
	PartnerID  string      `json:"partnerId,omitempty"`
	PartnerPort string     `json:"partnerPort,omitempty"`
	SubType    string      `json:"subType,omitempty"`      // RFC 2217 sub-type
	Value      interface{} `json:"value,omitempty"`
	Error      string      `json:"error,omitempty"`
	Encoding   string      `json:"encoding,omitempty"`    // "base64" for serial_data
	Data       string      `json:"data,omitempty"`         // Base64-encoded data
	Reason     string      `json:"reason,omitempty"`
}

// Client connects to a Tether server and relays serial data.
type Client struct {
	serverURL string
	token     string
	conn      *websocket.Conn
	clientID  string
	shortID   string
	logger    *slog.Logger

	// Serial data callback
	onData     func(data []byte)
	onControl  func(msg *Message)
	onPaired  func(msg *Message)
	onUnpaired func(reason string)
	onError   func(err string)

	// Connection state
	connected bool
	mu        sync.Mutex
	cancel    context.CancelFunc
}

// NewClient creates a new Tether WebSocket client.
func NewClient(serverURL, token string) *Client {
	return &Client{
		serverURL: serverURL,
		token:     token,
		logger:    slog.Default().With("component", "tether"),
	}
}

// OnData sets the callback for incoming serial data (binary).
func (c *Client) OnData(fn func(data []byte)) { c.onData = fn }

// OnControl sets the callback for serial control messages (RFC 2217).
func (c *Client) OnControl(fn func(msg *Message)) { c.onControl = fn }

// OnPaired sets the callback for when pairing is established.
func (c *Client) OnPaired(fn func(msg *Message)) { c.onPaired = fn }

// OnUnpaired sets the callback for when pairing is broken.
func (c *Client) OnUnpaired(fn func(reason string)) { c.onUnpaired = fn }

// OnError sets the callback for serial errors.
func (c *Client) OnError(fn func(err string)) { c.onError = fn }

// Connect connects to the Tether WebSocket server.
func (c *Client) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Parse URL and add token
	u, err := url.Parse(c.serverURL)
	if err != nil {
		return fmt.Errorf("invalid server URL: %w", err)
	}

	// Add token as query parameter
	q := u.Query()
	q.Set("clientId", c.token) // Use token as persistent client ID for now
	u.RawQuery = q.Encode()

	// Determine scheme and path
	wsURL := u.String()
	if strings.HasPrefix(wsURL, "https://") {
		wsURL = strings.Replace(wsURL, "https://", "wss://", 1)
	} else if strings.HasPrefix(wsURL, "http://") {
		wsURL = strings.Replace(wsURL, "http://", "ws://", 1)
	}

	// Ensure /ws path is appended
	if !strings.Contains(wsURL, "/ws") {
		if strings.Contains(wsURL, "?") {
			wsURL = strings.Replace(wsURL, "?", "/ws?", 1)
		} else {
			wsURL = wsURL + "/ws"
		}
	}

	c.logger.Info("Connecting to Tether server", "url", wsURL)

	headers := http.Header{}
	if c.token != "" {
		headers.Set("Authorization", "Bearer "+c.token)
	}

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, _, err := dialer.DialContext(ctx, wsURL, headers)
	if err != nil {
		return fmt.Errorf("failed to connect to Tether server: %w", err)
	}

	c.conn = conn
	c.connected = true

	// Start message reader
	ctx2, cancel := context.WithCancel(ctx)
	c.cancel = cancel
	go c.readMessages(ctx2)

	return nil
}

// Register registers as a serial port agent.
func (c *Client) Register(mode, portName string, baud int, parity string, dataBits, stopBits int) error {
	msg := &Message{
		Type:      TypeSerialRegister,
		Mode:      mode,
		PortName:  portName,
		Baud:      baud,
		Parity:    parity,
		DataBits:  dataBits,
		StopBits:  stopBits,
	}
	return c.SendJSON(msg)
}

// ConnectToServer requests pairing with a specific server agent.
func (c *Client) ConnectToServer(targetServer string) error {
	msg := &Message{
		Type:         TypeSerialConnect,
		TargetServer: targetServer,
	}
	return c.SendJSON(msg)
}

// DisconnectSerial disconnects from paired agent.
func (c *Client) DisconnectSerial() error {
	msg := &Message{
		Type: TypeSerialDisconnect,
	}
	return c.SendJSON(msg)
}

// SendData sends binary serial data to the paired agent.
func (c *Client) SendData(data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil || !c.connected {
		return fmt.Errorf("not connected")
	}

	return c.conn.WriteMessage(websocket.BinaryMessage, data)
}

// SendControl sends an RFC 2217 control message.
func (c *Client) SendControl(subType string, value interface{}) error {
	msg := &Message{
		Type:    TypeSerialControl,
		SubType: subType,
		Value:   value,
	}
	return c.SendJSON(msg)
}

// SendHeartbeat sends a heartbeat message.
func (c *Client) SendHeartbeat() error {
	msg := &Message{
		Type: TypeHeartbeat,
	}
	return c.SendJSON(msg)
}

// SendJSON sends a JSON message to the server.
func (c *Client) SendJSON(msg *Message) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil || !c.connected {
		return fmt.Errorf("not connected")
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	return c.conn.WriteMessage(websocket.TextMessage, data)
}

// readMessages reads messages from the WebSocket connection.
func (c *Client) readMessages(ctx context.Context) {
	defer c.Close()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		messageType, data, err := c.conn.ReadMessage()
		if err != nil {
			c.logger.Error("Read error", "error", err)
			c.connected = false
			return
		}

		switch messageType {
		case websocket.TextMessage:
			var msg Message
			if err := json.Unmarshal(data, &msg); err != nil {
				c.logger.Warn("Failed to parse message", "error", err)
				continue
			}
			c.handleMessage(&msg)

		case websocket.BinaryMessage:
			// Binary frame = serial data from partner
			if c.onData != nil {
				c.onData(data)
			}
		}
	}
}

// handleMessage processes incoming JSON messages.
func (c *Client) handleMessage(msg *Message) {
	switch msg.Type {
	case TypeWelcome:
		c.clientID = msg.ClientID
		c.shortID = msg.ShortID
		c.logger.Info("Welcome from Tether server", "clientId", msg.ClientID, "shortId", msg.ShortID)

	case TypeSerialRegistered:
		c.logger.Info("Registered as serial agent", "mode", msg.Mode, "portName", msg.PortName)

	case TypeSerialPaired:
		c.logger.Info("Serial pair established", "pairId", msg.PairID, "role", msg.Role, "partnerId", msg.PartnerID, "partnerPort", msg.PartnerPort)
		if c.onPaired != nil {
			c.onPaired(msg)
		}

	case TypeSerialUnpaired:
		c.logger.Info("Serial pair broken", "reason", msg.Reason)
		if c.onUnpaired != nil {
			c.onUnpaired(msg.Reason)
		}

	case TypeSerialError:
		c.logger.Error("Serial error from server", "error", msg.Error)
		if c.onError != nil {
			c.onError(msg.Error)
		}

	case TypeSerialControl:
		c.logger.Debug("Serial control message", "subType", msg.SubType, "value", msg.Value)
		if c.onControl != nil {
			c.onControl(msg)
		}

	case TypeHeartbeatAck:
		// Heartbeat acknowledged, connection is alive

	default:
		c.logger.Debug("Unhandled message type", "type", msg.Type)
	}
}

// Close closes the WebSocket connection.
func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.cancel != nil {
		c.cancel()
	}
	if c.conn != nil {
		c.conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
		c.conn.Close()
	}
	c.connected = false
}

// IsConnected returns whether the client is connected.
func (c *Client) IsConnected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.connected
}

// GetClientID returns the client ID assigned by the server.
func (c *Client) GetClientID() string {
	return c.clientID
}

// GetShortID returns the short ID assigned by the server.
func (c *Client) GetShortID() string {
	return c.shortID
}