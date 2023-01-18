package abi

import (
	"bytes"
	"encoding/json"
	"go/format"
	"text/template"
	"unicode"

	"github.com/indexsupply/x/isxerrors"
)

// convert snake case to camel case
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
	err := json.NewDecoder(bytes.NewReader(js)).Decode(&events)
	if err != nil {
		return nil, isxerrors.Errorf("parsing abi json: %w", err)
	}
	var (
		b bytes.Buffer
		t *template.Template
	)

	t = template.New("header")
	t, err = t.Parse(header)
	if err != nil {
		return nil, isxerrors.Errorf("parsing header template: %w", err)
	}
	err = t.Execute(&b, struct {
		Pkg string
	}{
		Pkg: pkg,
	})
	if err != nil {
		return nil, isxerrors.Errorf("executing header template: %w", err)
	}

	t = template.New("body")
	t.Funcs(template.FuncMap{"camel": camel})
	t, err = t.Parse(body)
	if err != nil {
		return nil, isxerrors.Errorf("parsing body template: %w", err)
	}

	for _, event := range events {
		if event.Type != "event" {
			continue
		}
		err = t.Execute(&b, event)
		if err != nil {
			return nil, isxerrors.Errorf("executing body template: %w", err)
		}
	}
	code, err := format.Source(b.Bytes())
	if err != nil {
		return nil, isxerrors.Errorf("formatting source: %w", err)
	}
	return code, nil
}

const header = `
package {{ .Pkg }}

import (
	"math/big"

	"github.com/indexsupply/x/abi"
)
`

const body = `
{{- define "x" -}}
abi.Input{
    Name: "{{.Name}}",
    Type: "{{.Type}}",
    {{ if eq .Type "tuple" -}}
    Inputs: []abi.Input{
    {{ range .Inputs -}}
    {{ template "x" . }}
    {{ end }}
    },
    {{ end }}
},
{{- end -}}

var {{ camel .Name }}Event = abi.Event{
    Name: "{{ .Name }}",
    Inputs: []abi.Input{
        {{ range .Inputs -}}
        {{ template "x" . }}
        {{ end }}
    },
}

type {{ camel .Name }} struct {
    it *abi.Item
}

{{ define "y" -}}
{{ range $inp := .Inputs -}}
{{ if eq .Type "tuple" }}
type {{camel .Name }} struct {
    it *abi.Item
}
{{ template "y" $inp -}}
{{ end -}}
{{ end -}}
{{ end -}}
{{ template "y" . -}}

{{- define "z" -}}
{{ $event := . -}}
{{ $en := camel $event.Name }}
{{ range $index, $inp := .Inputs -}}
{{ $in := camel $inp.Name }}
{{ if eq $inp.Type "tuple" -}}
func (x *{{ $en }}){{ $in }}() *{{ $in }} {
	i := x.it.At({{ $index }})
    return &{{ $in }}{&i}
}
{{ template "z" $inp -}}
{{ else if eq $inp.Type "address" -}}
func (x *{{ $en }}){{ $in }}() [20]byte {
    return x.it.At({{ $index }}).Address()
}
{{ else if eq $inp.Type "address[]" -}}
func (x *{{ $en }}){{ $in }}() [][20]byte {
	it := x.it.At({{ $index }})
	res := make([][20]byte, it.Len())
	for i, v := range it.List() {
		res[i] = v.Address()
	}
    return res
}
{{ else if eq $inp.Type "bool" -}}
func (x *{{ $en }}){{ $in }}() bool {
    return x.it.At({{ $index }}).Bool()
}
{{ else if eq $inp.Type "bool[]" -}}
func (x *{{ $en }}){{ $in }}() []bool {
	it := x.it.At({{ $index }})
	res := make([]bool, it.Len())
	for i, v := range it.List() {
		res[i] = v.Bool()
	}
    return res
}
{{ else if eq $inp.Type "bytes" -}}
func (x *{{ $en }}){{ $in }}() []byte{
    return x.it.At({{ $index }}).Bytes()
}
{{ else if eq $inp.Type "bytes[]" -}}
func (x *{{ $en }}){{ $in }}() [][]byte {
	it := x.it.At({{ $index }})
	res := make([][]byte, it.Len())
	for i, v := range it.List() {
		res[i] = v.Bytes()
	}
    return res
}
{{ else if eq $inp.Type "int" -}}
func (x *{{ $en }}){{ $in }}() int64 {
    return x.it.At({{ $index }}).Int64()
}
{{ else if eq $inp.Type "string" -}}
func (x *{{ $en }}){{ $in }}() string {
    return x.it.At({{ $index }}).String()
}
{{ else if eq $inp.Type "string[]" -}}
func (x *{{ $en }}){{ $in }}() []string {
	it := x.it.At({{ $index }})
	res := make([]string, it.Len())
	for i, v := range it.List() {
		res[i] = v.String()
	}
    return res
}
{{ else if eq $inp.Type "uint256" -}}
func (x *{{ $en }}){{ $in }}() *big.Int {
    return x.it.At({{ $index }}).BigInt()
}
{{ else if eq $inp.Type "uint256[]" -}}
func (x *{{ $en }}){{ $in }}() []*big.Int {
	it := x.it.At({{ $index }})
	res := make([]*big.Int, it.Len())
	for i, v := range it.List() {
		res[i] = v.BigInt()
	}
    return res
}
{{ else if eq $inp.Type "uint8" -}}
func (x *{{ $en }}){{ $in }}() uint8 {
    return x.it.At({{ $index }}).Uint8()
}
{{ else if eq $inp.Type "uint8[]" -}}
func (x *{{ $en }}){{ $in }}() []uint8 {
	it := x.it.At({{ $index }})
	res := make([]uint8, it.Len())
	for i, v := range it.List() {
		res[i] = v.Uint8()
	}
    return res
}
{{ else if eq $inp.Type "uint64" -}}
func (x *{{ $en }}){{ $in }}() uint64 {
    return x.it.At({{ $index }}).Uint64()
}
{{ else if eq $inp.Type "uint64[]" -}}
func (x *{{ $en }}){{ $in }}() []uint64 {
	it := x.it.At({{ $index }})
	res := make([]uint64, it.Len())
	for i, v := range it.List() {
		res[i] = v.Uint64()
	}
    return res
}
{{ end -}}
{{ end -}}
{{ end -}}
{{ template "z" . -}}
`
