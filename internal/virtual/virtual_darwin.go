//go:build darwin

package virtual

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"unsafe"
)

// VirtualPort manages a PTY-based virtual serial port on macOS.
type VirtualPort struct {
	ptyFile  *os.File
	ptyName  string
	linkPath string
	logger   *slog.Logger
}

// macOS ioctl constants
const (
	_IOC_OUT    = 0x40000000
	_IOC_IN     = 0x80000000
	_IOC_INOUT  = 0xC0000000
	_IOCPARM_MASK = 0x1FFF
	_IOSIZE_SHIFT = 16
)

func _IOC(inout, group, num, len uintptr) uintptr {
	return inout | ((len & uintptr(_IOCPARM_MASK)) << _IOSIZE_SHIFT) | ((group & 0xFF) << 8) | (num & 0xFF)
}

var (
	// TIOCGPTN = _IOR('T', 0x30, unsigned int) on Linux
	// On macOS, we use TIOCPTYGNAME which is different
	// We'll use exec-based approach instead
)

// ptsnameMacOS gets the slave PTY name using ptsname command or ioctl.
func ptsnameMacOS(f *os.File) (string, error) {
	// Try using ptsname_r equivalent via exec
	// On macOS, we can use the TIOCPTYGNAME ioctl
	var buf [128]byte
	_, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL,
		f.Fd(),
		uintptr(_IOC(_IOC_OUT, 'T', 0x40, 128)), // TIOCPTYGNAME
		uintptr(unsafe.Pointer(&buf[0])),
	)
	if errno != 0 {
		// Fallback: try /dev/fd/ approach
		link := fmt.Sprintf("/dev/fd/%d", f.Fd())
		target, err := os.Readlink(link)
		if err != nil {
			return "", fmt.Errorf("ptsname failed: errno=%d, fallback failed: %w", errno, err)
		}
		return target, nil
	}

	// Find null terminator
	name := string(buf[:])
	if idx := strings.IndexByte(name, 0); idx >= 0 {
		name = name[:idx]
	}
	return name, nil
}

func grantptMacOS(slaveName string) error {
	cmd := exec.Command("grantpt", slaveName)
	return cmd.Run()
}

func unlockptMacOS(f *os.File) error {
	// TIOCPTYUNLK on macOS
	_, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL,
		f.Fd(),
		uintptr(_IOC(_IOC_OUT, 'T', 0x41, 0)), // TIOCPTYUNLK
		0,
	)
	if errno != 0 {
		return fmt.Errorf("unlockpt failed: errno=%d", errno)
	}
	return nil
}

// Create creates a PTY and optionally symlinks it to linkPath on macOS.
func Create(linkPath string) (*VirtualPort, error) {
	logger := slog.Default().With("component", "virtual-pty")

	// Open PTY master
	ptyFile, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to open PTY master: %w", err)
	}

	// Get slave name
	slaveName, err := ptsnameMacOS(ptyFile)
	if err != nil {
		ptyFile.Close()
		return nil, fmt.Errorf("failed to get PTY slave name: %w", err)
	}

	// Grantpt and unlockpt
	if err := grantptMacOS(slaveName); err != nil {
		logger.Warn("grantpt failed (may still work)", "error", err)
	}
	if err := unlockptMacOS(ptyFile); err != nil {
		logger.Warn("unlockpt failed (may still work)", "error", err)
	}

	logger.Info("PTY created (macOS)", "slave", slaveName)

	vp := &VirtualPort{
		ptyFile:  ptyFile,
		ptyName:  slaveName,
		linkPath: linkPath,
		logger:   logger,
	}

	// Create symlink if linkPath is specified
	if linkPath != "" {
		os.Remove(linkPath)
		if err := os.Symlink(slaveName, linkPath); err != nil {
			ptyFile.Close()
			return nil, fmt.Errorf("failed to create symlink %s -> %s: %w", linkPath, slaveName, err)
		}
		logger.Info("Symlink created", "link", linkPath, "target", slaveName)
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

// PortName returns the path that applications should use.
func (vp *VirtualPort) PortName() string {
	if vp.linkPath != "" {
		return vp.linkPath
	}
	return vp.ptyName
}

// SetBaudRate is a no-op on PTY.
func (vp *VirtualPort) SetBaudRate(baud int) error {
	return nil
}

// CheckPTAvailable checks if PTY is available on macOS.
func CheckPTAvailable() error {
	f, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("PTY not available on macOS: %w", err)
	}
	f.Close()
	return nil
}