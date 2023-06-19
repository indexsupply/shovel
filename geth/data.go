package geth

import (
	"fmt"

	"github.com/golang/snappy"
	"github.com/indexsupply/x/freezer"
	"github.com/indexsupply/x/geth/schema"
	"github.com/indexsupply/x/isxhash"
	"github.com/indexsupply/x/jrpc"
)

func Latest(rc *jrpc.Client) ([]byte, []byte, error) {
	hash, err := rc.Get1([]byte("LastBlock"))
	if err != nil {
		return nil, nil, err
	}
	num, err := rc.Get1(append([]byte("H"), hash...))
	if err != nil {
		return nil, nil, err
	}
	return num, hash, nil
}

func Hash(n uint64, fc freezer.FileCache, rc *jrpc.Client) ([]byte, error) {
	fmax, err := fc.Max("headers")
	if err != nil {
		return nil, err
	}
	var res []byte
	switch {
	case n <= fmax:
		buf := Buffer{Number: n}
		err = fread(&buf, fc, "headers")
		res = isxhash.Keccak(buf.h)
	default:
		buf := make([]jrpc.HexBytes, 1)
		req := [][]byte{schema.Key("hashes", n, nil)}
		err = rc.Get(buf, req)
		res = buf[0]
	}
	return res, err
}

func Load(filter [][]byte, dst []Buffer, fc freezer.FileCache, rc *jrpc.Client) error {
	fmax, err := fc.Max("headers")
	if err != nil {
		return fmt.Errorf("loading max freezer: %w", err)
	}
	var first, last = dst[0], dst[len(dst)-1]
	switch {
	case last.Number <= fmax:
		if err := fblocks(dst, fc); err != nil {
			return fmt.Errorf("loading freezer: %w", err)
		}
	case first.Number > fmax:
		if err := jblocks(dst, rc); err != nil {
			return fmt.Errorf("loading db: %w", err)
		}
	case first.Number <= fmax:
		var split int
		for i, b := range dst {
			if b.Number == fmax+1 {
				split = i
				break
			}
		}
		if err := fblocks(dst[:split], fc); err != nil {
			return fmt.Errorf("loading freezer: %w", err)
		}
		if err := jblocks(dst[split:], rc); err != nil {
			return fmt.Errorf("loading db: %w", err)
		}
	default:
		panic("corrupt query")
	}
	return nil
}

type Buffer struct {
	Number  uint64
	hash    []byte
	h, b, r []byte
	snappy  []byte
}

func (b *Buffer) Hash() []byte     { return b.hash }
func (b *Buffer) Header() []byte   { return b.h }
func (b *Buffer) Bodies() []byte   { return b.b }
func (b *Buffer) Receipts() []byte { return b.r }

func fread(buf *Buffer, fc freezer.FileCache, t string) error {
	f, length, offset, err := fc.ReaderAt(t, buf.Number)
	if err != nil {
		return fmt.Errorf("getting file to read: %w", err)
	}
	buf.snappy = grow(buf.snappy, length)
	buf.snappy = buf.snappy[:length]
	nread, err := f.ReadAt(buf.snappy, offset)
	if err != nil {
		return fmt.Errorf("reading file: %w", err)
	}
	if nread != length {
		return fmt.Errorf("readat mismatch want: %d got: %d", length, nread)
	}
	slen, err := snappy.DecodedLen(buf.snappy)
	if err != nil {
		return fmt.Errorf("reading snappy length: %w", err)
	}
	switch t {
	case "headers":
		buf.h, err = snappy.Decode(grow(buf.h, slen), buf.snappy)
	case "bodies":
		buf.b, err = snappy.Decode(grow(buf.b, slen), buf.snappy)
	case "receipts":
		buf.r, err = snappy.Decode(grow(buf.r, slen), buf.snappy)
	}
	return err
}

func fblocks(dst []Buffer, fc freezer.FileCache) error {
	for i := range dst {
		if err := fread(&dst[i], fc, "headers"); err != nil {
			return fmt.Errorf("unable to read header: %w", err)
		}
		if err := fread(&dst[i], fc, "bodies"); err != nil {
			return fmt.Errorf("unable to read bodies: %w", err)
		}
		if err := fread(&dst[i], fc, "receipts"); err != nil {
			return fmt.Errorf("unable to read receipts: %w", err)
		}
	}
	return nil
}

type rpcBuffer struct {
	keys [][]byte
	vals []jrpc.HexBytes
}

func grow(s []byte, n int) []byte {
	if len(s) < n {
		s = append(s, make([]byte, n-len(s))...)
	}
	return s[:n]
}

func gcopy(dst, src []byte) []byte {
	dst = grow(dst, len(src))
	copy(dst, src)
	return dst
}

func jread(rb *rpcBuffer, dst []Buffer, rc *jrpc.Client, t string) error {
	for i := 0; i < len(dst); i++ {
		rb.keys[i] = schema.Key(t, dst[i].Number, dst[i].hash)
	}
	if err := rc.Get(rb.vals, rb.keys); err != nil {
		return fmt.Errorf("unable to load %s: %w", t, err)
	}
	for i := range rb.vals {
		switch t {
		case "headers":
			dst[i].h = gcopy(dst[i].h, rb.vals[i])
		case "bodies":
			dst[i].b = gcopy(dst[i].b, rb.vals[i])
		case "receipts":
			dst[i].r = gcopy(dst[i].r, rb.vals[i])
		default:
			return fmt.Errorf("jread unknown table %s", t)
		}
	}
	return nil
}

func setHashes(rb *rpcBuffer, dst []Buffer, rc *jrpc.Client) error {
	for i := range dst {
		rb.keys[i] = schema.Key("hashes", dst[i].Number, nil)
	}
	if err := rc.Get(rb.vals, rb.keys); err != nil {
		return fmt.Errorf("getting hashes: %w", err)
	}
	for i := 0; i < len(rb.vals); i++ {
		dst[i].hash = gcopy(dst[i].hash, rb.vals[i])
	}
	return nil
}

func jblocks(dst []Buffer, rc *jrpc.Client) error {
	rb := rpcBuffer{
		keys: make([][]byte, len(dst)),
		vals: make([]jrpc.HexBytes, len(dst)),
	}
	if err := setHashes(&rb, dst, rc); err != nil {
		return fmt.Errorf("requesting hashes: %w", err)
	}
	if err := jread(&rb, dst, rc, "headers"); err != nil {
		return fmt.Errorf("requesting headers: %w", err)
	}
	if err := jread(&rb, dst, rc, "bodies"); err != nil {
		return fmt.Errorf("requesting bodies: %w", err)
	}
	if err := jread(&rb, dst, rc, "receipts"); err != nil {
		return fmt.Errorf("requesting receipts: %w", err)
	}
	return nil
}
