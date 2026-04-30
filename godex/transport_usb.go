package godex

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/gousb"
)

type USBTransport struct {
	ctx  *gousb.Context
	dev  *gousb.Device
	cfg  *gousb.Config
	intf *gousb.Interface
	out  *gousb.OutEndpoint
	in   *gousb.InEndpoint
}

func OpenG500USB() (*USBTransport, error) {
	ctx := gousb.NewContext()
	dev, err := ctx.OpenDeviceWithVIDPID(gousb.ID(VendorID), gousb.ID(ProductID))
	if err != nil {
		_ = ctx.Close()
		return nil, fmt.Errorf("open GoDEX G500 usb: %w", err)
	}
	if dev == nil {
		_ = ctx.Close()
		return nil, fmt.Errorf("GoDEX G500 not found")
	}
	_ = dev.SetAutoDetach(true)

	cfg, err := dev.Config(1)
	if err != nil {
		_ = dev.Close()
		_ = ctx.Close()
		return nil, fmt.Errorf("set usb configuration: %w", err)
	}
	intf, err := cfg.Interface(0, 0)
	if err != nil {
		_ = cfg.Close()
		_ = dev.Close()
		_ = ctx.Close()
		return nil, fmt.Errorf("claim usb interface: %w", err)
	}
	out, err := intf.OutEndpoint(1)
	if err != nil {
		intf.Close()
		_ = cfg.Close()
		_ = dev.Close()
		_ = ctx.Close()
		return nil, fmt.Errorf("open usb out endpoint 0x01: %w", err)
	}
	in, err := intf.InEndpoint(2)
	if err != nil {
		intf.Close()
		_ = cfg.Close()
		_ = dev.Close()
		_ = ctx.Close()
		return nil, fmt.Errorf("open usb in endpoint 0x82: %w", err)
	}

	return &USBTransport{
		ctx:  ctx,
		dev:  dev,
		cfg:  cfg,
		intf: intf,
		out:  out,
		in:   in,
	}, nil
}

func (t *USBTransport) Send(command string, read bool, pause time.Duration) (string, error) {
	if t == nil || t.out == nil {
		return "", fmt.Errorf("usb transport is not open")
	}
	if pause <= 0 {
		pause = 120 * time.Millisecond
	}
	if err := t.WriteRaw(EncodeCommand(command)); err != nil {
		return "", err
	}
	time.Sleep(pause)
	if !read {
		return "", nil
	}
	if t.in == nil {
		return "", nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 1200*time.Millisecond)
	defer cancel()
	buf := make([]byte, 512)
	n, err := t.in.ReadContext(ctx, buf)
	if err != nil {
		return "", nil
	}
	return strings.TrimSpace(string(buf[:n])), nil
}

func (t *USBTransport) WriteRaw(payload []byte) error {
	if t == nil || t.out == nil {
		return fmt.Errorf("usb transport is not open")
	}
	for offset := 0; offset < len(payload); offset += 4096 {
		end := offset + 4096
		if end > len(payload) {
			end = len(payload)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_, err := t.out.WriteContext(ctx, payload[offset:end])
		cancel()
		if err != nil {
			return fmt.Errorf("usb write: %w", err)
		}
	}
	return nil
}

func (t *USBTransport) Close() error {
	if t == nil {
		return nil
	}
	if t.intf != nil {
		t.intf.Close()
		t.intf = nil
	}
	var err error
	if t.cfg != nil {
		err = t.cfg.Close()
		t.cfg = nil
	}
	if t.dev != nil {
		if closeErr := t.dev.Close(); err == nil {
			err = closeErr
		}
		t.dev = nil
	}
	if t.ctx != nil {
		if closeErr := t.ctx.Close(); err == nil {
			err = closeErr
		}
		t.ctx = nil
	}
	return err
}
