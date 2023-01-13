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

func Gen(js []byte) ([]byte, error) {
	events := []Event{}
	err := json.NewDecoder(bytes.NewReader(js)).Decode(&events)
	if err != nil {
		return nil, isxerrors.Errorf("parsing abi json: %w", err)
	}
	var (
		t = template.New("wt")
		b bytes.Buffer
	)
	t.Funcs(template.FuncMap{
		"export": func(name string) string {
			ru, n := utf8.DecodeRuneInString(name)
			return string(unicode.ToUpper(ru)) + name[n:]
		},
	})
	t, err = t.Parse(wrapperTemplate)
	if err != nil {
		return nil, isxerrors.Errorf("parsing template: %w", err)
	}

	for _, event := range events {
		if event.Type != "event" {
			continue
		}
		err = t.Execute(&b, event)
		if err != nil {
			return nil, isxerrors.Errorf("executing template: %w", err)
		}
	}
	code, err := format.Source(b.Bytes())
	if err != nil {
		return nil, isxerrors.Errorf("formatting source: %w", err)
	}
	return code, nil
}

const wrapperTemplate = `
{{- define "x" -}}
Input{
    Name: "{{.Name}}",
    Type: "{{.Type}}",
    {{ if eq .Type "tuple" -}}
    Inputs: []Input{
    {{ range .Inputs -}}
    {{ template "x" . }}
    {{ end }}
    },
    {{ end }}
},
{{- end -}}

var {{ export .Name }}Event = Event{
    Name: "{{ .Name }}",
    Inputs: []Input{
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
    it *abit.Item
}
{{ template "y" $inp -}}
{{ end -}}
{{ end -}}
{{ end -}}
{{ template "y" . -}}

{{- define "z" -}}
{{ $event := . -}}
{{ range $index, $inp := .Inputs -}}
{{ $fn := export $inp.Name }}
{{ if eq $inp.Type "tuple" -}}
func (x *{{ export $event.Name }}){{ export $inp.Name }}() *{{ $inp.Name }} {
    return &{{ $inp.Name }}{x.it.At({{ $index }})}
}
{{ template "z" $inp -}}
{{ else if eq $inp.Type "address" -}}
func (x *{{ $event.Name}}){{ $fn }}() [20]byte {
    return x.it.At({{ $index }}).Address()
}
{{ else if eq $inp.Type "int" -}}
func (x *{{ $event.Name}}){{ $fn }}() int64 {
    return x.it.At({{ $index }}).Int64()
}
{{ else if eq $inp.Type "uint256" -}}
func (x *{{ $event.Name}}){{ $fn }}() *big.Int {
    return x.it.At({{ $index }}).BigInt()
}
{{ end -}}
{{ end -}}
{{ end -}}
{{ template "z" . -}}
`
