package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type BotProcess struct {
	cmd  *exec.Cmd
	done chan error
}

func startBotProcess(botDir string) (*BotProcess, error) {
	lg := workerLog("worker.bot_runner")
	dir, err := resolveBotDir(botDir)
	if err != nil {
		lg.Printf("resolve bot dir error: %v", err)
		return nil, err
	}

	if err := stopExistingBotProcesses(dir); err != nil {
		lg.Printf("old bot cleanup warning: %v", err)
		fmt.Fprintf(os.Stderr, "warning: old bot process cleanup xato: %v\n", err)
	}

	cmd := exec.Command("go", "run", "./cmd/bot")
	cmd.Dir = dir
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	logWriter := io.Writer(io.Discard)
	var logCloser io.Closer
	if scaleWorkflowLogs != nil {
		if f, err := os.OpenFile(filepath.Join(scaleWorkflowLogs.Dir(), "bot-child.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); err == nil {
			logWriter = f
			logCloser = f
		}
	}
	cmd.Stdout = logWriter
	cmd.Stderr = logWriter

	if err := cmd.Start(); err != nil {
		if logCloser != nil {
			_ = logCloser.Close()
		}
		lg.Printf("bot start error: %v", err)
		return nil, fmt.Errorf("bot start xato: %w", err)
	}
	lg.Printf("bot started: pid=%d dir=%s", cmd.Process.Pid, dir)

	bp := &BotProcess{cmd: cmd, done: make(chan error, 1)}
	go func() {
		bp.done <- cmd.Wait()
		if logCloser != nil {
			_ = logCloser.Close()
		}
	}()

	select {
	case err := <-bp.done:
		if err == nil {
			lg.Printf("bot exited unexpectedly without error")
			return nil, errors.New("bot kutilmaganda tez yopildi")
		}
		lg.Printf("bot exited early with error: %v", err)
		return nil, fmt.Errorf("bot start xato: %w", err)
	case <-time.After(450 * time.Millisecond):
	}

	return bp, nil
}

func (bp *BotProcess) Stop(timeout time.Duration) error {
	lg := workerLog("worker.bot_runner")
	if bp == nil || bp.cmd == nil || bp.cmd.Process == nil {
		return nil
	}
	if timeout <= 0 {
		timeout = 3 * time.Second
	}

	pid := bp.cmd.Process.Pid
	lg.Printf("bot stop requested: pid=%d timeout=%s", pid, timeout)
	if pgid, err := syscall.Getpgid(pid); err == nil {
		_ = syscall.Kill(-pgid, syscall.SIGTERM)
	} else {
		_ = bp.cmd.Process.Signal(syscall.SIGTERM)
	}

	select {
	case <-time.After(timeout):
		if pgid, err := syscall.Getpgid(pid); err == nil {
			_ = syscall.Kill(-pgid, syscall.SIGKILL)
		} else {
			_ = bp.cmd.Process.Kill()
		}
		select {
		case <-bp.done:
		default:
		}
		return fmt.Errorf("bot force-killed (pid=%d)", pid)
	case err := <-bp.done:
		if err == nil {
			lg.Printf("bot stopped gracefully: pid=%d", pid)
			return nil
		}
		lg.Printf("bot stop completed with error (ignored): pid=%d err=%v", pid, err)
		return nil
	}
}

func resolveBotDir(botDir string) (string, error) {
	cands := make([]string, 0, 6)
	if strings.TrimSpace(botDir) != "" {
		cands = append(cands, strings.TrimSpace(botDir))
	}
	cands = append(cands, "../bot", "bot")

	if wd, err := os.Getwd(); err == nil {
		cands = append(cands,
			filepath.Join(wd, "../bot"),
			filepath.Join(wd, "bot"),
		)
	}

	for _, c := range cands {
		abs, err := filepath.Abs(c)
		if err != nil {
			continue
		}
		st, err := os.Stat(abs)
		if err != nil || !st.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(abs, "cmd", "bot", "main.go")); err == nil {
			return abs, nil
		}
	}

	return "", errors.New("bot papkasi topilmadi (cmd/bot/main.go yo'q)")
}

func stopExistingBotProcesses(botDir string) error {
	pids, err := findBotProcessPIDs(botDir)
	if err != nil {
		return err
	}
	if len(pids) == 0 {
		return nil
	}

	for _, pid := range pids {
		_ = syscall.Kill(pid, syscall.SIGTERM)
	}

	deadline := time.Now().Add(2 * time.Second)
	alive := append([]int(nil), pids...)
	for len(alive) > 0 && time.Now().Before(deadline) {
		next := alive[:0]
		for _, pid := range alive {
			if isProcessAlive(pid) {
				next = append(next, pid)
			}
		}
		alive = next
		if len(alive) > 0 {
			time.Sleep(120 * time.Millisecond)
		}
	}

	for _, pid := range alive {
		_ = syscall.Kill(pid, syscall.SIGKILL)
	}
	return nil
}

func findBotProcessPIDs(botDir string) ([]int, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, nil
	}

	wantDir := filepath.Clean(botDir)
	self := os.Getpid()
	pids := make([]int, 0, 4)

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(e.Name())
		if err != nil || pid <= 1 || pid == self {
			continue
		}

		cwd, err := os.Readlink(filepath.Join("/proc", e.Name(), "cwd"))
		if err != nil || filepath.Clean(cwd) != wantDir {
			continue
		}

		b, err := os.ReadFile(filepath.Join("/proc", e.Name(), "cmdline"))
		if err != nil {
			continue
		}
		cmdline := strings.TrimSpace(strings.ReplaceAll(string(b), "\x00", " "))
		if !isBotProcessCmdline(cmdline) {
			continue
		}

		pids = append(pids, pid)
	}

	sort.Ints(pids)
	return pids, nil
}

func isBotProcessCmdline(cmdline string) bool {
	c := strings.ToLower(strings.TrimSpace(cmdline))
	if c == "" {
		return false
	}
	if strings.Contains(c, "go run ./cmd/bot") {
		return true
	}
	if strings.Contains(c, "/.cache/go-build/") && strings.HasSuffix(c, "/bot") {
		return true
	}
	return false
}

func isProcessAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	if err == nil {
		return true
	}
	return errors.Is(err, syscall.EPERM)
}
