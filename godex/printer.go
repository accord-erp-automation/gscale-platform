package godex

import (
	"fmt"
	"time"
)

type Printer struct {
	transport Transport
}

func NewPrinter(transport Transport) *Printer {
	return &Printer{transport: transport}
}

func OpenG500() (*Printer, error) {
	transport, err := OpenG500USB()
	if err != nil {
		return nil, err
	}
	return NewPrinter(transport), nil
}

func (p *Printer) Close() error {
	if p == nil || p.transport == nil {
		return nil
	}
	return p.transport.Close()
}

func (p *Printer) Send(command string, read bool, pause time.Duration) (string, error) {
	if p == nil || p.transport == nil {
		return "", fmt.Errorf("printer transport is not open")
	}
	return p.transport.Send(command, read, pause)
}

func (p *Printer) Status() (string, error) {
	return p.Send("~S,STATUS", true, 120*time.Millisecond)
}

func (p *Printer) SetBuzzer(enabled bool) error {
	value := "0"
	if enabled {
		value = "1"
	}
	if _, err := p.Send("^XSET,BUZZER,"+value, false, 120*time.Millisecond); err != nil {
		return err
	}
	return nil
}

func (p *Printer) Recover() (string, error) {
	for _, command := range RecoverySequence {
		if _, err := p.Send(command, false, 300*time.Millisecond); err != nil {
			return "", err
		}
	}
	return p.Status()
}

func (p *Printer) Calibrate() (string, error) {
	attempts := [][]string{
		{"~S,ESG", "^AD", "^XSET,IMMEDIATE,1", "^XSET,ACTIVERESPONSE,1", "~S,CANCEL", "~S,SENSOR"},
		{"~S,ESG", "^AD", "^XSET,IMMEDIATE,1", "^XSET,ACTIVERESPONSE,1", "~S,CANCEL", "~S,SENSOR", "~V"},
		{"~S,ESG", "^AD", "^XSET,IMMEDIATE,1", "^XSET,ACTIVERESPONSE,1", "~Z", "~S,CANCEL", "~S,SENSOR"},
	}
	for _, commands := range attempts {
		for _, command := range commands {
			if _, err := p.Send(command, false, 350*time.Millisecond); err != nil {
				return "", err
			}
		}
		time.Sleep(1500 * time.Millisecond)
		for i := 0; i < 5; i++ {
			status, err := p.Status()
			if err != nil {
				return "", err
			}
			if status != "" && len(status) >= 3 && status[:3] == "00," {
				return status, nil
			}
			time.Sleep(400 * time.Millisecond)
		}
	}
	return p.Status()
}

func (p *Printer) DownloadGraphic(name string, graphic []byte) error {
	if _, err := p.Send(fmt.Sprintf("~MDELG,%s", name), false, 100*time.Millisecond); err != nil {
		// The graphic may not exist yet; keep the Python script's forgiving behavior.
	}
	if _, err := p.Send(fmt.Sprintf("~EB,%s,%d", name, len(graphic)), false, 50*time.Millisecond); err != nil {
		return err
	}
	if p == nil || p.transport == nil {
		return fmt.Errorf("printer transport is not open")
	}
	if err := p.transport.WriteRaw(graphic); err != nil {
		return err
	}
	time.Sleep(400 * time.Millisecond)
	return nil
}

func (p *Printer) PrintPack(input PackLabel, options LabelOptions) (string, error) {
	data, err := BuildPackLabel(input, options)
	if err != nil {
		return "", err
	}
	if err := p.SetBuzzer(false); err != nil {
		return "", fmt.Errorf("disable buzzer: %w", err)
	}
	if err := p.DownloadGraphic(TextGraphicName, data.TextGraphicBMP); err != nil {
		return "", fmt.Errorf("download text graphic: %w", err)
	}
	if err := p.DownloadGraphic(QRGraphicName, data.QRGraphicBMP); err != nil {
		return "", fmt.Errorf("download qr graphic: %w", err)
	}
	for idx, command := range data.Commands {
		if _, err := p.Send(command, false, 120*time.Millisecond); err != nil {
			return "", fmt.Errorf("send print command %d: %w", idx+1, err)
		}
	}
	time.Sleep(time.Second)
	status, err := p.Status()
	if err != nil {
		return "", fmt.Errorf("final status: %w", err)
	}
	return status, nil
}

func (p *Printer) PrintText(text string, options LabelOptions, center bool, contentXMM, contentYMM float64) (string, error) {
	for _, command := range BuildTextLabel(text, options, center, contentXMM, contentYMM) {
		if _, err := p.Send(command, false, 120*time.Millisecond); err != nil {
			return "", err
		}
	}
	time.Sleep(time.Second)
	return p.Status()
}

func (p *Printer) PrintQR(payload string, options LabelOptions, center bool, qrBoxMM, qrMul int, contentXMM, contentYMM float64) (string, error) {
	for _, command := range BuildQRLabel(payload, options, center, qrBoxMM, qrMul, contentXMM, contentYMM) {
		if _, err := p.Send(command, false, 120*time.Millisecond); err != nil {
			return "", err
		}
	}
	time.Sleep(time.Second)
	return p.Status()
}

func (p *Printer) PrintArchiveBatch(input ArchiveBatchLabel, options LabelOptions) (string, error) {
	for _, command := range BuildArchiveBatchLabel(input, options) {
		if _, err := p.Send(command, false, 120*time.Millisecond); err != nil {
			return "", err
		}
	}
	time.Sleep(time.Second)
	return p.Status()
}
