package freezer

import (
	"encoding/binary"
	"io/ioutil"
	"os"
	"path"
	"testing"

	"github.com/golang/snappy"
	"kr.dev/diff"
)

func fappend(t *testing.T, path string, data []byte) {
	t.Helper()
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0644)
	diff.Test(t, t.Fatalf, nil, err)
	n, err := file.Write(data)
	diff.Test(t, t.Fatalf, nil, err)
	if n != len(data) {
		t.Fatalf("expected to write %d wrote %d", len(data), n)
	}
}

func indexItem(file, offset int) []byte {
	var item = make([]byte, 6)
	binary.BigEndian.PutUint16(item[:2], uint16(file))
	binary.BigEndian.PutUint32(item[2:], uint32(offset))
	return item
}

func compress(b []byte) []byte {
	return snappy.Encode(nil, b)
}

func dumpFile(t *testing.T, name string) {
	b, err := ioutil.ReadFile(name)
	diff.Test(t, t.Fatalf, nil, err)
	t.Logf("name: %s\n%x\n", name, b)
}

func TestRead(t *testing.T) {
	var (
		dir = t.TempDir()
		foo = []byte("foo")
		bar = []byte("bar")
		baz = []byte("baz")
	)
	fappend(t, path.Join(dir, "headers.0000.cdat"), compress(foo))
	fappend(t, path.Join(dir, "headers.0001.cdat"), compress(bar))
	fappend(t, path.Join(dir, "headers.0001.cdat"), compress(baz))
	fappend(t, path.Join(dir, "headers.cidx"), indexItem(0, 0))
	fappend(t, path.Join(dir, "headers.cidx"), indexItem(0, 5))
	fappend(t, path.Join(dir, "headers.cidx"), indexItem(1, 5))
	fappend(t, path.Join(dir, "headers.cidx"), indexItem(1, 10))

	dumpFile(t, path.Join(dir, "headers.0000.cdat"))
	dumpFile(t, path.Join(dir, "headers.cidx"))

	fc := &fileCache{
		dir:   dir,
		files: map[fname]*os.File{},
	}

	_, length, offset, err := fc.File("headers", 0)
	diff.Test(t, t.Fatalf, nil, err)
	diff.Test(t, t.Errorf, len(compress(foo)), length)
	diff.Test(t, t.Errorf, int64(0), offset)

	_, length, offset, err = fc.File("headers", 1)
	diff.Test(t, t.Fatalf, nil, err)
	diff.Test(t, t.Errorf, len(compress(bar)), length)
	diff.Test(t, t.Errorf, int64(0), offset)

	_, length, offset, err = fc.File("headers", 2)
	diff.Test(t, t.Fatalf, nil, err)
	diff.Test(t, t.Errorf, len(compress(baz)), length)
	diff.Test(t, t.Errorf, int64(5), offset)
}
