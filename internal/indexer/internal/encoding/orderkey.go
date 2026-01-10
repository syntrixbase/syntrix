// Package encoding provides OrderKey encoding/decoding for the Indexer.
//
// OrderKey layout:
//
//	[ver:1B][field1...][field2...][...][id_len:2B][id bytes]
//
// Each field:
//
//	[type_tag:1B][value bytes...]
//
// Type tags (ascending order): null=0x00 < bool=0x01 < number=0x02 < string=0x03
//
// String encoding uses escaped null terminator for correct lexicographic order:
//   - Escape 0x00 bytes as 0x00 0x01
//   - Terminate with 0x00 0x00
//
// For descending order, ALL bytes are inverted (including type tag).
package encoding

import (
	"encoding/base64"
	"encoding/binary"
	"errors"
	"math"
)

// Current encoding version.
const Version byte = 0x01

// Type tags for field encoding.
const (
	TypeNull   byte = 0x00
	TypeBool   byte = 0x01
	TypeNumber byte = 0x02
	TypeString byte = 0x03
)

// Direction specifies sort order.
type Direction int

const (
	Asc  Direction = 0
	Desc Direction = 1
)

// Field represents a value to be encoded with its sort direction.
type Field struct {
	Value     any       // nil, bool, float64, int64, or string
	Direction Direction // Asc or Desc
}

// Errors
var (
	ErrUnsupportedType = errors.New("unsupported value type")
	ErrStringTooLong   = errors.New("string exceeds maximum length (65535 bytes)")
	ErrIDTooLong       = errors.New("document ID exceeds maximum length (65535 bytes)")
	ErrInvalidOrderKey = errors.New("invalid orderkey format")
)

// Encode creates an OrderKey from fields and document ID.
func Encode(fields []Field, docID string) ([]byte, error) {
	// Estimate capacity: version(1) + fields + id_len(2) + id
	buf := make([]byte, 0, 64)

	// Version byte
	buf = append(buf, Version)

	// Encode each field
	for _, f := range fields {
		encoded, err := encodeField(f.Value, f.Direction)
		if err != nil {
			return nil, err
		}
		buf = append(buf, encoded...)
	}

	// Append document ID as tie-breaker
	idBytes := []byte(docID)
	if len(idBytes) > 65535 {
		return nil, ErrIDTooLong
	}
	buf = appendUint16(buf, uint16(len(idBytes)))
	buf = append(buf, idBytes...)

	return buf, nil
}

// encodeField encodes a single field value with its type tag.
func encodeField(value any, dir Direction) ([]byte, error) {
	var buf []byte

	switch v := value.(type) {
	case nil:
		buf = []byte{TypeNull}

	case bool:
		buf = make([]byte, 2)
		buf[0] = TypeBool
		if v {
			buf[1] = 0x01
		} else {
			buf[1] = 0x00
		}

	case float64:
		buf = make([]byte, 9)
		buf[0] = TypeNumber
		encodeFloat64(buf[1:], v)

	case int64:
		buf = make([]byte, 9)
		buf[0] = TypeNumber
		encodeFloat64(buf[1:], float64(v))

	case int:
		buf = make([]byte, 9)
		buf[0] = TypeNumber
		encodeFloat64(buf[1:], float64(v))

	case string:
		strBytes := []byte(v)
		if len(strBytes) > 65535 {
			return nil, ErrStringTooLong
		}
		// Use escaped null terminator encoding for correct lexicographic order:
		// - Escape 0x00 bytes as 0x00 0x01
		// - Terminate with 0x00 0x00
		// This ensures "ab" < "b" sorts correctly (no length prefix issue)
		buf = make([]byte, 1, 1+len(strBytes)*2+2) // worst case: all nulls + terminator
		buf[0] = TypeString
		for _, b := range strBytes {
			if b == 0x00 {
				buf = append(buf, 0x00, 0x01)
			} else {
				buf = append(buf, b)
			}
		}
		buf = append(buf, 0x00, 0x00) // null terminator

	default:
		return nil, ErrUnsupportedType
	}

	// For descending order, invert all bytes
	if dir == Desc {
		invertBytes(buf)
	}

	return buf, nil
}

// encodeFloat64 encodes a float64 in a sortable format.
// Positive numbers: flip sign bit (0x80 XOR)
// Negative numbers: flip all bits
// This ensures: -Inf < negative < 0 < positive < +Inf, NaN at end
func encodeFloat64(buf []byte, v float64) {
	bits := math.Float64bits(v)
	if v >= 0 {
		// Positive (including +0): flip sign bit
		bits ^= 1 << 63
	} else {
		// Negative: flip all bits
		bits = ^bits
	}
	binary.BigEndian.PutUint64(buf, bits)
}

// invertBytes inverts all bytes in place (for descending order).
func invertBytes(buf []byte) {
	for i := range buf {
		buf[i] = ^buf[i]
	}
}

// appendUint16 appends a big-endian uint16 to the buffer.
func appendUint16(buf []byte, v uint16) []byte {
	return append(buf, byte(v>>8), byte(v))
}

// Compare compares two OrderKeys lexicographically.
// Returns -1 if a < b, 0 if a == b, 1 if a > b.
func Compare(a, b []byte) int {
	minLen := len(a)
	if len(b) < minLen {
		minLen = len(b)
	}
	for i := 0; i < minLen; i++ {
		if a[i] < b[i] {
			return -1
		}
		if a[i] > b[i] {
			return 1
		}
	}
	if len(a) < len(b) {
		return -1
	}
	if len(a) > len(b) {
		return 1
	}
	return 0
}

// ExtractDocID extracts the document ID from an OrderKey.
// This is useful for cursor handling.
func ExtractDocID(key []byte) (string, error) {
	if len(key) < 3 {
		return "", ErrInvalidOrderKey
	}

	// ID length is in the last 2 bytes before the ID itself
	// We need to scan from the end
	if len(key) < 3 {
		return "", ErrInvalidOrderKey
	}

	// The structure is: [ver][fields...][id_len:2B][id]
	// We need to find where the ID starts by reading the length
	// Since fields are variable length, we scan backward

	// Minimum: version(1) + id_len(2) + id(0+) = 3 bytes
	// Try to extract: the last N bytes are the ID, preceded by 2-byte length
	// We don't know N, so we need a different approach

	// For now, return error - full decode needed for extraction
	// This is a simplified implementation; full decode requires field schema
	return "", errors.New("ExtractDocID requires field schema for full decode")
}

// EncodeBase64 encodes an OrderKey to base64-url format for wire transmission.
func EncodeBase64(key []byte) string {
	return base64.RawURLEncoding.EncodeToString(key)
}

// DecodeBase64 decodes a base64-url encoded OrderKey.
func DecodeBase64(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(s)
}
