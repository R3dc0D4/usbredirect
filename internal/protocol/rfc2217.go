package protocol

// RFC 2217 Telnet Com Port Control Option
// RFC: https://www.rfc-editor.org/rfc/rfc2217.html
//
// This implements the RFC 2217 protocol for negotiating serial port
// settings over a network connection. The client (software side) can
// request changes to baud rate, data bits, parity, stop bits, and
// flow control, which are forwarded to the server (device side) to
// reconfigure the physical serial port.

// ControlCommand represents RFC 2217 control commands.
type ControlCommand string

const (
	// Baud rate control
	CmdSetBaud       ControlCommand = "set_baud"
	CmdGetBaud       ControlCommand = "get_baud"

	// Data bits control
	CmdSetDataBits   ControlCommand = "set_databits"
	CmdGetDataBits   ControlCommand = "get_databits"

	// Parity control
	CmdSetParity     ControlCommand = "set_parity"
	CmdGetParity     ControlCommand = "get_parity"

	// Stop bits control
	CmdSetStopBits   ControlCommand = "set_stopbits"
	CmdGetStopBits   ControlCommand = "get_stopbits"

	// Flow control
	CmdSetFlowControl ControlCommand = "set_flowcontrol"
	CmdGetFlowControl ControlCommand = "get_flowcontrol"

	// Line state signals
	CmdSetDTR        ControlCommand = "set_dtr"
	CmdSetRTS        ControlCommand = "set_rts"
	CmdGetLineState  ControlCommand = "get_linestate"

	// Modem signals
	CmdGetModemSignals ControlCommand = "get_modem_signals"

	// Break
	CmdBreak         ControlCommand = "break"

	// Purge
	CmdPurge         ControlCommand = "purge"
)

// PortConfig represents the current serial port configuration.
type PortConfig struct {
	Baud       int    `json:"baud"`
	DataBits   int    `json:"dataBits"`
	Parity     string `json:"parity"`       // "none", "odd", "even", "mark", "space"
	StopBits   int    `json:"stopBits"`
	FlowControl string `json:"flowControl"` // "none", "xon/xoff", "rts/cts"
}

// ControlMessage represents an RFC 2217 control message
// sent between client and server (via Tether or direct TCP).
type ControlMessage struct {
	Type    ControlCommand `json:"type"`              // Command type
	Value   interface{}    `json:"value,omitempty"`    // Command value
	Port    string          `json:"port,omitempty"`     // Serial port name (server side)
	Config  *PortConfig     `json:"config,omitempty"`   // Full port config (for set_config)
}

// PortConfigResponse represents the response to a port configuration request.
type PortConfigResponse struct {
	Success bool       `json:"success"`
	Config  PortConfig `json:"config"`
	Error   string     `json:"error,omitempty"`
}

// ParityString converts integer parity to string.
// RFC 2217 parity values: 0=None, 1=Odd, 2=Even, 3=Mark, 4=Space
func ParityString(p int) string {
	switch p {
	case 0:
		return "none"
	case 1:
		return "odd"
	case 2:
		return "even"
	case 3:
		return "mark"
	case 4:
		return "space"
	default:
		return "none"
	}
}

// ParityInt converts string parity to integer (RFC 2217 values).
func ParityInt(p string) int {
	switch p {
	case "none":
		return 0
	case "odd":
		return 1
	case "even":
		return 2
	case "mark":
		return 3
	case "space":
		return 4
	default:
		return 0
	}
}

// StopBitsString converts integer stop bits to string.
func StopBitsString(sb int) string {
	switch sb {
	case 1:
		return "1"
	case 2:
		return "2"
	default:
		return "1"
	}
}

// FlowControlString converts integer flow control to string.
// RFC 2217 flow control: 0=None, 1=XON/XOFF, 2=RST/CTS, 3=Both
func FlowControlString(fc int) string {
	switch fc {
	case 0:
		return "none"
	case 1:
		return "xon/xoff"
	case 2:
		return "rts/cts"
	case 3:
		return "both"
	default:
		return "none"
	}
}

// DefaultPortConfig returns the default port configuration.
func DefaultPortConfig() *PortConfig {
	return &PortConfig{
		Baud:       9600,
		DataBits:   8,
		Parity:     "none",
		StopBits:   1,
		FlowControl: "none",
	}
}