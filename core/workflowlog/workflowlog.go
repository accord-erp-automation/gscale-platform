package workflowlog

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

var workerNameSanitizer = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

type Manager struct {
	process string
	dir     string

	mu      sync.Mutex
	files   map[string]*os.File
	loggers map[string]*log.Logger
}

func New(process string) (*Manager, error) {
	process = strings.TrimSpace(process)
	if process == "" {
		process = "app"
	}

	wd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("workflowlog: getwd: %w", err)
	}
	root := findWorkspaceRoot(wd)
	dir := filepath.Join(root, "logs", process)

	// Every restart gets a clean log set for this process.
	if err := os.RemoveAll(dir); err != nil {
		return nil, fmt.Errorf("workflowlog: clear dir: %w", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("workflowlog: mkdir: %w", err)
	}

	return &Manager{
		process: process,
		dir:     dir,
		files:   make(map[string]*os.File),
		loggers: make(map[string]*log.Logger),
	}, nil
}

func (m *Manager) Dir() string {
	if m == nil {
		return ""
	}
	return m.dir
}

func (m *Manager) Logger(worker string) *log.Logger {
	if m == nil {
		return log.New(io.Discard, "[workflow] ", log.LstdFlags|log.Lmicroseconds|log.LUTC)
	}

	name := sanitizeWorkerName(worker)
	m.mu.Lock()
	defer m.mu.Unlock()

	if l, ok := m.loggers[name]; ok {
		return l
	}

	p := filepath.Join(m.dir, name+".log")
	f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		// Do not crash main flow on logging setup issues.
		return log.New(io.Discard, "["+name+"] ", log.LstdFlags|log.Lmicroseconds|log.LUTC)
	}

	m.files[name] = f
	l := log.New(f, "["+name+"] ", log.LstdFlags|log.Lmicroseconds|log.LUTC)
	m.loggers[name] = l
	return l
}

func (m *Manager) Close() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for k, f := range m.files {
		_ = f.Close()
		delete(m.files, k)
	}
}

func sanitizeWorkerName(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "worker"
	}
	v = workerNameSanitizer.ReplaceAllString(v, "_")
	v = strings.Trim(v, "._-")
	if v == "" {
		return "worker"
	}
	return strings.ToLower(v)
}

func findWorkspaceRoot(start string) string {
	cur := start
	for {
		if _, err := os.Stat(filepath.Join(cur, "go.work")); err == nil {
			return cur
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			// Fallback: keep logs near current process dir.
			return start
		}
		cur = parent
	}
}
