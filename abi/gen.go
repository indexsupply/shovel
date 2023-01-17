package abi

import (
	"bytes"
	"encoding/json"
	"go/format"
	"text/template"
	"unicode"
	"unicode/utf8"

	"github.com/indexsupply/x/isxerrors"
)

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
	t.Funcs(template.FuncMap{
		"export": func(name string) string {
			ru, n := utf8.DecodeRuneInString(name)
			return string(unicode.ToUpper(ru)) + name[n:]
		},
	})
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

import "github.com/indexsupply/x/abi"
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

var {{ export .Name }}Event = abi.Event{
    Name: "{{ .Name }}",
    Inputs: []abi.Input{
        {{ range .Inputs -}}
        {{ template "x" . }}
        {{ end }}
    },
}

type {{ export .Name }} struct {
    it *abi.Item
}

{{ define "y" -}}
{{ range $inp := .Inputs -}}
{{ if eq .Type "tuple" }}
type {{export .Name }} struct {
    it *abi.Item
}
{{ template "y" $inp -}}
{{ end -}}
{{ end -}}
{{ end -}}
{{ template "y" . -}}

{{- define "z" -}}
{{ $event := . -}}
{{ $en := export $event.Name }}
{{ range $index, $inp := .Inputs -}}
{{ $in := export $inp.Name }}
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
{{ else if eq $inp.Type "int" -}}
func (x *{{ $en }}){{ $in }}() int64 {
    return x.it.At({{ $index }}).Int64()
}
{{ else if eq $inp.Type "uint256" -}}
func (x *{{ $en }}){{ $in }}() *big.Int {
    return x.it.At({{ $index }}).BigInt()
}
{{ end -}}
{{ end -}}
{{ end -}}
{{ template "z" . -}}
`
