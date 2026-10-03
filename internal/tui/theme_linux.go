//go:build linux

package tui

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unsafe"

	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/sys/unix"
)

const themeWatchMask = unix.IN_ATTRIB |
	unix.IN_CLOSE_WRITE |
	unix.IN_CREATE |
	unix.IN_DELETE |
	unix.IN_DELETE_SELF |
	unix.IN_MODIFY |
	unix.IN_MOVE_SELF |
	unix.IN_MOVED_FROM |
	unix.IN_MOVED_TO

type themeMonitor struct {
	stateDir     string
	themeDir     string
	colorsPath   string
	themeName    string
	done         chan struct{}
	closeOnce    sync.Once
	wakeMu       sync.Mutex
	fd           int
	wakeFD       int
	watches      map[int]string
	lastHash     [sha256.Size]byte
	usingDefault bool
}

func newThemeMonitor() (*themeMonitor, themeStyles) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, defaultInteractiveTheme()
	}
	return newThemeMonitorAt(filepath.Join(home, ".local", "state", "omarchy", "current"))
}

func newThemeMonitorAt(stateDir string) (*themeMonitor, themeStyles) {
	monitor := &themeMonitor{
		stateDir:   stateDir,
		themeDir:   filepath.Join(stateDir, "theme"),
		colorsPath: filepath.Join(stateDir, "theme", "colors.toml"),
		themeName:  filepath.Join(stateDir, "theme.name"),
		done:       make(chan struct{}),
		fd:         -1,
		wakeFD:     -1,
		watches:    make(map[int]string),
	}
	theme, status, hash := readThemeStyles(monitor.colorsPath)
	if status == themeValid {
		monitor.lastHash = hash
		return monitor, theme
	}
	monitor.usingDefault = true
	return monitor, defaultInteractiveTheme()
}

func (m *themeMonitor) wait() tea.Cmd {
	if m == nil {
		return nil
	}
	return func() tea.Msg { return m.next() }
}

func (m *themeMonitor) close() {
	if m == nil {
		return
	}
	m.closeOnce.Do(func() {
		close(m.done)
		var event [8]byte
		binary.NativeEndian.PutUint64(event[:], 1)
		m.wakeMu.Lock()
		if m.wakeFD >= 0 {
			_, _ = unix.Write(m.wakeFD, event[:])
		}
		m.wakeMu.Unlock()
	})
}

func (m *themeMonitor) next() tea.Msg {
	if err := m.start(); err != nil {
		return nil
	}
	var buffer [4096]byte
	for {
		select {
		case <-m.done:
			m.shutdown()
			return nil
		default:
		}

		pollFDs := [2]unix.PollFd{
			{Fd: int32(m.fd), Events: unix.POLLIN},
			{Fd: int32(m.wakeFD), Events: unix.POLLIN},
		}
		_, err := unix.Poll(pollFDs[:], -1)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			m.shutdown()
			return nil
		}
		if pollFDs[1].Revents != 0 {
			m.shutdown()
			return nil
		}
		if pollFDs[0].Revents&(unix.POLLERR|unix.POLLHUP|unix.POLLNVAL) != 0 {
			m.shutdown()
			return nil
		}
		if pollFDs[0].Revents&unix.POLLIN == 0 {
			continue
		}

		count, err := unix.Read(m.fd, buffer[:])
		if errors.Is(err, unix.EAGAIN) || errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			m.shutdown()
			return nil
		}
		if !m.hasRelevantEvent(buffer[:count]) {
			continue
		}
		m.resetWatches()

		theme, status, hash := readThemeStyles(m.colorsPath)
		switch status {
		case themeValid:
			if !m.usingDefault && hash == m.lastHash {
				continue
			}
			m.usingDefault = false
			m.lastHash = hash
			return themeStylesMsg{theme: theme}
		case themeMissing:
			if m.usingDefault {
				continue
			}
			m.usingDefault = true
			m.lastHash = [sha256.Size]byte{}
			return themeStylesMsg{theme: defaultInteractiveTheme()}
		case themeInvalid:
			continue
		}
	}
}

func (m *themeMonitor) start() error {
	if m.fd >= 0 {
		return nil
	}
	fd, err := unix.InotifyInit1(unix.IN_CLOEXEC | unix.IN_NONBLOCK)
	if err != nil {
		return err
	}
	wakeFD, err := unix.Eventfd(0, unix.EFD_CLOEXEC|unix.EFD_NONBLOCK)
	if err != nil {
		_ = unix.Close(fd)
		return err
	}
	m.wakeMu.Lock()
	select {
	case <-m.done:
		m.wakeMu.Unlock()
		_ = unix.Close(fd)
		_ = unix.Close(wakeFD)
		return os.ErrClosed
	default:
		m.fd = fd
		m.wakeFD = wakeFD
	}
	m.wakeMu.Unlock()
	m.resetWatches()
	return nil
}

func (m *themeMonitor) resetWatches() {
	for descriptor := range m.watches {
		_, _ = unix.InotifyRmWatch(m.fd, uint32(descriptor))
	}
	clear(m.watches)

	directories := make(map[string]struct{}, 2)
	for _, target := range []string{m.stateDir, m.themeDir} {
		if directory := nearestExistingDirectory(target); directory != "" {
			directories[directory] = struct{}{}
		}
	}
	for directory := range directories {
		descriptor, err := unix.InotifyAddWatch(m.fd, directory, themeWatchMask|unix.IN_ONLYDIR)
		if err == nil {
			m.watches[descriptor] = directory
		}
	}
}

func (m *themeMonitor) hasRelevantEvent(data []byte) bool {
	relevant := false
	for offset := 0; offset+unix.SizeofInotifyEvent <= len(data); {
		event := (*unix.InotifyEvent)(unsafe.Pointer(&data[offset]))
		size := unix.SizeofInotifyEvent + int(event.Len)
		if offset+size > len(data) {
			break
		}
		directory, watched := m.watches[int(event.Wd)]
		if watched {
			name := ""
			if event.Len > 0 {
				name = unix.ByteSliceToString(data[offset+unix.SizeofInotifyEvent : offset+size])
			}
			path := directory
			if name != "" {
				path = filepath.Join(directory, name)
			}
			if m.relevantPath(path) {
				relevant = true
			}
		}
		offset += size
	}
	return relevant
}

func (m *themeMonitor) relevantPath(path string) bool {
	for _, target := range []string{m.stateDir, m.themeDir, m.colorsPath, m.themeName} {
		if path == target || pathContains(target, path) {
			return true
		}
	}
	return false
}

func (m *themeMonitor) shutdown() {
	if m.fd >= 0 {
		_ = unix.Close(m.fd)
		m.fd = -1
	}
	m.wakeMu.Lock()
	if m.wakeFD >= 0 {
		_ = unix.Close(m.wakeFD)
		m.wakeFD = -1
	}
	m.wakeMu.Unlock()
	clear(m.watches)
}

func nearestExistingDirectory(path string) string {
	for {
		info, err := os.Stat(path)
		if err == nil && info.IsDir() {
			return path
		}
		parent := filepath.Dir(path)
		if parent == path {
			return ""
		}
		path = parent
	}
}

func pathContains(path, parent string) bool {
	relative, err := filepath.Rel(parent, path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
