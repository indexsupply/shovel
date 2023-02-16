package genabi

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"go/format"
	"strings"
	"text/template"
	"unicode"

	"github.com/indexsupply/x/isxerrors"
	"github.com/indexsupply/x/isxhash"
)

//go:embed template.txt
var abitemp string

func imports(events []Event) []string {
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
	imports = append(imports, "github.com/indexsupply/x/abi/schema")
	return imports
}

func templateType(inputs []Input) map[string]struct{} {
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

func itemFunc(input Input) string {
	switch input.Type {
	case "address":
		return "Address()"
	case "bool":
		return "Bool()"
	case "bytes":
		return "Bytes()"
	case "bytes32":
		return "Bytes32()"
	case "bytes4":
		return "Bytes4()"
	case "uint8":
		return "Uint8()"
	case "uint16":
		return "Uint16()"
	case "uint32":
		return "Uint32()"
	case "uint64":
		return "Uint64()"
	case "uint120", "uint256":
		return "BigInt()"
	default:
		panic(fmt.Sprintf("unkown type: %s", input.Type))
	}
}

func goType(input Input) string {
	if isTuple(input) {
		return camel(input.Name)
	}
	if isArray(input) {
		var t string
		for i := 0; i < dimension(input); i++ {
			t += "[]"
		}
		return t + camel(input.Name)
	}
	switch input.Type {
	case "address":
		return "[20]byte"
	case "bool":
		return "bool"
	case "bytes":
		return "[]byte"
	case "bytes32":
		return "[32]byte"
	case "bytes4":
		return "[4]byte"
	case "uint8":
		return "uint8"
	case "uint64":
		return "uint64"
	case "uint120":
		return "*big.Int"
	case "uint256":
		return "*big.Int"
	default:
		panic(fmt.Sprintf("unkown type: %s", input.Type))
	}
}

func hasTuple(input Input) bool {
	return strings.HasPrefix(input.Type, "tuple")
}

func isTuple(input Input) bool {
	return strings.HasSuffix(input.Type, "tuple")
}

func isArray(input Input) bool {
	return strings.HasSuffix(input.Type, "]")
}

func dimension(input Input) int {
	var n int
	for _, c := range input.Type {
		if c == rune('[') {
			n++
		}
	}
	return n
}

// used to aggregate template data in template.txt
type listHelper struct {
	Input      Input
	Index      int
	Nested     bool
	inputIndex int
}

func (t listHelper) HasNext() bool {
	return (dimension(t.Input) - 1 - t.Index) != 0
}

func (lh listHelper) ItemIndex() int {
	if lh.Nested {
		return lh.Index
	}
	return lh.inputIndex
}

func (lh listHelper) Next() listHelper {
	return listHelper{
		Nested: true,
		Input:  lh.Input,
		Index:  lh.Index + 1,
	}
}

func (lh listHelper) Dimension() string {
	var out string
	for i := lh.Index; i < dimension(lh.Input); i++ {
		out += "[]"
	}
	return out
}

// We generate structs for Event.Inputs and
// Input.Components. Since both Inputs and Components
// are just a []Input, structHelper abstracts Event
// and Input away so that the struct template can
// simply iterate over structHelper.Inputs.
// Name can either be Event.Name or Input.Name.
type structHelper struct {
	Name   string
	Inputs []Input
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

type Event struct {
	Name      string
	Type      string //event
	Anonymous bool
	Inputs    []Input
}

func (e Event) Signature() string {
	var s strings.Builder
	s.WriteString(e.Name)
	s.WriteString("(")
	for i := range e.Inputs {
		s.WriteString(e.Inputs[i].Signature())
		if i+1 < len(e.Inputs) {
			s.WriteString(",")
		}
	}
	s.WriteString(")")
	return s.String()
}

func (e Event) SchemaSignature() string {
	var s strings.Builder
	s.WriteString("(")
	for i, inp := range e.Inputs {
		if inp.Indexed {
			continue
		}
		s.WriteString(inp.Signature())
		if i+1 < len(e.Inputs) {
			s.WriteString(",")
		}
	}
	s.WriteString(")")
	return s.String()
}

func (e Event) SignatureHash() [32]byte {
	return isxhash.Keccak32([]byte(e.Signature()))
}

func (e Event) SigHashLiteral() string {
	b := e.SignatureHash()
	s := "[32]byte{"
	for i := range b {
		s += fmt.Sprintf("0x%x", b[i])
		if i != len(b) {
			s += ", "
		}
	}
	return s + "}"
}

type Input struct {
	Indexed    bool
	Name       string
	Type       string
	Components []Input
}

func (inp Input) Signature() string {
	if !strings.HasPrefix(inp.Type, "tuple") {
		return inp.Type
	}
	var s strings.Builder
	s.WriteString("(")
	for i, c := range inp.Components {
		s.WriteString(c.Signature())
		if i+1 < len(inp.Components) {
			s.WriteString(",")
		}
	}
	s.WriteString(")")
	return strings.Replace(inp.Type, "tuple", s.String(), 1)
}

func Gen(pkg string, js []byte) ([]byte, error) {
	events := []Event{}
	err := json.Unmarshal(js, &events)
	if err != nil {
		return nil, isxerrors.Errorf("parsing abi json: %w", err)
	}

	t := template.New("abi").Funcs(template.FuncMap{
		"camel":    camel,
		"goType":   goType,
		"itemFunc": itemFunc,
		"sub": func(x, y int) int {
			return x - y
		},
		"add": func(x, y int) int {
			return x + y
		},
		"listHelper": func(i int, it Input) listHelper {
			return listHelper{Input: it, inputIndex: i}
		},
		"structHelper": func(name string, inputs []Input) structHelper {
			return structHelper{Name: name, Inputs: inputs}
		},
		"hasTuple": hasTuple,
		"isTuple":  isTuple,
		"isArray":  isArray,
		"unindexed": func(inputs []Input) []Input {
			var res []Input
			for _, inp := range inputs {
				if inp.Indexed {
					continue
				}
				res = append(res, inp)
			}
			return res
		},
		"indexed": func(inputs []Input) []Input {
			var res []Input
			for _, inp := range inputs {
				if !inp.Indexed {
					continue
				}
				res = append(res, inp)
			}
			return res
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
