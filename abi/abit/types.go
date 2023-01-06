// Types for ABI encoding / decoding
package abit

import "strings"

type kind byte

const (
	S kind = iota //static
	D             //dynamic
	T             //tuple
	L             //list
)

func Resolve(desc string, fields ...Type) Type {
	if strings.HasSuffix(desc, "[]") {
		return List(Resolve(strings.TrimSuffix(desc, "[]"), fields...))
	}
	switch desc {
	case "address":
		return Address
	case "bool":
		return Bool
	case "bytes":
		return Bytes
	case "bytes32":
		return Bytes32
	case "tuple":
		return Tuple(fields...)
	case "uint8":
		return Uint8
	case "uint64":
		return Uint64
	case "uint256":
		return Uint256
	default:
		return Type{}
	}
}

type Type struct {
	Kind kind
	name string

	Fields []*Type //For Tuple
	Elem   *Type   //For List
}

// Returns the name of the type in a format ready for ABI signatures
// This is achieved by calling abit.Name on tuple abit.Fields and
// tuple abit.Elem for tuples and arrays respectively.
//
// For example:
// tuple(uint8, address) becomes (uint8, address)
// tuple(uint8, address[] becomes (uint8, address)[]
func (t Type) Name() string {
	switch t.Kind {
	case L:
		return t.Elem.Name() + "[]"
	case T:
		var s strings.Builder
		s.WriteString("(")
		for i, f := range t.Fields {
			s.WriteString(f.Name())
			if i+1 < len(t.Fields) {
				s.WriteString(",")
			}
		}
		s.WriteString(")")
		return s.String()
	default:
		return t.name
	}
}

var (
	Address = Type{
		name: "address",
		Kind: S,
	}
	Bool = Type{
		name: "bool",
		Kind: S,
	}
	Bytes = Type{
		name: "bytes",
		Kind: D,
	}
	Bytes32 = Type{
		name: "bytes32",
		Kind: S,
	}
	String = Type{
		name: "string",
		Kind: D,
	}
	Uint8 = Type{
		name: "uint8",
		Kind: S,
	}
	Uint64 = Type{
		name: "uint64",
		Kind: S,
	}
	Uint256 = Type{
		name: "uint256",
		Kind: S,
	}
)

func List(et Type) Type {
	return Type{Kind: L, Elem: &et}
}

func Tuple(types ...Type) Type {
	t := Type{Kind: T}
	for i := range types {
		t.Fields = append(t.Fields, &types[i])
	}
	return t
}
