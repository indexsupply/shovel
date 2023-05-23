package gethdb

import (
	"encoding/binary"
	"fmt"
	"os"
	"path"
	"sync"

	"github.com/golang/snappy"
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

type Freezer struct {
	dir string

	// formatting strings can be expensive
	// so this map uses struct keys for faster
	// lookups
	sync.RWMutex
	files map[fname]*os.File
}

func NewFreezer(dir string) *Freezer {
	return &Freezer{
		dir:   dir,
		files: make(map[fname]*os.File),
	}
}

func (fr *Freezer) open(fn fname) (*os.File, error) {
	fr.RLock()
	f, ok := fr.files[fn]
	if ok {
		fr.RUnlock()
		return f, nil
	}
	fr.RUnlock()
	fr.Lock()
	defer fr.Unlock()
	f, err := os.Open(path.Join(fr.dir, fn.String()))
	if err != nil {
		return nil, err
	}
	fr.files[fn] = f
	return f, nil
}

// Returns the highest block number in the freezer
// Max(...) + 1 can be found in Geth's LevelDB
func (fr *Freezer) Max(table string) (uint64, error) {
	f, err := fr.open(fname{name: table, num: -1, ext: "cidx"})
	if err != nil {
		return 0, err
	}
	s, err := f.Stat()
	if err != nil {
		return 0, err
	}
	return uint64(s.Size()/6) - 2, nil
}

// Returns the current and next file number and file offset for a given block
// If the requested block and next block are contained in the same file
// the number of bytes to read is computed as follows: nextOffest - currOffset
// If the requested block and the next block are in different files,
// the block's data is located at the beginning of the next file and
// the number of bytes to read is computed as follows: nextOffest
func (fr *Freezer) FileNum(table string, bn uint64) (uint16, uint32, uint16, uint32) {
	idx, err := fr.open(fname{name: table, num: -1, ext: "cidx"})
	if err != nil {
		return 0, 0, 0, 0
	}
	var b [12]byte
	n, err := idx.ReadAt(b[:], int64(bn*6))
	if err != nil {
		return 0, 0, 0, 0
	}
	if n != 12 {
		return 0, 0, 0, 0
	}
	var (
		cf = binary.BigEndian.Uint16(b[0:2])
		co = binary.BigEndian.Uint32(b[2:6])
		nf = binary.BigEndian.Uint16(b[6:8])
		no = binary.BigEndian.Uint32(b[8:12])
	)
	return cf, co, nf, no
}

func (fr *Freezer) Read(dst []byte, table string, bn uint64) ([]byte, error) {
	currFile, currOffset, nextFile, nextOffest := fr.FileNum(table, bn)
	var buf []byte
	switch {
	case currFile == nextFile:
		f, err := fr.open(fname{name: table, num: int(currFile), ext: "cdat"})
		if err != nil {
			return nil, fmt.Errorf("opening dat file: %w", err)
		}
		buf = make([]byte, int(nextOffest-currOffset))
		if _, err := f.ReadAt(buf, int64(currOffset)); err != nil {
			return nil, fmt.Errorf("reading dat file: %w", err)
		}
	case currFile != nextFile:
		f, err := fr.open(fname{name: table, num: int(nextFile), ext: "cdat"})
		if err != nil {
			return nil, fmt.Errorf("opening current: %w", err)
		}
		buf = make([]byte, int(nextOffest))
		if _, err := f.ReadAt(buf, 0); err != nil {
			return nil, fmt.Errorf("reading dat file: %w", err)
		}
	}
	return snappy.Decode(dst[:cap(dst)], buf)
}
