// Package worldinfo reads just enough of Minecraft's NBT world format to
// answer one question: what Minecraft version was this world last saved
// with? (requirements.md: verifying exported/imported world data's version.)
// It's a minimal, read-only NBT decoder -- not a general-purpose NBT
// library -- since that's all extracting Data.Version from level.dat needs.
package worldinfo

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
)

const (
	tagEnd = iota
	tagByte
	tagShort
	tagInt
	tagLong
	tagFloat
	tagDouble
	tagByteArray
	tagString
	tagList
	tagCompound
	tagIntArray
	tagLongArray
)

// decodeRootCompound parses a full NBT stream (one named root compound tag,
// the format level.dat and similar files use) into nested Go values:
// map[string]any for compounds, []any for lists, and int8/int16/int32/int64/
// float32/float64/string/[]byte/[]int32/[]int64 for primitives.
func decodeRootCompound(r io.Reader) (map[string]any, error) {
	nr := &nbtReader{r: r}
	tagType, err := nr.readByte()
	if err != nil {
		return nil, err
	}
	if tagType != tagCompound {
		return nil, fmt.Errorf("unexpected root tag type %d (expected compound)", tagType)
	}
	if _, err := nr.readString(); err != nil { // root tag's own name, discarded
		return nil, err
	}
	return nr.readCompoundPayload()
}

type nbtReader struct {
	r io.Reader
}

func (nr *nbtReader) readByte() (byte, error) {
	var buf [1]byte
	if _, err := io.ReadFull(nr.r, buf[:]); err != nil {
		return 0, err
	}
	return buf[0], nil
}

func (nr *nbtReader) readInt32() (int32, error) {
	var buf [4]byte
	if _, err := io.ReadFull(nr.r, buf[:]); err != nil {
		return 0, err
	}
	return int32(binary.BigEndian.Uint32(buf[:])), nil
}

func (nr *nbtReader) readString() (string, error) {
	var lenBuf [2]byte
	if _, err := io.ReadFull(nr.r, lenBuf[:]); err != nil {
		return "", err
	}
	n := binary.BigEndian.Uint16(lenBuf[:])
	if n == 0 {
		return "", nil
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(nr.r, buf); err != nil {
		return "", err
	}
	return string(buf), nil
}

func (nr *nbtReader) readCompoundPayload() (map[string]any, error) {
	out := map[string]any{}
	for {
		tagType, err := nr.readByte()
		if err != nil {
			return nil, err
		}
		if tagType == tagEnd {
			return out, nil
		}
		name, err := nr.readString()
		if err != nil {
			return nil, err
		}
		val, err := nr.readPayload(tagType)
		if err != nil {
			return nil, err
		}
		out[name] = val
	}
}

func (nr *nbtReader) readPayload(tagType byte) (any, error) {
	switch tagType {
	case tagByte:
		b, err := nr.readByte()
		return int8(b), err
	case tagShort:
		var buf [2]byte
		if _, err := io.ReadFull(nr.r, buf[:]); err != nil {
			return nil, err
		}
		return int16(binary.BigEndian.Uint16(buf[:])), nil
	case tagInt:
		return nr.readInt32()
	case tagLong:
		var buf [8]byte
		if _, err := io.ReadFull(nr.r, buf[:]); err != nil {
			return nil, err
		}
		return int64(binary.BigEndian.Uint64(buf[:])), nil
	case tagFloat:
		var buf [4]byte
		if _, err := io.ReadFull(nr.r, buf[:]); err != nil {
			return nil, err
		}
		return math.Float32frombits(binary.BigEndian.Uint32(buf[:])), nil
	case tagDouble:
		var buf [8]byte
		if _, err := io.ReadFull(nr.r, buf[:]); err != nil {
			return nil, err
		}
		return math.Float64frombits(binary.BigEndian.Uint64(buf[:])), nil
	case tagByteArray:
		n, err := nr.readInt32()
		if err != nil {
			return nil, err
		}
		buf := make([]byte, n)
		if n > 0 {
			if _, err := io.ReadFull(nr.r, buf); err != nil {
				return nil, err
			}
		}
		return buf, nil
	case tagString:
		return nr.readString()
	case tagList:
		elemType, err := nr.readByte()
		if err != nil {
			return nil, err
		}
		n, err := nr.readInt32()
		if err != nil {
			return nil, err
		}
		list := make([]any, 0)
		for i := int32(0); i < n; i++ {
			v, err := nr.readPayload(elemType)
			if err != nil {
				return nil, err
			}
			list = append(list, v)
		}
		return list, nil
	case tagCompound:
		return nr.readCompoundPayload()
	case tagIntArray:
		n, err := nr.readInt32()
		if err != nil {
			return nil, err
		}
		arr := make([]int32, n)
		for i := range arr {
			v, err := nr.readInt32()
			if err != nil {
				return nil, err
			}
			arr[i] = v
		}
		return arr, nil
	case tagLongArray:
		n, err := nr.readInt32()
		if err != nil {
			return nil, err
		}
		arr := make([]int64, n)
		for i := range arr {
			var buf [8]byte
			if _, err := io.ReadFull(nr.r, buf[:]); err != nil {
				return nil, err
			}
			arr[i] = int64(binary.BigEndian.Uint64(buf[:]))
		}
		return arr, nil
	default:
		return nil, fmt.Errorf("unknown nbt tag type %d", tagType)
	}
}
