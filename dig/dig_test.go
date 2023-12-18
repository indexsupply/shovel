package dig

import (
	"encoding/hex"
	"strings"
	"testing"

	"github.com/indexsupply/x/bint"

	"kr.dev/diff"
)

func TestHasStatic(t *testing.T) {
	cases := []struct {
		t    atype
		want bool
	}{
		{
			static(),
			true,
		},
		{
			dynamic(),
			false,
		},
		{
			array(static()),
			false,
		},
		{
			array(dynamic()),
			false,
		},
		{
			arrayK(2, static()),
			true,
		},
		{
			arrayK(3, arrayK(2, static())),
			true,
		},
	}
	for _, tc := range cases {
		got := hasStatic(tc.t)
		diff.Test(t, t.Errorf, got, tc.want)
	}
}
func TestSizeof(t *testing.T) {
	cases := []struct {
		t    atype
		want int
	}{
		{
			static(),
			32,
		},
		{
			dynamic(),
			0,
		},
		{
			array(static()),
			0,
		},
		{
			array(dynamic()),
			0,
		},
		{
			arrayK(2, static()),
			64,
		},
		{
			arrayK(3, arrayK(2, static())),
			192,
		},
	}
	for _, tc := range cases {
		got := sizeof(tc.t)
		diff.Test(t, t.Errorf, got, tc.want)
	}
}

func TestHasKind(t *testing.T) {
	cases := []struct {
		t    atype
		k    byte
		want bool
	}{
		{
			static(),
			's',
			false,
		},
		{
			dynamic(),
			's',
			false,
		},
		{
			array(dynamic()),
			'd',
			true,
		},
		{
			array(array(dynamic())),
			'd',
			true,
		},
		{
			array(array(dynamic())),
			'a',
			true,
		},
		{
			tuple(array(dynamic())),
			's',
			false,
		},
	}
	for _, tc := range cases {
		got := tc.t.hasKind(tc.k)
		diff.Test(t, t.Errorf, got, tc.want)
	}
}

func hb(s string) []byte {
	s = strings.Map(func(r rune) rune {
		switch {
		case r >= '0' && r <= '9':
			return r
		case r >= 'a' && r <= 'f':
			return r
		default:
			return -1
		}
	}, strings.ToLower(s))
	b, _ := hex.DecodeString(s)
	return b
}

func n2b(x uint64) []byte {
	var b [32]byte
	bint.Encode(b[:], x)
	return b[:]
}

func TestScan(t *testing.T) {
	cases := []struct {
		desc  string
		input []byte
		at    atype
		want  [][][]byte
	}{
		{
			desc: "tuple of numbers",
			input: hb(`
				000000000000000000000000000000000000000000000000000000000000002a
				000000000000000000000000000000000000000000000000000000000000002a
			`),
			at: tuple(sel(0, static()), sel(1, static())),
			want: [][][]byte{
				[][]byte{n2b(42), n2b(42)},
			},
		},
		{
			desc: "tuple of array with static types",
			input: hb(`
				000000000000000000000000000000000000000000000000000000000000002a
				0000000000000000000000000000000000000000000000000000000000000040
				0000000000000000000000000000000000000000000000000000000000000002
				000000000000000000000000000000000000000000000000000000000000002b
				000000000000000000000000000000000000000000000000000000000000002c

			`),
			at: tuple(sel(0, static()), array(sel(1, static()))),
			want: [][][]byte{
				[][]byte{n2b(42), n2b(43)},
				[][]byte{n2b(42), n2b(44)},
			},
		},
		{
			desc: "tuple of array with dynamic types",
			input: hb(`
				000000000000000000000000000000000000000000000000000000000000002a
				0000000000000000000000000000000000000000000000000000000000000040
				0000000000000000000000000000000000000000000000000000000000000002
				0000000000000000000000000000000000000000000000000000000000000040
				0000000000000000000000000000000000000000000000000000000000000080
				0000000000000000000000000000000000000000000000000000000000000003
				666f6f0000000000000000000000000000000000000000000000000000000000
				0000000000000000000000000000000000000000000000000000000000000003
				6261720000000000000000000000000000000000000000000000000000000000
			`),
			at: tuple(sel(0, static()), array(sel(1, dynamic()))),
			want: [][][]byte{
				[][]byte{n2b(42), []byte("foo")},
				[][]byte{n2b(42), []byte("bar")},
			},
		},
		{
			desc: "dynamic nested list of dynamic types",
			input: hb(`
				0000000000000000000000000000000000000000000000000000000000000002
				0000000000000000000000000000000000000000000000000000000000000040
				0000000000000000000000000000000000000000000000000000000000000120
				0000000000000000000000000000000000000000000000000000000000000002
				0000000000000000000000000000000000000000000000000000000000000040
				0000000000000000000000000000000000000000000000000000000000000080
				0000000000000000000000000000000000000000000000000000000000000005
				68656c6c6f000000000000000000000000000000000000000000000000000000
				0000000000000000000000000000000000000000000000000000000000000005
				776f726c64000000000000000000000000000000000000000000000000000000
				0000000000000000000000000000000000000000000000000000000000000001
				0000000000000000000000000000000000000000000000000000000000000020
				0000000000000000000000000000000000000000000000000000000000000003
				6279650000000000000000000000000000000000000000000000000000000000
			`),
			at: array(array(sel(0, dynamic()))),
			want: [][][]byte{
				[][]byte{[]byte("hello")},
				[][]byte{[]byte("world")},
				[][]byte{[]byte("bye")},
			},
		},
	}
	for _, tc := range cases {
		res := NewResult(tc.at)
		err := res.Scan(tc.input)
		diff.Test(t, t.Fatalf, err, nil)
		diff.Test(t, t.Errorf, res.Bytes(), tc.want)
	}
}

func TestABIType(t *testing.T) {
	cases := []struct {
		input Input
		want  atype
		n     int
	}{
		{
			Input{Name: "a", Type: "bytes32"},
			static(),
			0,
		},
		{
			Input{Name: "a", Type: "bytes32[]"},
			array(static()),
			0,
		},
		{
			Input{
				Name: "a",
				Type: "tuple[]",
				Components: []Input{
					Input{Name: "b", Type: "uint256"},
					Input{Name: "c", Type: "bytes"},
				},
			},
			array(tuple(static(), dynamic())),
			0,
		},
	}
	for _, tc := range cases {
		_, got := tc.input.ABIType(0)
		diff.Test(t, t.Errorf, tc.want, got)
	}
}

func TestSelected(t *testing.T) {
	event := Event{
		Name: "test",
		Inputs: []Input{
			Input{Name: "z"},
			Input{
				Name:   "a",
				Column: "a",
				Components: []Input{
					Input{Name: "b", Column: "b"},
					Input{Name: "c", Column: "c"},
				},
			},
			Input{Name: "d", Column: "d"},
			Input{Name: "e", Column: ""},
		},
	}
	want := []Input{
		Input{Name: "b", Column: "b"},
		Input{Name: "c", Column: "c"},
		Input{
			Name:   "a",
			Column: "a",
			Components: []Input{
				Input{Name: "b", Column: "b"},
				Input{Name: "c", Column: "c"},
			},
		},
		Input{Name: "d", Column: "d"},
	}
	diff.Test(t, t.Errorf, want, event.Selected())
}

func TestNumIndexed(t *testing.T) {
	event := Event{
		Name: "",
		Inputs: []Input{
			Input{Indexed: true, Name: "a"},
			Input{Indexed: true, Name: "b", Column: "b"},
			Input{Indexed: true, Name: "c", Column: "c"},
		},
	}
	diff.Test(t, t.Errorf, 3, event.numIndexed())
}
