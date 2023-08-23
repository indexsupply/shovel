package schema

import (
	"testing"

	"kr.dev/diff"
)

func TestString(t *testing.T) {
	cases := []struct {
		input Type
		want  string
	}{
		{
			Static(),
			"static",
		},
		{
			Dynamic(),
			"dynamic",
		},
		{
			Array(Static()),
			"[]static",
		},
		{
			Tuple(),
			"tuple()",
		},
		{
			Tuple(Static(), Dynamic()),
			"tuple(static,dynamic)",
		},
		{
			Tuple(Array(Tuple(Array(Static())))),
			"tuple([]tuple([]static))",
		},
		{
			Tuple(Array(Tuple(ArrayK(42, Static())))),
			"tuple([]tuple([42]static))",
		},
	}
	for _, tc := range cases {
		diff.Test(t, t.Errorf, tc.input.String(), tc.want)
	}
}

func TestContains(t *testing.T) {
	cases := []struct {
		desc  string
		t     Type
		input byte
		want  bool
	}{
		{
			"tuple no tuples",
			Tuple(),
			't',
			false,
		},
		{
			"tuple with tuples",
			Tuple(Tuple()),
			't',
			true,
		},
		{
			"tuple no arrays",
			Tuple(),
			'a',
			false,
		},
		{
			"tuple array",
			Tuple(Array(Static())),
			'a',
			true,
		},
		{
			"tuple tuple array",
			Tuple(Static(), Tuple(Array(Static()))),
			'a',
			true,
		},
		{
			"tuple tuple array",
			Tuple(Static(), Tuple(Array(Static()))),
			'd',
			false,
		},
	}
	for _, tc := range cases {
		if tc.t.Contains(tc.input) != tc.want {
			t.Errorf("expected %s.Contains(%s) to be: %v", tc.t, string(tc.input), tc.want)
		}
	}
}

func TestParse(t *testing.T) {
	cases := []struct {
		desc  string
		input string
		want  Type
	}{
		{
			desc:  "single static",
			input: "(uint8)",
			want:  Tuple(Static()),
		},
		{
			desc:  "single static with spaces",
			input: "( uint8 )",
			want:  Tuple(Static()),
		},
		{
			desc:  "single dynamic",
			input: "(bytes)",
			want:  Tuple(Dynamic()),
		},
		{
			desc:  "multiple static",
			input: "(uint8,uint8)",
			want:  Tuple(Static(), Static()),
		},
		{
			desc:  "multiple dynamic",
			input: "(bytes,bytes)",
			want:  Tuple(Dynamic(), Dynamic()),
		},
		{
			desc:  "mixed",
			input: "(bytes32,bytes)",
			want:  Tuple(Static(), Dynamic()),
		},
		{
			desc:  "mixed with spaces",
			input: "( bytes32, bytes )",
			want:  Tuple(Static(), Dynamic()),
		},
		{
			desc:  "fixed size array",
			input: "(bytes32,bytes)[2]",
			want:  ArrayK(2, Tuple(Static(), Dynamic())),
		},
		{
			desc:  "array",
			input: "(bytes32,bytes)[]",
			want:  Array(Tuple(Static(), Dynamic())),
		},
		{
			desc:  "array with spaces",
			input: "( bytes32 , bytes ) []",
			want:  Array(Tuple(Static(), Dynamic())),
		},
		{
			desc:  "nested array",
			input: "(bytes32,bytes)[][][]",
			want:  Array(Array(Array(Tuple(Static(), Dynamic())))),
		},
		{
			desc:  "nested tuple",
			input: "(bytes32,(bytes32))",
			want:  Tuple(Static(), Tuple(Static())),
		},
		{
			desc:  "nested tuple with array",
			input: "(bytes32,(bytes32[]))",
			want:  Tuple(Static(), Tuple(Array(Static()))),
		},
		{
			desc:  "complex",
			input: "((address,bytes32,bytes,(uint8,uint8))[][])",
			want: Tuple(
				Array(
					Array(
						Tuple(
							Static(),
							Static(),
							Dynamic(),
							Tuple(
								Static(),
								Static(),
							),
						),
					),
				),
			),
		},
	}
	for _, tc := range cases {
		diff.Test(t, t.Errorf, Parse(tc.input), tc.want)
	}
}

func TestTuple(t *testing.T) {
	cases := []struct {
		desc  string
		want  Type
		tuple Type
	}{
		{
			desc:  "zero",
			tuple: Tuple(),
			want:  Type{Kind: 't', Static: true},
		},
		{
			desc:  "static",
			tuple: Tuple(Static()),
			want: Type{
				Kind:   't',
				Static: true,
				Size:   32,
				Fields: []Type{
					Type{
						Kind:   's',
						Static: true,
						Size:   32,
					},
				},
			},
		},
		{
			desc:  "dynamic",
			tuple: Tuple(Dynamic()),
			want: Type{
				Kind:   't',
				Static: false,
				Size:   0,
				Fields: []Type{
					Type{
						Kind:   'd',
						Static: false,
						Size:   0,
					},
				},
			},
		},
		{
			desc:  "mixed",
			tuple: Tuple(Static(), Dynamic()),
			want: Type{
				Kind:   't',
				Static: false,
				Size:   0,
				Fields: []Type{
					Type{
						Kind:   's',
						Static: true,
						Size:   32,
					},
					Type{
						Kind:   'd',
						Static: false,
						Size:   0,
					},
				},
			},
		},
		{
			desc:  "nested static",
			tuple: Tuple(Static(), Tuple(Static())),
			want: Type{
				Kind:   't',
				Static: true,
				Size:   64,
				Fields: []Type{
					Type{
						Kind:   's',
						Static: true,
						Size:   32,
					},
					Type{
						Kind:   't',
						Static: true,
						Size:   32,
						Fields: []Type{
							Type{
								Kind:   's',
								Static: true,
								Size:   32,
							},
						},
					},
				},
			},
		},
		{
			desc:  "nested dynamic",
			tuple: Tuple(Static(), Tuple(Dynamic())),
			want: Type{
				Kind:   't',
				Static: false,
				Size:   0,
				Fields: []Type{
					Type{
						Kind:   's',
						Static: true,
						Size:   32,
					},
					Type{
						Kind:   't',
						Static: false,
						Size:   0,
						Fields: []Type{
							Type{
								Kind:   'd',
								Static: false,
								Size:   0,
							},
						},
					},
				},
			},
		},
	}
	for _, tc := range cases {
		diff.Test(t, t.Errorf, tc.tuple, tc.want)
	}
}

func TestSize(t *testing.T) {
	cases := []struct {
		t    Type
		want int
	}{
		{
			Static(),
			32,
		},
		{
			Dynamic(),
			0,
		},
		{
			Array(Static()),
			0,
		},
		{
			Array(Dynamic()),
			0,
		},
		{
			ArrayK(2, Static()),
			64,
		},
		{
			ArrayK(3, ArrayK(2, Static())),
			192,
		},
	}
	for _, tc := range cases {
		got := tc.t.size()
		diff.Test(t, t.Errorf, got, tc.want)
	}
}
