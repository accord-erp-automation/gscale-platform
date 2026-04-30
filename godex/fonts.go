package godex

import (
	"fmt"
	"image"
	"image/color"
	"os"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

type FontSet struct {
	Regular20 font.Face
	Regular26 font.Face
	Bold21    font.Face
	Bold24    font.Face
}

func LoadFontSet(regularPath, boldPath string) (*FontSet, error) {
	regular20, err := loadFontFace(regularPath, 20)
	if err != nil {
		return nil, err
	}
	regular26, err := loadFontFace(regularPath, 26)
	if err != nil {
		_ = regular20.Close()
		return nil, err
	}
	bold21, err := loadFontFace(boldPath, 21)
	if err != nil {
		_ = regular20.Close()
		_ = regular26.Close()
		return nil, err
	}
	bold24, err := loadFontFace(boldPath, 24)
	if err != nil {
		_ = regular20.Close()
		_ = regular26.Close()
		_ = bold21.Close()
		return nil, err
	}
	return &FontSet{
		Regular20: regular20,
		Regular26: regular26,
		Bold21:    bold21,
		Bold24:    bold24,
	}, nil
}

func (fs *FontSet) Close() {
	if fs == nil {
		return
	}
	for _, face := range []font.Face{fs.Regular20, fs.Regular26, fs.Bold21, fs.Bold24} {
		if face != nil {
			_ = face.Close()
		}
	}
}

func loadFontFace(path string, size float64) (font.Face, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read font %s: %w", path, err)
	}
	ttf, err := opentype.Parse(b)
	if err != nil {
		return nil, fmt.Errorf("parse font %s: %w", path, err)
	}
	face, err := opentype.NewFace(ttf, &opentype.FaceOptions{
		Size:    size,
		DPI:     72,
		Hinting: font.HintingFull,
	})
	if err != nil {
		return nil, fmt.Errorf("create font face %s: %w", path, err)
	}
	return face, nil
}

func textWidth(face font.Face, text string) int {
	if text == "" {
		return 0
	}
	bounds, _ := font.BoundString(face, text)
	return (bounds.Max.X - bounds.Min.X).Ceil()
}

func drawTextTop(dst *image.RGBA, x, y int, face font.Face, text string) {
	if text == "" {
		return
	}
	bounds, _ := font.BoundString(face, text)
	baseline := y - bounds.Min.Y.Ceil()
	d := font.Drawer{
		Dst:  dst,
		Src:  image.NewUniform(color.Black),
		Face: face,
		Dot:  fixed.P(x, baseline),
	}
	d.DrawString(text)
}
