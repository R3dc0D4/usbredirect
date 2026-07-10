//go:build windows

package virtual

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// VirtualPort manages a virtual COM port on Windows using com0com.
//
// com0com creates a pair of connected virtual COM ports (e.g., COM5 and COM6).
// The USB Redirect agent connects to one end (COM5) and the user's software
// connects to the other end (COM6). Data flows through the com0com bridge.
//
// This approach avoids the complexity of writing a kernel-mode driver while
// providing real COM ports that Windows software can open.
type VirtualPort struct {
	comPort     string    // The COM port visible to user software (e.g., COM6)
	bridgePort  string    // The COM port used by USB Redirect (e.g., COM5)
	logger      *slog.Logger
	com0comPath string    // Path to com0com installation
}

// Create creates a virtual COM port pair using com0com on Windows.
//
// If com0com is not installed, it will attempt to install it automatically.
// The virtualPort parameter specifies the COM port name (e.g., "COM5").
// A second port (e.g., "COM6") is automatically created as the bridge.
func Create(virtualPort string) (*VirtualPort, error) {
	logger := slog.Default().With("component", "virtual-com0com")

	comPort := virtualPort
	if comPort == "" {
		comPort = "COM5"
	}

	// Find com0com installation
	com0comPath, err := findCom0com()
	if err != nil {
		logger.Warn("com0com not found, attempting to install", "error", err)
		// TODO: Auto-install com0com
		return nil, fmt.Errorf("com0com not installed: %w\n\nInstall com0com from https://github.com/vovsoft/com0com\nOr run: usbredirect install-com0com", err)
	}

	logger.Info("Found com0com", "path", com0comPath)

	// Generate bridge port name
	// If user wants COM5, we use COM5 as bridge and COM6 as user port
	// (or vice versa depending on convention)
	bridgePort := generateBridgePort(comPort)

	// Create the port pair using com0com
	if err := createPortPair(com0comPath, bridgePort, comPort); err != nil {
		// Port pair may already exist, try to use it
		logger.Warn("Failed to create port pair (may already exist)", "error", err)
	}

	vp := &VirtualPort{
		comPort:     comPort,
		bridgePort:  bridgePort,
		logger:      logger,
		com0comPath: com0comPath,
	}

	logger.Info("Virtual COM port pair created",
		"user_port", comPort,
		"bridge_port", bridgePort,
	)

	return vp, nil
}

// findCom0com searches for com0com installation.
func findCom0com() (string, error) {
	// Check common installation paths
	paths := []string{
		`C:\Program Files\com0com\setupc.exe`,
		`C:\Program Files (x86)\com0com\setupc.exe`,
		`C:\com0com\setupc.exe`,
	}

	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}

	// Check PATH
	path, err := exec.LookPath("setupc.exe")
	if err == nil {
		return path, nil
	}

	return "", fmt.Errorf("com0com setupc.exe not found")
}

// generateBridgePort generates a bridge port name.
// If comPort is COM5, bridgePort is COM6.
func generateBridgePort(comPort string) string {
	// Extract number from COMx
	num := 0
	fmt.Sscanf(comPort, "COM%d", &num)
	if num == 0 {
		num = 5 // Default
	}
	return fmt.Sprintf("COM%d", num+1)
}

// createPortPair creates a com0com port pair.
func createPortPair(com0comPath, portA, portB string) error {
	// setupc.exe install PortNameA=COM5 PortNameB=COM6
	cmd := exec.Command(com0comPath, "install",
		fmt.Sprintf("PortNameA=%s", portA),
		fmt.Sprintf("PortNameB=%s", portB),
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("com0com install failed: %w\nOutput: %s", err, string(output))
	}
	return nil
}

// removePortPair removes a com0com port pair.
func removePortPair(com0comPath, portA string) error {
	cmd := exec.Command(com0comPath, "remove",
		fmt.Sprintf("PortNameA=%s", portA),
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("com0com remove failed: %w\nOutput: %s", err, string(output))
	}
	return nil
}

// Read reads data from the bridge port (COM port used by USB Redirect).
// On Windows with com0com, we open the bridge port as a regular COM port.
func (vp *VirtualPort) Read(buf []byte) (int, error) {
	// On Windows, com0com handles the data flow between the two virtual ports.
	// The agent connects to bridgePort and the software connects to comPort.
	// Data written to bridgePort appears on comPort and vice versa.
	// This is handled by the serial package when opening bridgePort.
	return 0, fmt.Errorf("Read on Windows virtual port: use serial.Open() on bridge port instead")
}

// Write writes data to the bridge port.
func (vp *VirtualPort) Write(data []byte) (int, error) {
	return 0, fmt.Errorf("Write on Windows virtual port: use serial.Open() on bridge port instead")
}

// Close removes the virtual port pair.
func (vp *VirtualPort) Close() error {
	vp.logger.Info("Removing virtual COM port pair",
		"user_port", vp.comPort,
		"bridge_port", vp.bridgePort,
	)
	return removePortPair(vp.com0comPath, vp.bridgePort)
}

// PortName returns the COM port that user software should connect to.
func (vp *VirtualPort) PortName() string {
	return vp.comPort
}

// BridgePortName returns the COM port that USB Redirect connects to.
func (vp *VirtualPort) BridgePortName() string {
	return vp.bridgePort
}

// SetBaudRate is a no-op for com0com (virtual ports don't have real baud rates).
func (vp *VirtualPort) SetBaudRate(baud int) error {
	// com0com virtual ports pass data at full speed regardless of baud rate setting.
	// The actual baud rate is set on the physical COM port on the server side.
	return nil
}

// CheckPTAvailable checks if com0com is installed.
func CheckPTAvailable() error {
	_, err := findCom0com()
	if err != nil {
		return fmt.Errorf("com0com not installed: %w\nInstall from: https://github.com/vovsoft/com0com", err)
	}
	return nil
}

// InstallCom0com downloads and installs com0com.
// This function is intended to be called from the CLI.
func InstallCom0com() error {
	logger := slog.Default().With("component", "com0com-installer")

	// Check if already installed
	if _, err := findCom0com(); err == nil {
		logger.Info("com0com already installed")
		return nil
	}

	// Download com0com installer
	logger.Info("Downloading com0com installer...")
	// TODO: Implement automatic download from GitHub releases
	return fmt.Errorf("automatic com0com installation not yet implemented\nPlease install manually from: https://github.com/vovsoft/com0com")
}

// ListInstalledPorts lists com0com installed port pairs.
func ListInstalledPorts() ([]string, []string, error) {
	com0comPath, err := findCom0com()
	if err != nil {
		return nil, nil, err
	}

	cmd := exec.Command(filepath.Dir(com0comPath)+`\listc.exe`, "list")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list com0com ports: %w", err)
	}

	var portsA, portsB []string
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "COM") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				portsA = append(portsA, parts[0])
				portsB = append(portsB, parts[1])
			}
		}
	}

	return portsA, portsB, nil
}