package app

import (
	bridgestate "bridge/state"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const (
	defaultCalibrateDevice = "/dev/usb/lp0"
	zebraGlobalLockPath    = "/tmp/gscale-zebra/zebra.lock"
)

type calibrateOptions struct {
	device string
	save   bool
	dryRun bool
}

func (a *App) handleCalibrateCommand(ctx context.Context, chatID int64, text string) error {
	opts, err := parseCalibrateOptions(text)
	if err != nil {
		return a.tg.SendMessage(ctx, chatID, "Format: /calibrate [--device /dev/usb/lp0] [--no-save] [--dry-run]")
	}

	if a.hasAnyBatchSession() {
		return a.tg.SendMessage(ctx, chatID, "Batch ishlab turganda calibration mumkin emas. Avval Batch Stop qiling.")
	}

	device := a.resolveCalibrateDevice(opts.device)
	cmds := buildCalibrationCommands(opts.save)
	if opts.dryRun {
		lines := []string{
			fmt.Sprintf("Calibration DRY-RUN: device=%s save=%v", device, opts.save),
			"Yuboriladigan commandlar:",
		}
		for i, cmd := range cmds {
			lines = append(lines, fmt.Sprintf("%d) %q", i+1, cmd))
		}
		return a.tg.SendMessage(ctx, chatID, strings.Join(lines, "\n"))
	}

	start := time.Now()
	if err := a.tg.SendMessage(ctx, chatID, fmt.Sprintf("Calibration boshlandi: device=%s save=%v", device, opts.save)); err != nil {
		return err
	}

	if err := runZebraCalibration(ctx, device, opts.save); err != nil {
		a.logRun.Printf("calibration error: device=%s save=%v err=%v", device, opts.save, err)
		return a.tg.SendMessage(ctx, chatID, "Calibration xato: "+friendlyCalibrateError(err, device))
	}

	dur := time.Since(start).Round(10 * time.Millisecond)
	return a.tg.SendMessage(ctx, chatID, fmt.Sprintf("Calibration tugadi: device=%s time=%s", device, dur))
}

func (a *App) resolveCalibrateDevice(preferred string) string {
	if v := strings.TrimSpace(preferred); v != "" {
		return v
	}

	bridgePath := strings.TrimSpace(a.cfg.BridgeStateFile)
	if bridgePath != "" {
		snap, err := bridgestate.New(bridgePath).Read()
		if err == nil {
			if d := strings.TrimSpace(snap.Zebra.DevicePath); d != "" {
				return d
			}
		}
	}

	return defaultCalibrateDevice
}

func (a *App) hasAnyBatchSession() bool {
	if a == nil || a.control == nil {
		return false
	}
	return a.control.ActiveBatch().Active
}

func parseCalibrateOptions(text string) (calibrateOptions, error) {
	opts := calibrateOptions{save: true}
	fields := strings.Fields(strings.TrimSpace(text))
	if len(fields) == 0 {
		return opts, nil
	}

	for i := 1; i < len(fields); i++ {
		arg := strings.TrimSpace(fields[i])
		if arg == "" {
			continue
		}
		lower := strings.ToLower(arg)

		switch {
		case lower == "--no-save" || lower == "nosave" || lower == "no-save":
			opts.save = false
		case lower == "--save":
			opts.save = true
		case lower == "--dry-run" || lower == "dry-run":
			opts.dryRun = true
		case strings.HasPrefix(lower, "--device="):
			opts.device = strings.TrimSpace(arg[len("--device="):])
		case lower == "--device":
			if i+1 >= len(fields) {
				return opts, errors.New("device qiymati yo'q")
			}
			i++
			opts.device = strings.TrimSpace(fields[i])
		case strings.HasPrefix(arg, "/dev/"):
			opts.device = arg
		default:
			return opts, fmt.Errorf("noma'lum parametr: %s", arg)
		}
	}

	return opts, nil
}

func buildCalibrationCommands(save bool) []string {
	cmds := []string{"~JC\n"}
	if save {
		cmds = append(cmds, "^XA^JUS^XZ\n")
	}
	return cmds
}

func runZebraCalibration(ctx context.Context, device string, save bool) error {
	device = strings.TrimSpace(device)
	if device == "" {
		return errors.New("zebra device bo'sh")
	}

	return withZebraGlobalLock(8*time.Second, func() error {
		f, err := os.OpenFile(device, os.O_WRONLY, 0)
		if err != nil {
			return fmt.Errorf("zebra device ochilmadi: %w", err)
		}
		defer f.Close()

		cmds := buildCalibrationCommands(save)
		for i, cmd := range cmds {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			if _, err := f.WriteString(cmd); err != nil {
				return fmt.Errorf("calibration command #%d xato: %w", i+1, err)
			}
			time.Sleep(350 * time.Millisecond)
		}
		return nil
	})
}

func friendlyCalibrateError(err error, device string) string {
	if err == nil {
		return "noma'lum xato"
	}

	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return "calibration bekor qilindi"
	}
	if errors.Is(err, os.ErrNotExist) {
		if strings.TrimSpace(device) != "" {
			return fmt.Sprintf("Zebra qurilmasi ulanmagan yoki topilmadi: %s", strings.TrimSpace(device))
		}
		return "Zebra qurilmasi ulanmagan yoki topilmadi"
	}

	var errno syscall.Errno
	if errors.As(err, &errno) {
		switch errno {
		case syscall.EBUSY:
			return "Zebra qurilma band, boshqa dastur ishlatyapti"
		case syscall.EACCES, syscall.EPERM:
			return "Zebra qurilmaga ruxsat yo'q (permission denied)"
		}
	}

	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	switch {
	case strings.Contains(msg, "no such file or directory"):
		return "Zebra qurilmasi ulanmagan yoki topilmadi"
	case strings.Contains(msg, "device or resource busy"), strings.Contains(msg, "resource busy"):
		return "Zebra qurilma band, boshqa dastur ishlatyapti"
	case strings.Contains(msg, "permission denied"):
		return "Zebra qurilmaga ruxsat yo'q (permission denied)"
	default:
		return strings.TrimSpace(err.Error())
	}
}

func withZebraGlobalLock(timeout time.Duration, fn func() error) error {
	if timeout <= 0 {
		timeout = 2 * time.Second
	}

	if err := os.MkdirAll(filepath.Dir(zebraGlobalLockPath), 0o755); err != nil {
		return fmt.Errorf("zebra lock dir ochilmadi: %w", err)
	}

	f, err := os.OpenFile(zebraGlobalLockPath, os.O_CREATE|os.O_RDWR, 0o666)
	if err != nil {
		return fmt.Errorf("zebra lock file ochilmadi: %w", err)
	}
	defer f.Close()

	deadline := time.Now().Add(timeout)
	for {
		err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			break
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EAGAIN) {
			return fmt.Errorf("zebra lock xato: %w", err)
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("zebra lock timeout")
		}
		time.Sleep(25 * time.Millisecond)
	}
	defer func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	}()

	return fn()
}
