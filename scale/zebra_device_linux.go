package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
)

type ZebraPrinter struct {
	DevicePath   string
	VendorID     string
	ProductID    string
	Manufacturer string
	Product      string
	Serial       string
	BusNum       string
	DevNum       string
}

func (p ZebraPrinter) IsZebra() bool {
	if strings.EqualFold(strings.TrimSpace(p.VendorID), "0a5f") {
		return true
	}
	text := strings.ToLower(strings.TrimSpace(p.Manufacturer + " " + p.Product))
	return strings.Contains(text, "zebra") || strings.Contains(text, "ztc")
}

func (p ZebraPrinter) DisplayName() string {
	name := strings.TrimSpace(p.Manufacturer + " " + p.Product)
	if name == "" {
		return "unknown"
	}
	return name
}

func FindZebraPrinters() ([]ZebraPrinter, error) {
	devices, err := filepath.Glob("/dev/usb/lp*")
	if err != nil {
		return nil, err
	}

	printers := make([]ZebraPrinter, 0, len(devices))
	for _, dev := range devices {
		p := ZebraPrinter{DevicePath: dev}
		fillZebraSysfs(&p)
		printers = append(printers, p)
	}

	sort.Slice(printers, func(i, j int) bool {
		return printers[i].DevicePath < printers[j].DevicePath
	})
	return printers, nil
}

func SelectZebraPrinter(preferred string) (ZebraPrinter, error) {
	printers, err := FindZebraPrinters()
	if err != nil {
		return ZebraPrinter{}, err
	}
	if len(printers) == 0 {
		return ZebraPrinter{}, errors.New("zebra: USB printer topilmadi")
	}

	if strings.TrimSpace(preferred) != "" {
		want := strings.TrimSpace(preferred)
		for _, p := range printers {
			if p.DevicePath == want {
				return p, nil
			}
		}
		return ZebraPrinter{}, fmt.Errorf("zebra: ko'rsatilgan device topilmadi: %s", want)
	}

	for _, p := range printers {
		if p.IsZebra() {
			return p, nil
		}
	}
	return printers[0], nil
}

func fillZebraSysfs(p *ZebraPrinter) {
	base := filepath.Base(p.DevicePath)
	classPath := filepath.Join("/sys/class/usbmisc", base)
	ifacePath, err := filepath.EvalSymlinks(filepath.Join(classPath, "device"))
	if err != nil {
		return
	}

	parent := filepath.Dir(ifacePath)
	p.VendorID = readTrimFile(filepath.Join(parent, "idVendor"))
	p.ProductID = readTrimFile(filepath.Join(parent, "idProduct"))
	p.Manufacturer = readTrimFile(filepath.Join(parent, "manufacturer"))
	p.Product = readTrimFile(filepath.Join(parent, "product"))
	p.Serial = readTrimFile(filepath.Join(parent, "serial"))
	p.BusNum = readTrimFile(filepath.Join(parent, "busnum"))
	p.DevNum = readTrimFile(filepath.Join(parent, "devnum"))
}

func readTrimFile(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func zebraSendRaw(device string, payload []byte) error {
	return withZebraGlobalLock(5*time.Second, func() error {
		return zebraSendRawUnlocked(device, payload)
	})
}

func zebraSendRawUnlocked(device string, payload []byte) error {
	if strings.TrimSpace(device) == "" {
		return errors.New("zebra: device bo'sh")
	}
	if len(payload) == 0 {
		return errors.New("zebra: payload bo'sh")
	}

	fd, err := syscall.Open(device, syscall.O_WRONLY|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
	if err != nil {
		return fmt.Errorf("zebra: device ochilmadi: %w", err)
	}
	defer syscall.Close(fd)

	if err := writeFDNonBlocking(fd, payload, time.Now().Add(4*time.Second)); err != nil {
		return fmt.Errorf("zebra: yozib bo'lmadi: %w", err)
	}
	return nil
}

func zebraSendSGD(device string, command string) error {
	command = strings.TrimSpace(command)
	if command == "" {
		return errors.New("zebra: command bo'sh")
	}
	if !strings.HasSuffix(command, "\r\n") {
		command += "\r\n"
	}
	return zebraSendRaw(device, []byte(command))
}

func queryZebraHostStatus(device string, timeout time.Duration) (string, error) {
	resp, err := zebraTransceiveRaw(device, []byte("~HS\n"), timeout)
	if err != nil {
		return "", err
	}
	return normalizeZebraResponse(resp), nil
}

func queryZebraSGDVar(device, key string, timeout time.Duration) (string, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return "", errors.New("zebra: key bo'sh")
	}
	cmd := fmt.Sprintf("! U1 getvar \"%s\"\r\n", key)
	resp, err := zebraTransceiveRaw(device, []byte(cmd), timeout)
	if err != nil {
		return "", err
	}
	text := normalizeZebraResponse(resp)
	text = strings.TrimSpace(strings.Trim(text, "\""))
	return text, nil
}

func zebraTransceiveRaw(device string, payload []byte, timeout time.Duration) ([]byte, error) {
	if timeout <= 0 {
		timeout = 1200 * time.Millisecond
	}

	lockTimeout := timeout + 2*time.Second
	var out []byte
	err := withZebraGlobalLock(lockTimeout, func() error {
		resp, err := zebraTransceiveRawUnlocked(device, payload, timeout)
		if err != nil {
			return err
		}
		out = resp
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func zebraTransceiveRawUnlocked(device string, payload []byte, timeout time.Duration) ([]byte, error) {
	if timeout <= 0 {
		timeout = 1200 * time.Millisecond
	}

	fd, err := syscall.Open(device, syscall.O_RDWR|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
	if err != nil {
		if err := zebraSendRawUnlocked(device, payload); err != nil {
			return nil, err
		}
		return nil, errors.New("zebra: R/W open bo'lmadi; query yuborildi")
	}
	defer syscall.Close(fd)

	if err := writeFDNonBlocking(fd, payload, time.Now().Add(timeout)); err != nil {
		return nil, fmt.Errorf("zebra: payload yuborilmadi: %w", err)
	}

	deadline := time.Now().Add(timeout)
	buf := make([]byte, 4096)
	resp := make([]byte, 0, 4096)

	for time.Now().Before(deadline) {
		n, rerr := syscall.Read(fd, buf)
		if n > 0 {
			resp = append(resp, buf[:n]...)
			if n < len(buf) {
				break
			}
		}

		if rerr != nil {
			errNo, ok := rerr.(syscall.Errno)
			if ok && (errNo == syscall.EAGAIN || errNo == syscall.EWOULDBLOCK) {
				time.Sleep(35 * time.Millisecond)
				continue
			}
			if len(resp) > 0 {
				break
			}
			return nil, rerr
		}

		if n == 0 {
			time.Sleep(35 * time.Millisecond)
		}
	}

	if len(resp) == 0 {
		return nil, errors.New("zebra: javob olinmadi")
	}
	return resp, nil
}

func writeFDNonBlocking(fd int, payload []byte, deadline time.Time) error {
	off := 0
	for off < len(payload) {
		n, err := syscall.Write(fd, payload[off:])
		if n > 0 {
			off += n
		}
		if err != nil {
			errNo, ok := err.(syscall.Errno)
			if ok && (errNo == syscall.EAGAIN || errNo == syscall.EWOULDBLOCK) {
				if time.Now().After(deadline) {
					return fmt.Errorf("timeout: %w", err)
				}
				time.Sleep(20 * time.Millisecond)
				continue
			}
			return err
		}
		if n == 0 {
			if time.Now().After(deadline) {
				return errors.New("timeout")
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	return nil
}

func normalizeZebraResponse(raw []byte) string {
	text := string(raw)
	text = strings.ReplaceAll(text, "\x00", "")
	text = strings.ReplaceAll(text, "\r", "\n")
	rows := strings.Split(text, "\n")
	clean := make([]string, 0, len(rows))
	for _, row := range rows {
		row = strings.TrimSpace(row)
		if row == "" {
			continue
		}
		clean = append(clean, row)
	}
	return strings.Join(clean, "\n")
}
