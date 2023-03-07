package bloom

import (
	"crypto/rand"
	"encoding/hex"
	"testing"
)

func BenchmarkFilterAdd(b *testing.B) {
	b.Skip()
	input := make([][]byte, 1000)
	for i := 0; i < 1000; i++ {
		var d [32]byte
		_, err := rand.Read(d[:])
		if err != nil {
			b.Fatal(err)
		}
		input[i] = d[:]
	}
	b.ReportAllocs()
	b.ResetTimer()
	var bf Filter
	for i := 0; i < b.N; i++ {
		for j := 0; j < 1000; j++ {
			bf.Add(input[j])
		}
	}
}

func BenchmarkFilterMissing(b *testing.B) {
	var (
		bf    Filter
		input [][32]byte
	)
	for i := 0; i < 1000; i++ {
		var d [32]byte
		_, err := rand.Read(d[:])
		if err != nil {
			b.Fatal(err)
		}
		bf.Add(d[:])
		input = append(input, d)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for j := 0; j < len(input); j++ {
			bf.Missing(input[j][:])
		}
	}
}

func TestFilter(t *testing.T) {
	var data [32]byte
	_, err := rand.Read(data[:])
	if err != nil {
		t.Fatal(err)
	}
	var bf Filter
	bf.Add(data[:])
	if bf.Missing(data[:]) {
		t.Errorf("expected data to exist in bloom filter")
	}
}

func TestExistsWithEvent(t *testing.T) {
	bb, _ := hex.DecodeString("2d209418e80821025f0850088d4d4aae8131728a4025ddb00cdd04a4d4542804101ad388908140ab1449d2182f04554c1370256a8f2b2f6906778941542e201c0cf960f04709a46ead0a9c6b42366bb289e910a8d94d3224164c1c30c81b2b85171004004e4214660046720caa6968d1cd06283b401416435a0404d4b85ac8844700a19a76e2add9a05fb4c40210028b22006f9bc5246409243801c2283434c8db0061d37c28714b22a06f840c10040e724016263d1b214240444ea205c453d04836d103e6710272802c4840440c50461be906402b0c4b54dc1c99aa3080e202d130b32a2d542000c580b74a76c15d8005b2322970cc0e41b1292a34480970a1")
	sb, _ := hex.DecodeString("b8e138887d0aa13bab447e82de9d5c1777041ecd21ca36ba824ff1e6c07ddda4")
	bf := Filter(bb)
	if bf.Missing(sb) {
		t.Errorf("expected data to exist in bloom filter")
	}
}
