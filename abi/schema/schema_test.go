package schema

import (
	"testing"

	"kr.dev/diff"
)

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
			desc:  "array",
			input: "(bytes32,bytes)[]",
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
