//go:build linux

package virtual

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
	"syscall"
	"unsafe"
)

// VirtualPort manages a PTY-based virtual serial port on Linux.
type VirtualPort struct {
	ptyFile  *os.File
	ptyName  string
	linkPath string
	logger   *slog.Logger
}

// Linux ioctl constants for PTY management
const (
	TIOCGPTN    = 0x80045430 // Get PTY number
	TIOCSPTLCK  = 0x40045431 // Lock/unlock PTY
)

// grantpt changes the ownership and permissions of the slave PTY.
func grantpt(f *os.File) error {
	// On Linux, grantpt() is typically a no-op or handled by kernel.
	// The slave PTY is created with correct permissions by default.
	// We just need to make sure it's accessible.
	return nil
}

// unlockpt unlocks the slave PTY.
func unlockpt(f *os.File) error {
	var lock int32 = 0
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), TIOCSPTLCK, uintptr(unsafe.Pointer(&lock)))
	if errno != 0 {
		return fmt.Errorf("unlockpt failed: errno=%d", errno)
	}
	return nil
}

// ptsname returns the slave PTY name for the given master PTY.
func ptsname(f *os.File) (string, error) {
	var ptynum uint32
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), TIOCGPTN, uintptr(unsafe.Pointer(&ptynum)))
	if errno != 0 {
		return "", fmt.Errorf("TIOCGPTN failed: errno=%d", errno)
	}
	return fmt.Sprintf("/dev/pts/%d", ptynum), nil
}

// Create creates a PTY and optionally symlinks it to linkPath.
// If linkPath is empty, only the PTY is created.
// If linkPath is set (e.g., /dev/ttyV0), a symlink is created.
func Create(linkPath string) (*VirtualPort, error) {
	logger := slog.Default().With("component", "virtual-pty")

	// Open PTY master
	ptyFile, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to open PTY master: %w", err)
	}

	// Grantpt - unlock the slave PTY
	if err := grantpt(ptyFile); err != nil {
		ptyFile.Close()
		return nil, fmt.Errorf("failed to grantpt: %w", err)
	}

	// Unlockpt - unlock the slave PTY
	if err := unlockpt(ptyFile); err != nil {
		ptyFile.Close()
		return nil, fmt.Errorf("failed to unlockpt: %w", err)
	}

	// Get the slave PTY name
	slaveName, err := ptsname(ptyFile)
	if err != nil {
		ptyFile.Close()
		return nil, fmt.Errorf("failed to get PTY slave name: %w", err)
	}

	logger.Info("PTY created", "slave", slaveName)

	vp := &VirtualPort{
		ptyFile:  ptyFile,
		ptyName:  slaveName,
		linkPath: linkPath,
		logger:   logger,
	}

	// Create symlink if linkPath is specified
	if linkPath != "" {
		// Remove existing symlink/file if it exists
		os.Remove(linkPath)

		if err := os.Symlink(slaveName, linkPath); err != nil {
			ptyFile.Close()
			return nil, fmt.Errorf("failed to create symlink %s -> %s: %w", linkPath, slaveName, err)
		}
		logger.Info("Symlink created", "link", linkPath, "target", slaveName)
	}

	// Set permissions on the slave PTY so non-root users can access it
	if err := os.Chmod(slaveName, 0666); err != nil {
		logger.Warn("Failed to set slave PTY permissions (may need root)", "error", err)
	}

	logger.Info("Virtual serial port ready", "port", vp.PortName())
	return vp, nil
}

// Read reads data from the PTY master.
func (vp *VirtualPort) Read(buf []byte) (int, error) {
	return vp.ptyFile.Read(buf)
}

// Write writes data to the PTY master.
func (vp *VirtualPort) Write(data []byte) (int, error) {
	return vp.ptyFile.Write(data)
}

// Close closes the PTY and removes the symlink.
func (vp *VirtualPort) Close() error {
	var errs []string

	if vp.ptyFile != nil {
		if err := vp.ptyFile.Close(); err != nil {
			errs = append(errs, fmt.Sprintf("close PTY: %v", err))
		}
	}

	if vp.linkPath != "" {
		if err := os.Remove(vp.linkPath); err != nil && !os.IsNotExist(err) {
			errs = append(errs, fmt.Sprintf("remove symlink: %v", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors closing virtual port: %s", strings.Join(errs, "; "))
	}

	vp.logger.Info("Virtual serial port closed")
	return nil
}

// PortName returns the path that applications should use to connect.
func (vp *VirtualPort) PortName() string {
	if vp.linkPath != "" {
		return vp.linkPath
	}
	return vp.ptyName
}

// SetBaudRate is a no-op on PTY (baud rate is virtual).
func (vp *VirtualPort) SetBaudRate(baud int) error {
	return nil
}

// CheckPTAvailable checks if PTY is available on this system.
func CheckPTAvailable() error {
	f, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("PTY not available: %w (try: sudo chmod 666 /dev/ptmx)", err)
	}
	f.Close()
	return nil
}