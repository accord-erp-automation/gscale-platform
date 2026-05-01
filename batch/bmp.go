package batch

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
)

func EncodeMonoBMP(src image.Image) ([]byte, error) {
	b := src.Bounds()
	width := b.Dx()
	height := b.Dy()
	rowBytes := ((width + 31) / 32) * 4
	pixelBytes := rowBytes * height
	const headerBytes = 14 + 40 + 8
	fileBytes := headerBytes + pixelBytes

	var out bytes.Buffer
	out.Grow(fileBytes)
	out.Write([]byte{'B', 'M'})
	writeU32(&out, uint32(fileBytes))
	writeU16(&out, 0)
	writeU16(&out, 0)
	writeU32(&out, headerBytes)

	writeU32(&out, 40)
	writeI32(&out, int32(width))
	writeI32(&out, int32(height))
	writeU16(&out, 1)
	writeU16(&out, 1)
	writeU32(&out, 0)
	writeU32(&out, uint32(pixelBytes))
	writeI32(&out, 0)
	writeI32(&out, 0)
	writeU32(&out, 2)
	writeU32(&out, 2)

	out.Write([]byte{0x00, 0x00, 0x00, 0x00})
	out.Write([]byte{0xff, 0xff, 0xff, 0x00})

	row := make([]byte, rowBytes)
	for y := b.Max.Y - 1; y >= b.Min.Y; y-- {
		for i := range row {
			row[i] = 0
		}
		for x := b.Min.X; x < b.Max.X; x++ {
			if isLight(src.At(x, y)) {
				relX := x - b.Min.X
				row[relX/8] |= 0x80 >> uint(relX%8)
			}
		}
		out.Write(row)
	}
	return out.Bytes(), nil
}

func isLight(c color.Color) bool {
	r, g, b, _ := c.RGBA()
	y := (299*r + 587*g + 114*b) / 1000
	return y > 0x7fff
}

func writeU16(buf *bytes.Buffer, v uint16) {
	_ = binary.Write(buf, binary.LittleEndian, v)
}

func writeU32(buf *bytes.Buffer, v uint32) {
	_ = binary.Write(buf, binary.LittleEndian, v)
}

func writeI32(buf *bytes.Buffer, v int32) {
	_ = binary.Write(buf, binary.LittleEndian, v)
}
