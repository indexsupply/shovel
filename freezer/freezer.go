package freezer

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path"
	"sync"
)

type fname struct {
	name string
	num  int
	ext  string
}

func (fn fname) String() string {
	if fn.num < 0 {
		return fmt.Sprintf("%s.%s", fn.name, fn.ext)
	}
	return fmt.Sprintf("%s.%04d.%s", fn.name, fn.num, fn.ext)
}

type FileCache interface {
	File(string, uint64) (*os.File, int, int64, error)
	ReaderAt(string, uint64) (io.ReaderAt, int, int64, error)
	Max(string) (uint64, error)
}

type fileCache struct {
	dir string

	// formatting strings can be expensive
	// so this map uses struct keys for faster
	// lookups
	sync.RWMutex
	files map[fname]*os.File
}

func New(dir string) FileCache {
	return &fileCache{
		dir:   dir,
		files: make(map[fname]*os.File),
	}
}

func (fc *fileCache) open(fn fname) (*os.File, error) {
	fc.RLock()
	f, ok := fc.files[fn]
	if ok {
		fc.RUnlock()
		return f, nil
	}
	fc.RUnlock()
	fc.Lock()
	defer fc.Unlock()
	f, err := os.Open(path.Join(fc.dir, fn.String()))
	if err != nil {
		return nil, err
	}
	fc.files[fn] = f
	return f, nil
}

// Returns the highest block number in the freezer
// Max(...) + 1 can be found in Geth's LevelDB
func (fc *fileCache) Max(table string) (uint64, error) {
	f, err := fc.open(fname{name: table, num: -1, ext: "cidx"})
	if err != nil {
		return 0, err
	}
	s, err := f.Stat()
	if err != nil {
		return 0, err
	}
	return uint64(s.Size()/6) - 2, nil
}

func (fc *fileCache) ReaderAt(table string, blockNum uint64) (io.ReaderAt, int, int64, error) {
	return fc.File(table, blockNum)
}

func (fc *fileCache) File(table string, blockNum uint64) (*os.File, int, int64, error) {
	idx, err := fc.open(fname{name: table, num: -1, ext: "cidx"})
	if err != nil {
		return nil, 0, 0, err
	}
	var b [12]byte
	n, err := idx.ReadAt(b[:], int64(blockNum*6))
	if err != nil {
		return nil, 0, 0, err
	}
	if n != 12 {
		return nil, 0, 0, fmt.Errorf("expeced to read 12 bytes from index")
	}
	var (
		cf = binary.BigEndian.Uint16(b[0:2])
		co = binary.BigEndian.Uint32(b[2:6])
		nf = binary.BigEndian.Uint16(b[6:8])
		no = binary.BigEndian.Uint32(b[8:12])
	)
	switch {
	case cf == nf:
		f, err := fc.open(fname{name: table, num: int(cf), ext: "cdat"})
		if err != nil {
			return nil, 0, 0, fmt.Errorf("opening dat file: %w", err)
		}
		return f, int(no - co), int64(co), nil
	default:
		f, err := fc.open(fname{name: table, num: int(nf), ext: "cdat"})
		if err != nil {
			return nil, 0, 0, fmt.Errorf("opening dat file: %w", err)
		}
		return f, int(no), int64(0), nil
	}
}
