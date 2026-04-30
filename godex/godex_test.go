package godex

import (
	"bytes"
	"image"
	"image/color"
	"strings"
	"testing"
	"time"
)

func TestMMDotsMatchesPythonRound(t *testing.T) {
	if got := MMDots(50, 203); got != 400 {
		t.Fatalf("MMDots(50,203) = %d, want 400", got)
	}
	if got := MMDots(18, 203); got != 144 {
		t.Fatalf("MMDots(18,203) = %d, want 144", got)
	}
}

func TestEncodeScanPayloadUsesURLPathShape(t *testing.T) {
	got := EncodeScanPayload("ACCORD", "ZARQAND PRYANIKI", "89", "5", "30A5")
	want := "https://scan.wspace.sbs/L/ACCORD/ZARQAND+PRYANIKI/89/5/30A5"
	if got != want {
		t.Fatalf("payload = %q, want %q", got, want)
	}
}

func TestEncodeMonoBMPWritesOneBitBMP(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 9, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 9; x++ {
			img.Set(x, y, color.White)
		}
	}
	img.Set(0, 0, color.Black)

	b, err := EncodeMonoBMP(img)
	if err != nil {
		t.Fatalf("EncodeMonoBMP: %v", err)
	}
	if !bytes.HasPrefix(b, []byte("BM")) {
		t.Fatalf("missing BMP signature")
	}
	if got := int(b[28]) | int(b[29])<<8; got != 1 {
		t.Fatalf("bits per pixel = %d, want 1", got)
	}
}

func TestBuildPackLabelMatchesExpectedEZPLShape(t *testing.T) {
	data, err := BuildPackLabel(PackLabel{
		CompanyName: "Accord",
		ProductName: "Zo'r pista 100gr kok",
		KGText:      "89 kg",
		BruttoText:  "5",
		EPC:         "30a5fea7709854d93c2b7593",
	}, DefaultPackLabelOptions())
	if err != nil {
		t.Fatalf("BuildPackLabel: %v", err)
	}
	joined := strings.Join(data.Commands, "\n")
	for _, want := range []string{
		"~S,ESG",
		"^AD",
		"^XSET,UNICODE,1",
		"^Q50,3",
		"^W50",
		"Y0,0,TEXTLBL",
		"BA,",
		"30A5FEA7709854D93C2B7593",
		"QRLBL",
		"E",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("commands missing %q:\n%s", want, joined)
		}
	}
	if !bytes.HasPrefix(data.TextGraphicBMP, []byte("BM")) {
		t.Fatalf("text graphic is not BMP")
	}
	if !bytes.HasPrefix(data.QRGraphicBMP, []byte("BM")) {
		t.Fatalf("qr graphic is not BMP")
	}
	if !strings.HasPrefix(data.QRPayload, DefaultQRBaseURL) {
		t.Fatalf("qr payload = %q", data.QRPayload)
	}
}

func TestPrinterPrintPackSendsGraphicsBeforeCommands(t *testing.T) {
	ft := &fakeTransport{}
	p := NewPrinter(ft)
	_, err := p.PrintPack(PackLabel{
		CompanyName: "Accord",
		ProductName: "Test Product",
		KGText:      "1",
		BruttoText:  "5",
		EPC:         "30A5FEA7709854D93C2B7593",
	}, DefaultPackLabelOptions())
	if err != nil {
		t.Fatalf("PrintPack: %v", err)
	}
	joined := strings.Join(ft.commands, "\n")
	if !strings.Contains(joined, "~EB,TEXTLBL,") || !strings.Contains(joined, "~EB,QRLBL,") {
		t.Fatalf("graphic downloads missing:\n%s", joined)
	}
	if buzzer := strings.Index(joined, "^XSET,BUZZER,0"); buzzer < 0 || buzzer > strings.Index(joined, "~EB,TEXTLBL,") {
		t.Fatalf("buzzer was not disabled before graphic download:\n%s", joined)
	}
	if firstY := strings.Index(joined, "Y0,0,TEXTLBL"); firstY < strings.Index(joined, "~EB,QRLBL,") {
		t.Fatalf("label command sent before graphic download:\n%s", joined)
	}
	if len(ft.rawWrites) != 2 {
		t.Fatalf("raw writes = %d, want 2", len(ft.rawWrites))
	}
}

type fakeTransport struct {
	commands  []string
	rawWrites [][]byte
}

func (f *fakeTransport) Send(command string, read bool, pause time.Duration) (string, error) {
	f.commands = append(f.commands, command)
	if read {
		return "00,00000", nil
	}
	return "", nil
}

func (f *fakeTransport) WriteRaw(payload []byte) error {
	cp := append([]byte(nil), payload...)
	f.rawWrites = append(f.rawWrites, cp)
	return nil
}

func (f *fakeTransport) Close() error { return nil }
