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

type nctx struct {
	getter
	Type  abit.Type
	Index int
}

func nestCtx(g getter) nctx {
	return nctx{
		getter: g,
		Type:   *g.Input.ABIType().Elem,
		Index:  1,
	}
}

func (n nctx) ReturnType() string {
	switch t := n.Type; t.Kind {
	case abit.L:
		var s strings.Builder
		elems := t.Elems()
		root, elems := elems[len(elems)-1], elems[:len(elems)-1]
		for _, e := range elems {
			s.WriteString("[")
			if e.Length > 0 {
				s.WriteString(strconv.Itoa(int(e.Length)))
			}
			s.WriteString("]")
		}
		switch root.Kind {
		case abit.T:
			s.WriteString(camel(n.Input.Name))
		default:
			s.WriteString(root.TemplateType)
		}
		return s.String()
	case abit.T:
		return camel(n.Input.Name)
	default:
		return t.TemplateType
	}
}

func (n nctx) Next() *nctx {
	if n.Type.Elem == nil {
		return nil
	}
	if n.Type.Elem.Kind != abit.L {
		return nil
	}
	return &nctx{
		getter: n.getter,
		Type:   *n.Type.Elem,
		Index:  n.Index + 1,
	}
}

// used to aggregate template data in template.txt
type getter struct {
	receiver string
	Input    Input
	Index    int
}

func (g getter) Receiver() string {
	return camel(g.receiver)
}

func (g getter) Method() string {
	return camel(g.Input.Name)
}

func (g getter) ReturnType() string {
	switch t := g.Input.ABIType(); t.Kind {
	case abit.L:
		var s strings.Builder
		elems := t.Elems()
		root, elems := elems[len(elems)-1], elems[:len(elems)-1]
		for _, e := range elems {
			s.WriteString("[")
			if e.Length > 0 {
				s.WriteString(strconv.Itoa(int(e.Length)))
			}
			s.WriteString("]")
		}
		switch root.Kind {
		case abit.T:
			s.WriteString(camel(g.Input.Name))
		default:
			s.WriteString(root.TemplateType)
		}
		return s.String()
	case abit.T:
		return camel(g.Input.Name)
	default:
		return t.TemplateType
	}
}

func (g getter) New() string {
	switch t := g.Input.ABIType(); t.Kind {
	case abit.T:
		return g.Input.Name
	case abit.L:
		switch r := t.Root(); r.Kind {
		case abit.T:
			return camel(g.Input.Name)
		default:
			return r.TemplateFunc
		}
	default:
		return t.TemplateFunc
	}
}

func (g getter) Type() abit.Type {
	return g.Input.ABIType()
}

func newGetter(i int, s string, it Input) getter {
	return getter{
		Index:    i,
		receiver: s,
		Input:    it,
	}
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
		"camel":  camel,
		"getter": newGetter,
		"nctx":   nestCtx,
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
