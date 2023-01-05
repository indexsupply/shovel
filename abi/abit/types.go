// Types for ABI encoding / decoding
package abit

type kind byte

const (
	S kind = iota //static
	D             //dynamic
	T             //tuple
	L             //list
)

type Type struct {
	Name string
	Kind kind

	Fields []*Type //For Tuple
	Elem   *Type   //For List
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
	String = Type{
		Name: "string",
		Kind: D,
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
		Elem: &et,
		Name: et.Name + "[]",
		Kind: L,
	}
}

func Tuple(types ...Type) Type {
	t := Type{Name: "tuple", Kind: T}
	for i := range types {
		t.Fields = append(t.Fields, &types[i])
	}
	return t
}
