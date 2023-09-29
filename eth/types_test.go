package eth

import (
	"encoding/json"
	"testing"

	"kr.dev/diff"
)

func TestDecode(t *testing.T) {
	cases := []struct {
		input string
		want  uint64
		err   error
	}{
		{
			input: "2a",
			want:  42,
			err:   nil,
		},
		{
			input: "1167e6e", //odd length
			want:  18251374,
			err:   nil,
		},
	}
	for _, tc := range cases {
		got, err := decode(tc.input)
		diff.Test(t, t.Fatalf, tc.err, err)
		diff.Test(t, t.Errorf, tc.want, got)
	}
}

func TestGet(t *testing.T) {
	cases := []struct {
		input []int
		i     int
		after []int
	}{
		{
			input: []int{},
			i:     0,
			after: []int{0},
		},
		{
			input: []int{},
			i:     2,
			after: []int{0, 0, 0},
		},
		{
			input: []int{0, 1, 2},
			i:     2,
			after: []int{0, 1, 2},
		},
		{
			input: []int{0, 1, 2},
			i:     4,
			after: []int{0, 1, 2, 0, 0},
		},
	}
	for _, tc := range cases {
		after, _ := get(tc.input, tc.i)
		diff.Test(t, t.Errorf, tc.after, after)
	}
}

func TestGet_Make(t *testing.T) {
	type item struct{ d byte }
	var x = []item{}
	x, it := get(x, 0)
	it.d = 0x2a
	diff.Test(t, t.Errorf, byte(0x2a), x[0].d)

	x, it = get(x, 43)
	it.d = 0x2c
	diff.Test(t, t.Errorf, byte(0x2c), x[43].d)
}

func TestBytes(t *testing.T) {
	cases := []struct {
		input string
		want  Bytes
		err   error
	}{
		{
			input: `{"D": "0x2a"}`,
			want:  []byte{0x2a},
			err:   nil,
		},
	}
	for _, tc := range cases {
		var item = struct{ D Bytes }{}
		err := json.Unmarshal([]byte(tc.input), &item)
		diff.Test(t, t.Fatalf, tc.err, err)
		diff.Test(t, t.Errorf, tc.want, item.D)
	}
}

func TestBytes_Write(t *testing.T) {
	var x Bytes
	diff.Test(t, t.Errorf, 0, len(x))
	diff.Test(t, t.Errorf, 0, cap(x))

	x.Write(make([]byte, 32))
	diff.Test(t, t.Errorf, 32, len(x))
	diff.Test(t, t.Errorf, 32, cap(x))

	x.Write(make([]byte, 16))
	diff.Test(t, t.Errorf, 16, len(x))
	diff.Test(t, t.Errorf, 32, cap(x))
}
