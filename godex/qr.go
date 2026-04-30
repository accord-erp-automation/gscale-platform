package godex

import (
	"fmt"

	qrcode "github.com/skip2/go-qrcode"
)

func RenderQRGraphic(payload string, boxDots int) ([]byte, error) {
	if payload == "" {
		return nil, fmt.Errorf("qr payload is empty")
	}
	if boxDots <= 0 {
		return nil, fmt.Errorf("qr box dots must be positive")
	}
	qr, err := qrcode.New(payload, qrcode.Low)
	if err != nil {
		return nil, fmt.Errorf("build qr: %w", err)
	}
	return EncodeMonoBMP(qr.Image(boxDots))
}
