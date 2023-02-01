package abig

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"go/format"
	"text/template"
	"unicode"

	"github.com/indexsupply/x/abi"
	"github.com/indexsupply/x/abi/abit"
	"github.com/indexsupply/x/isxerrors"
)

//go:embed template.txt
var abitemp string

func imports(events []abi.Event) []string {
	var types = map[string]struct{}{}
	for _, event := range events {
		for t, _ := range templateType(event.Inputs) {
			types[t] = struct{}{}
		}
	}
	var imports []string
	for t, _ := range types {
		switch t {
		case "*big.Int":
			imports = append(imports, "math/big")
		}
	}
	imports = append(imports, "github.com/indexsupply/x/abi")
	return imports
}

func templateType(inputs []abi.Input) map[string]struct{} {
	var types = map[string]struct{}{}
	for _, input := range inputs {
		if len(input.Components) == 0 {
			types[goType(input)] = struct{}{}
			continue
		}
		for t := range templateType(input.Components) {
			types[t] = struct{}{}
		}
	}
	return types
}

func elems(t abit.Type) []abit.Type {
	var res []abit.Type
	for e := &t; e.Elem != nil; e = e.Elem {
		res = append(res, *e)
	}
	return res
}
func dimension(t abit.Type) int {
	return len(elems(t))
}

func root(t abit.Type) abit.Type {
	for e := &t; ; e = e.Elem {
		if e.Elem == nil {
			return *e
		}
	}
}

func dims(inp abi.Input) string {
	var out string
	for i := 0; i < dimension(inp.ABIType()); i++ {
		out += "[]"
	}
	return out
}

func itemFunc(t abit.Type) string {
	switch t.Name {
	case "address":
		return "Address()"
	case "bool":
		return "Bool()"
	case "bytes":
		return "Bytes()"
	case "bytes32":
		return "Bytes32()"
	case "uint8":
		return "Uint8()"
	case "uint64":
		return "Uint64()"
	case "uint120":
		return "BigInt()"
	case "uint256":
		return "BigInt()"
	case "tuple":
		return "NewDetails()"
	case "list":
		return itemFunc(root(t))
	default:
		panic(fmt.Sprintf("unkown type: %s", t.Name))
	}
}

func goType(input abi.Input) string {
	switch input.ABIType().Name {
	case "address":
		return "[20]byte"
	case "bool":
		return "bool"
	case "bytes":
		return "[]byte"
	case "bytes32":
		return "[32]byte"
	case "tuple":
		return camel(input.Name)
	case "list":
		return dims(input) + camel(input.Name)
	case "uint8":
		return "uint8"
	case "uint64":
		return "uint64"
	case "uint120":
		return "*big.Int"
	case "uint256":
		return "*big.Int"
	default:
		panic(fmt.Sprintf("unkown type: %s", input.ABIType().Name))
	}
}

// used to aggregate template data in template.txt
type tinput struct {
	Input abi.Input
	Index int

	nested     bool
	inputIndex int
}

func (t tinput) Type() abit.Type {
	return elems(t.Input.ABIType())[t.Index]
}

func (t tinput) Nested() bool {
	return t.nested
}

func (t tinput) HasNext() bool {
	return (dimension(t.Input.ABIType()) - 1 - t.Index) != 0
}

func (t tinput) ItemIndex() int {
	if t.nested {
		return t.Index
	}
	return t.inputIndex
}

func (t tinput) Next() tinput {
	return tinput{
		nested: true,
		Input:  t.Input,
		Index:  t.Index + 1,
	}
}

func (t tinput) Dim() string {
	var out string
	for i := t.Index; i < dimension(t.Input.ABIType()); i++ {
		out += "[]"
	}
	return out
}

// We generate structs for Event.Inputs and
// Input.Components. Since both Inputs and Components
// are just a []abi.Input, tstruct abstracts Event
// and Input away so that the struct template can
// simply iterate over tstruct.Inputs.
// Name can either be Event.Name or Input.Name.
type tstruct struct {
	Name   string
	Inputs []abi.Input
}

func camel(str string) string {
	var (
		in  = []rune(str)
		res []rune
	)
	for i, r := range in {
		switch {
		case r == '_':
			//skip
		case i == 0 || in[i-1] == '_':
			res = append(res, unicode.ToUpper(r))
		default:
			res = append(res, r)
		}
	}
	return string(res)
}

func Gen(pkg string, js []byte) ([]byte, error) {
	events := []abi.Event{}
	err := json.Unmarshal(js, &events)
	if err != nil {
		return nil, isxerrors.Errorf("parsing abi json: %w", err)
	}

	t := template.New("abi").Funcs(template.FuncMap{
		"camel":    camel,
		"goType":   goType,
		"itemFunc": itemFunc,
		"tinput": func(i int, it abi.Input) tinput {
			return tinput{
				Input:      it,
				inputIndex: i,
			}
		},
		"tstruct": func(name string, inputs []abi.Input) tstruct {
			return tstruct{
				Name:   name,
				Inputs: inputs,
			}
		},
		"sub": func(x, y int) int {
			return x - y
		},
		"add": func(x, y int) int {
			return x + y
		},
	})
	t, err = t.Parse(abitemp)
	if err != nil {
		return nil, isxerrors.Errorf("parsing template: %w", err)
	}

	var b bytes.Buffer
	err = t.ExecuteTemplate(&b, "package", pkg)
	if err != nil {
		return nil, isxerrors.Errorf("executing package template: %w", err)
	}
	err = t.ExecuteTemplate(&b, "imports", imports(events))
	if err != nil {
		return nil, isxerrors.Errorf("executing imports template: %w", err)
	}

	for _, event := range events {
		if event.Type != "event" {
			continue
		}
		err = t.ExecuteTemplate(&b, "event", event)
		if err != nil {
			return nil, isxerrors.Errorf("executing event template: %w", err)
		}
	}
	code, err := format.Source(b.Bytes())
	if err != nil {
		return b.Bytes(), isxerrors.Errorf("formatting source: %w", err)
	}
	return code, nil
}
