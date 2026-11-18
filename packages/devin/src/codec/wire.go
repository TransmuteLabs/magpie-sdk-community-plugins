package devin

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"fmt"
	"io"
	"unicode/utf8"
)

const (
	maxBody   = 16 << 20
	maxFields = 16384
	textField = 6 // A UTF-8 string, carried on protobuf wire type 2.
	repeated  = 16
)

type field struct {
	n int
	w int
	u uint64
	b []byte
}

type message map[int][]field

func (m message) number(n int) uint64 {
	if len(m[n]) == 0 {
		return 0
	}
	return m[n][0].u
}

func (m message) data(n int) []byte {
	if len(m[n]) == 0 {
		return nil
	}
	return m[n][0].b
}

func (m message) text(n int) string { return string(m.data(n)) }

// readMessage rejects unknown fields and duplicate singular fields. A nil
// schema only validates the envelope of discarded credential metadata.
func readMessage(b []byte, schema map[int]int) (message, error) {
	m := message{}
	for count := 0; len(b) > 0; count++ {
		if count >= maxFields {
			return nil, fmt.Errorf("too many protobuf fields")
		}
		key, used := binary.Uvarint(b)
		if used <= 0 || key>>3 == 0 || key>>3 > (1<<29)-1 {
			return nil, fmt.Errorf("invalid protobuf tag")
		}
		b = b[used:]
		f := field{n: int(key >> 3), w: int(key & 7)}
		spec, ok := schema[f.n]
		if schema != nil {
			if !ok {
				return nil, fmt.Errorf("unsupported protobuf field %d", f.n)
			}
			wire := spec & 7
			if wire == textField {
				wire = 2
			}
			if wire != f.w || (spec&repeated == 0 && len(m[f.n]) != 0) {
				return nil, fmt.Errorf("invalid protobuf field %d", f.n)
			}
		}
		switch f.w {
		case 0:
			f.u, used = binary.Uvarint(b)
			if used <= 0 {
				return nil, fmt.Errorf("invalid protobuf varint")
			}
			b = b[used:]
		case 1, 5:
			size := 8
			if f.w == 5 {
				size = 4
			}
			if len(b) < size {
				return nil, io.ErrUnexpectedEOF
			}
			f.b, b = b[:size], b[size:]
		case 2:
			n, size := binary.Uvarint(b)
			if size <= 0 || n > uint64(len(b)-size) {
				return nil, io.ErrUnexpectedEOF
			}
			f.b, b = b[size:size+int(n)], b[size+int(n):]
			if schema != nil && spec&7 == textField && !utf8.Valid(f.b) {
				return nil, fmt.Errorf("invalid protobuf string")
			}
		default:
			return nil, fmt.Errorf("unsupported protobuf wire type")
		}
		m[f.n] = append(m[f.n], f)
	}
	return m, nil
}

func unframe(b []byte) ([]byte, error) {
	if len(b) < 5 || len(b) > maxBody+5 {
		return nil, fmt.Errorf("invalid Connect frame size")
	}
	if b[0] != 0 && b[0] != 1 {
		return nil, fmt.Errorf("unsupported Connect frame flags")
	}
	if uint64(binary.BigEndian.Uint32(b[1:5])) != uint64(len(b)-5) {
		return nil, fmt.Errorf("expected exactly one complete Connect data frame")
	}
	if b[0] == 0 {
		return b[5:], nil
	}
	src := bytes.NewReader(b[5:])
	z, err := gzip.NewReader(src)
	if err != nil {
		return nil, fmt.Errorf("invalid Connect gzip")
	}
	z.Multistream(false)
	out, err := io.ReadAll(io.LimitReader(z, maxBody+1))
	closeErr := z.Close()
	if err != nil || closeErr != nil || src.Len() != 0 {
		return nil, fmt.Errorf("invalid Connect gzip")
	}
	if len(out) > maxBody {
		return nil, fmt.Errorf("Connect message exceeds limit")
	}
	return out, nil
}

func addNumber(b []byte, n int, v uint64) []byte {
	b = binary.AppendUvarint(b, uint64(n<<3))
	return binary.AppendUvarint(b, v)
}

func addBytes(b []byte, n int, v []byte) []byte {
	b = binary.AppendUvarint(b, uint64(n<<3|2))
	b = binary.AppendUvarint(b, uint64(len(v)))
	return append(b, v...)
}

func addText(b []byte, n int, v string) []byte {
	if v == "" {
		return b
	}
	return addBytes(b, n, []byte(v))
}
