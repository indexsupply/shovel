package at

type kind byte

const (
	S kind = iota
	D
	T
	L
)

type Type struct {
	Name string
	Kind kind

	Fields      []*Type //For Tuple
	ElementType *Type   //For List
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
	Uint64 = Type{
		Name: "uint64",
		Kind: S,
	}
	String = Type{
		Name: "string",
		Kind: D,
	}
	Uint256 = Type{
		Name: "uint256",
		Kind: S,
	}
)

func List(et Type) Type {
	return Type{
		ElementType: &et,
		Name:        "list",
		Kind:        L,
	}
}

func Tuple(types ...Type) Type {
	t := Type{Name: "tuple", Kind: T}
	for i := range types {
		t.Fields = append(t.Fields, &types[i])
	}
	return t
}
