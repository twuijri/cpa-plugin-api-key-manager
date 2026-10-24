package desktop

import (
	"encoding/binary"
	"fmt"
)

func number(field int, value uint64) []byte {
	output := binary.AppendUvarint(nil, uint64(field<<3))
	return binary.AppendUvarint(output, value)
}

func message(field int, value []byte) []byte {
	output := binary.AppendUvarint(nil, uint64(field<<3|2))
	output = binary.AppendUvarint(output, uint64(len(value)))
	return append(output, value...)
}

type wireField struct {
	Number  int
	Kind    int
	Bytes   []byte
	Integer uint64
}

func parseWire(data []byte) ([]wireField, error) {
	var out []wireField
	for len(data) > 0 {
		tag, n := binary.Uvarint(data)
		if n <= 0 || tag>>3 == 0 {
			return nil, fmt.Errorf("invalid protobuf tag")
		}
		data = data[n:]
		f := wireField{Number: int(tag >> 3), Kind: int(tag & 7)}
		switch f.Kind {
		case 0:
			value, n := binary.Uvarint(data)
			if n <= 0 {
				return nil, fmt.Errorf("invalid protobuf integer")
			}
			f.Integer, data = value, data[n:]
		case 2:
			length, n := binary.Uvarint(data)
			if n <= 0 || length > uint64(len(data)-n) {
				return nil, fmt.Errorf("invalid protobuf length")
			}
			data = data[n:]
			f.Bytes, data = data[:int(length)], data[int(length):]
		case 1, 5:
			length := 8
			if f.Kind == 5 {
				length = 4
			}
			if len(data) < length {
				return nil, fmt.Errorf("truncated protobuf field")
			}
			f.Bytes, data = data[:length], data[length:]
		default:
			return nil, fmt.Errorf("unsupported protobuf wire type")
		}
		out = append(out, f)
		if len(out) > 65536 {
			return nil, fmt.Errorf("protobuf frame contains too many fields")
		}
	}
	return out, nil
}

func nested(data []byte, path ...int) ([][]byte, error) {
	values := [][]byte{data}
	for _, field := range path {
		var next [][]byte
		for _, value := range values {
			parsed, err := parseWire(value)
			if err != nil {
				return nil, err
			}
			for _, item := range parsed {
				if item.Number == field && item.Kind == 2 {
					next = append(next, item.Bytes)
				}
			}
		}
		values = next
	}
	return values, nil
}

// Field and extension numbers are from Gemini macOS 1.135.1.1291 and a live
// ProcessQuery round trip. No Antigravity endpoint or credential is used.
func requestFrames(prompt string) [][]byte {
	metadata := number(4, 21) // RequestMetadata.client_type = MACOS.
	metadata = append(metadata, number(5, 1)...)
	metadata = append(metadata, message(6, nil)...)
	metadata = append(metadata, message(7, []byte("en"))...)
	metadata = append(metadata, message(2, message(1, []byte("en-US")))...)
	initial := message(1, metadata)
	initial = append(initial, message(1001, message(1, nil))...) // ClientGenerateFeature.template.
	initial = append(initial, message(1004, nil)...)             // PrefetchInputFeature.
	return [][]byte{
		message(1000, initial),
		message(1007, message(1, []byte(prompt))),
		message(1007, message(2, nil)),
	}
}

func responseText(frame []byte) (string, bool, error) {
	texts, err := nested(frame, 1003, 1, 1, 2, 1)
	if err != nil {
		return "", false, err
	}
	if len(texts) == 0 {
		return "", false, nil
	}
	return string(texts[0]), true, nil
}
