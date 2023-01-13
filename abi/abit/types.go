// Types for ABI encoding / decoding
package abit

import (
	"strconv"
	"strings"
)

type kind byte

const (
	S kind = iota //static
	D             //dynamic
	T             //tuple
	L             //list
)

func Resolve(desc string, fields ...Type) Type {
	a := strings.LastIndex(desc, "[")
	b := strings.LastIndex(desc, "]")
	// list case
	if a > 0 && b > 0 {
		sizestr := desc[a+1 : b]
		if sizestr == "" {
			return List(Resolve(desc[:a], fields...))
		}
		size, _ := strconv.ParseUint(sizestr, 10, 32)
		return ListK(Resolve(desc[:a], fields...), uint(size))
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
	case "string":
		return String
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
	Name string

	Fields   []*Type //For Tuple
	Elem     *Type   //For List
	ListSize uint    // 0 for dynamic List, otherwise the size of a fixed-length list
}

// Returns signature of the type including it's Elem and Fields
// For example:
//
//	A size 8 list of address becomes 'address[8]'
//	tuple(uint8, address) becomes (uint8, address)
//	tuple(uint8, address)[] becomes (uint8, address)[]
func (t Type) Signature() string {
	switch t.Kind {
	case L:
		var s strings.Builder
		s.WriteString(t.Elem.Signature())
		s.WriteString("[")
		if t.ListSize > 0 {
			s.WriteString(strconv.Itoa(int(t.ListSize)))
		}
		s.WriteString("]")

		return s.String()
	case T:
		var s strings.Builder
		s.WriteString("(")
		for i, f := range t.Fields {
			s.WriteString(f.Signature())
			if i+1 < len(t.Fields) {
				s.WriteString(",")
			}
		}
		s.WriteString(")")
		return s.String()
	default:
		return t.Name
	}
}

var (
	Address = Type{
		Name: "address",
		Kind: S,
	}
	Bool = Type{
		Name: "bool",
		Kind: S,
	}
	Bytes = Type{
		Name: "bytes",
		Kind: D,
	}
	Bytes32 = Type{
		Name: "bytes32",
		Kind: S,
	}
	String = Type{
		Name: "string",
		Kind: D,
	}
	Uint8 = Type{
		Name: "uint8",
		Kind: S,
	}
	Uint64 = Type{
		Name: "uint64",
		Kind: S,
	}
	Uint256 = Type{
		Name: "uint256",
		Kind: S,
	}
)

func List(et Type) Type {
	t := Type{
		Name: et.Signature(),
		Kind: L,
		Elem: &et,
	}
	t.Name = t.Signature()
	return t
}

// Fixed list of length k
func ListK(et Type, size uint) Type {
	t := Type{
		Kind:     L,
		Elem:     &et,
		ListSize: size,
	}
	t.Name = t.Signature()
	return t
}

func Tuple(types ...Type) Type {
	t := Type{Name: "tuple", Kind: T}
	for i := range types {
		t.Fields = append(t.Fields, &types[i])
	}
	return t
}
