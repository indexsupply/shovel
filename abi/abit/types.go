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
		return ListK(Resolve(desc[:a], fields...), uint(k))
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

	// The abi package uses these fields
	// for code gen.
	TemplateType string
	TemplateFunc string

	Fields []*Type //For Tuple
	Elem   *Type   //For List
	Length uint    //0 for dynamic List, otherwise the size of a fixed-length list
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

func (t Type) Elems() []Type {
	var res []Type
	for e := &t; e != nil; e = e.Elem {
		res = append(res, *e)
	}
	return res
}

func (t Type) Dimension() int {
	var d int
	for e := &t; e.Elem != nil; e = e.Elem {
		d++
	}
	return d
}

func (t Type) Root() Type {
	elems := t.Elems()
	if len(elems) == 0 {
		return Type{}
	}
	return elems[len(elems)-1]
}

var (
	Address = Type{
		Name:         "address",
		Kind:         S,
		TemplateFunc: "Address",
		TemplateType: "[20]byte",
	}
	Bool = Type{
		Name:         "bool",
		Kind:         S,
		TemplateFunc: "Bool",
		TemplateType: "bool",
	}
	Bytes = Type{
		Name:         "bytes",
		Kind:         D,
		TemplateFunc: "Bytes",
		TemplateType: "[]byte",
	}
	Bytes32 = Type{
		Name:         "bytes32",
		Kind:         S,
		TemplateFunc: "Bytes32",
		TemplateType: "[32]byte",
	}
	String = Type{
		Name:         "string",
		Kind:         D,
		TemplateFunc: "String",
		TemplateType: "string",
	}
	Uint8 = Type{
		Name:         "uint8",
		Kind:         S,
		TemplateFunc: "Uint8",
		TemplateType: "uint8",
	}
	Uint64 = Type{
		Name:         "uint64",
		Kind:         S,
		TemplateFunc: "Uint64",
		TemplateType: "uint64",
	}
	Uint256 = Type{
		Name:         "uint256",
		Kind:         S,
		TemplateFunc: "BigInt",
		TemplateType: "*big.Int",
	}
)

func List(et Type) Type {
	return Type{
		Name: "list",
		Kind: L,
		Elem: &et,
	}
}

// Fixed list of length k
func ListK(et Type, k uint) Type {
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
