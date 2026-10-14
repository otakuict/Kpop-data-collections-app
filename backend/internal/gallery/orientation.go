package gallery

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"image"
	"image/jpeg"
)

// jpegOrientation reads the EXIF orientation before JPEG decoding discards it.
func jpegOrientation(data []byte) int {
	if len(data) < 4 || data[0] != 0xff || data[1] != 0xd8 {
		return 1
	}
	for pos := 2; pos+4 <= len(data); {
		if data[pos] != 0xff {
			break
		}
		marker := data[pos+1]
		if marker == 0xda || marker == 0xd9 {
			break
		}
		length := int(binary.BigEndian.Uint16(data[pos+2 : pos+4]))
		if length < 2 || pos+2+length > len(data) {
			break
		}
		segment := data[pos+4 : pos+2+length]
		if marker == 0xe1 && bytes.HasPrefix(segment, []byte("Exif\x00\x00")) {
			if orientation := tiffOrientation(segment[6:]); orientation != 1 {
				return orientation
			}
		}
		pos += 2 + length
	}
	return 1
}

func tiffOrientation(data []byte) int {
	if len(data) < 8 {
		return 1
	}
	var order binary.ByteOrder
	switch string(data[:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return 1
	}
	if order.Uint16(data[2:4]) != 42 {
		return 1
	}
	offset := uint64(order.Uint32(data[4:8]))
	if offset+2 > uint64(len(data)) {
		return 1
	}
	count := uint64(order.Uint16(data[offset : offset+2]))
	for i := uint64(0); i < count; i++ {
		entry := offset + 2 + i*12
		if entry+12 > uint64(len(data)) {
			break
		}
		if order.Uint16(data[entry:entry+2]) == 0x0112 && order.Uint16(data[entry+2:entry+4]) == 3 && order.Uint32(data[entry+4:entry+8]) == 1 {
			orientation := int(order.Uint16(data[entry+8 : entry+10]))
			if orientation >= 1 && orientation <= 8 {
				return orientation
			}
		}
	}
	return 1
}

func orientImage(src image.Image, orientation int) image.Image {
	if orientation < 2 || orientation > 8 {
		return src
	}
	bounds := src.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	width, height := w, h
	if orientation >= 5 {
		width, height = h, w
	}
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dx, dy := x, y
			switch orientation {
			case 2:
				dx = w - 1 - x
			case 3:
				dx, dy = w-1-x, h-1-y
			case 4:
				dy = h - 1 - y
			case 5:
				dx, dy = y, x
			case 6:
				dx, dy = h-1-y, x
			case 7:
				dx, dy = h-1-y, w-1-x
			case 8:
				dx, dy = y, w-1-x
			}
			dst.Set(dx, dy, src.At(bounds.Min.X+x, bounds.Min.Y+y))
		}
	}
	return dst
}

func (s *Store) RotateImage(ctx context.Context, setID SetID, imageID ImageID) error {
	var data []byte
	err := s.db.QueryRowContext(ctx, "SELECT bytes FROM images WHERE id=? AND set_id=?", imageID, setID).Scan(&data)
	if err != nil {
		return err
	}
	src, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		return err
	}
	var out bytes.Buffer
	if err := jpeg.Encode(&out, orientImage(src, 6), &jpeg.Options{Quality: 82}); err != nil {
		return err
	}
	sum := sha256.Sum256(out.Bytes())
	result, err := s.db.ExecContext(ctx, "UPDATE images SET bytes=?,hash=? WHERE id=? AND set_id=?", out.Bytes(), hex.EncodeToString(sum[:]), imageID, setID)
	if err != nil {
		return err
	}
	if count, err := result.RowsAffected(); err != nil || count == 0 {
		return sql.ErrNoRows
	}
	return nil
}
