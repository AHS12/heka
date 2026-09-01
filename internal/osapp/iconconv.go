package osapp

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/png"
)

// pngToICO converts PNG bytes to ICO format (required by Windows systray).
// The ICO file contains one 32x32 PNG-compressed image entry.
func pngToICO(pngBytes []byte) ([]byte, error) {
	img, _, err := image.Decode(bytes.NewReader(pngBytes))
	if err != nil {
		return nil, err
	}

	// Resize to 32x32 using nearest-neighbor (simple, no deps).
	const size = 32
	small := image.NewRGBA(image.Rect(0, 0, size, size))
	bounds := img.Bounds()
	sx := float64(bounds.Dx()) / float64(size)
	sy := float64(bounds.Dy()) / float64(size)
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			small.Set(x, y, img.At(bounds.Min.X+int(float64(x)*sx), bounds.Min.Y+int(float64(y)*sy)))
		}
	}

	// Encode the small image as PNG (ICO embeds PNG for 32-bit icons).
	var imgBuf bytes.Buffer
	if err := png.Encode(&imgBuf, small); err != nil {
		return nil, err
	}
	imgData := imgBuf.Bytes()

	// Build ICO file:
	// Header (6 bytes) + 1 directory entry (16 bytes) + image data
	buf := new(bytes.Buffer)

	// ICO header
	if err := binary.Write(buf, binary.LittleEndian, uint16(0)); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.LittleEndian, uint16(1)); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.LittleEndian, uint16(1)); err != nil {
		return nil, err
	}

	// Directory entry
	buf.WriteByte(byte(size)) // Width
	buf.WriteByte(byte(size)) // Height
	buf.WriteByte(0)          // Color palette
	buf.WriteByte(0)          // Reserved
	if err := binary.Write(buf, binary.LittleEndian, uint16(1)); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.LittleEndian, uint16(32)); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.LittleEndian, uint32(len(imgData))); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.LittleEndian, uint32(6+16)); err != nil {
		return nil, err
	}

	// Image data (PNG-compressed)
	buf.Write(imgData)

	return buf.Bytes(), nil
}
