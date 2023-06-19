package schema

import (
	"encoding/binary"

	"github.com/indexsupply/x/bint"
)

func Key(t string, n uint64, h []byte) (k []byte) {
	switch t {
	case "hashes":
		k = append(k, 'h')
		k = append(k, binary.BigEndian.AppendUint64([]byte{}, n)...)
		k = append(k, 'n')
	case "headers":
		k = append(k, 'h')
		k = append(k, binary.BigEndian.AppendUint64([]byte{}, n)...)
		k = append(k, h...)
	case "bodies":
		k = append(k, 'b')
		k = append(k, binary.BigEndian.AppendUint64([]byte{}, n)...)
		k = append(k, h...)
	case "receipts":
		k = append(k, 'r')
		k = append(k, binary.BigEndian.AppendUint64([]byte{}, n)...)
		k = append(k, h...)
	default:
		return nil
	}
	return
}

func ParseKey(k []byte) (string, uint64, []byte) {
	switch {
	case k[0] == 'h' && len(k) == 10:
		return "hashes", bint.Uint64(k[1:9]), nil
	case k[0] == 'h':
		return "headers", bint.Uint64(k[1:9]), k[9:]
	case k[0] == 'b':
		return "bodies", bint.Uint64(k[1:9]), k[9:]
	case k[0] == 'r':
		return "receipts", bint.Uint64(k[1:9]), k[9:]
	default:
		return "", 0, nil
	}
}
