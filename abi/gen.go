package abi

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"go/format"
	"strconv"
	"strings"
	"text/template"
	"unicode"

	"github.com/indexsupply/x/abi/abit"
	"github.com/indexsupply/x/isxerrors"
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
	return imports
}

func templateType(inputs []Input) map[string]struct{} {
	var types = map[string]struct{}{}
	for _, input := range inputs {
		if len(input.Components) == 0 {
			types[input.ABIType().TemplateType] = struct{}{}
			continue
		}
		for t := range templateType(input.Components) {
			types[t] = struct{}{}
		}
	}
	return types
}

func returnType(t abit.Type, input Input) string {
	switch t.Kind {
	case abit.S, abit.D:
		return t.TemplateType
	case abit.T:
		return camel(input.Name)
	case abit.L:
		var s strings.Builder
		for _, e := range t.Elems() {
			s.WriteString("[")
			if e.Length > 0 {
				s.WriteString(strconv.Itoa(int(e.Length)))
			}
			s.WriteString("]")
		}
		switch r := t.Root(); r.Kind {
		case abit.T:
			s.WriteString(camel(input.Name))
		default:
			s.WriteString(r.TemplateType)
		}
		return s.String()
	default:
		panic("returnType must have kind: S,D,T, or L")
	}
}

func newType(input Input) string {
	switch t := input.ABIType(); t.Kind {
	case abit.S, abit.D:
		return t.TemplateFunc
	case abit.T:
		return camel(input.Name)
	case abit.L:
		if t.Root().Kind == abit.T {
			return camel(input.Name)
		}
		return t.Root().TemplateFunc
	default:
		panic("newType must have kind: S,D,T, or L")
	}
}

type nctx struct {
	Input Input
	Type  abit.Type
	Index int
}

func (n nctx) New() string {
	return newType(n.Input)
}

func (n nctx) ReturnType() string {
	return returnType(n.Type, n.Input)
}

func (n nctx) Next() *nctx {
	if n.Type.Elem == nil {
		return nil
	}
	if n.Type.Elem.Kind != abit.L {
		return nil
	}
	return &nctx{
		Input: n.Input,
		Type:  *n.Type.Elem,
		Index: n.Index + 1,
	}
}

// used to aggregate template data in template.txt
type getter struct {
	receiver string
	Input    Input
	Index    int
}

func (g getter) Nest() nctx {
	return nctx{
		Input: g.Input,
		Type:  *g.Input.ABIType().Elem,
		Index: 1,
	}
}

func (g getter) Receiver() string {
	return camel(g.receiver)
}

func (g getter) Method() string {
	return camel(g.Input.Name)
}

func (g getter) New() string {
	return newType(g.Input)
}

func (g getter) ReturnType() string {
	return returnType(g.Input.ABIType(), g.Input)
}

func (g getter) Type() abit.Type {
	return g.Input.ABIType()
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
	events := []Event{}
	err := json.Unmarshal(js, &events)
	if err != nil {
		return nil, isxerrors.Errorf("parsing abi json: %w", err)
	}

	t := template.New("abi").Funcs(template.FuncMap{
		"camel": camel,
		"getter": func(i int, s string, it Input) getter {
			return getter{
				Index:    i,
				receiver: s,
				Input:    it,
			}
		},
		"sub": func(x, y int) int {
			return x - y
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
