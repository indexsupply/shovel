package abi

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"go/format"
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

type nestedGetter struct {
	g     getter
	Type  abit.Type
	Index int
}

func newNestedGetter(g getter) nestedGetter {
	return nestedGetter{
		g:     g,
		Type:  *g.Input.ABIType().Elem,
		Index: 1,
	}
}

func (ng nestedGetter) Next() *nestedGetter {
	if ng.Type.Elem == nil {
		return nil
	}
	if ng.Type.Elem.Kind != abit.L {
		return nil
	}
	return &nestedGetter{
		g:     ng.g,
		Type:  *ng.Type.Elem,
		Index: ng.Index + 1,
	}
}

// used to aggregate template data in template.txt
type getter struct {
	Event Event
	Input Input
	Index int
}

func (g getter) Struct() string {
	return camel(g.Event.Name)
}

func (g getter) Method() string {
	return camel(g.Input.Name)
}

func (g getter) Type() abit.Type {
	return g.Input.ABIType()
}

func newGetter(i int, event Event, input Input) getter {
	return getter{
		Index: i,
		Event: event,
		Input: input,
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
		"camel":   camel,
		"getter":  newGetter,
		"ngetter": newNestedGetter,
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
		fmt.Printf("%s\n", b.Bytes())
		return nil, isxerrors.Errorf("formatting source: %w", err)
	}
	return code, nil
}
