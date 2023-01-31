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
	// list case
	var (
		a = strings.LastIndex(desc, "[")
		b = strings.LastIndex(desc, "]")
	)
	if a > 0 && b > 0 {
		lenDesc := desc[a+1 : b]
		if lenDesc == "" {
			return List(Resolve(desc[:a], fields...))
		}
		k, err := strconv.ParseUint(lenDesc, 10, 32)
		if err != nil {
			//TODO(ryan): panic might not be the best thing to do here
			//but there is a larger issue to solve when it comes to
			//errors when parsing logs. See: #61
			panic("list length descriptor is not a number")
		}
		return ListK(uint(k), Resolve(desc[:a], fields...))
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

	// Name is used in building the type's [Signature] and also
	// in debugging. For types of kind L or T, the Name is not used
	// since [Signature] and [Resolve] deal with these kinds of types
	// explicitly.
	Name string

	Fields []*Type //For Tuple
	Elem   *Type   //For List
	Length uint    // 0 for dynamic List, otherwise the size of a fixed-length list
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
		if t.Length > 0 {
			s.WriteString(strconv.Itoa(int(t.Length)))
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

// A type is conidered static if the following conditions are met:
// 1. Fixed size type (eg Uint256 or Bytes32)
// 2. Fixed size list containing fixed size types (eg ListK(1, Uint256))
// 3. Tuple containing only fixed sized types (eg Tuple(Tuple(Uint256)))
// All other types are considered dynamic.
//
// See the Types section of the spec for more discussion:
// https://docs.soliditylang.org/en/v0.8.17/abi-spec.html#types
func (t Type) Static() bool {
	if t.Kind == D {
		return false
	}
	if t.Elem != nil && t.Length > 0 && t.Elem.Kind == D {
		return false
	}
	if t.Elem != nil && t.Length == 0 {
		return false
	}
	for _, f := range t.Fields {
		if !f.Static() {
			return false
		}
	}
	return true
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
	return Type{
		Name: "list",
		Kind: L,
		Elem: &et,
	}
}

// Fixed sized list of length k
func ListK(k uint, et Type) Type {
	return Type{
		Name:   "list",
		Kind:   L,
		Elem:   &et,
		Length: k,
	}
}

func Tuple(types ...Type) Type {
	t := Type{Name: "tuple", Kind: T}
	for i := range types {
		t.Fields = append(t.Fields, &types[i])
	}
	return t
}
