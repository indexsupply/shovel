package nested

import (
	"bytes"
	"fmt"
	"go/format"
	"strings"
	"text/template"

	"github.com/indexsupply/x/abi"
)

const ct = `
{{with .Event}}
type {{titleize .Name}} struct {
	{{range .Inputs}}
		{{geninput .}}
	{{end}}
}
{{end}}
`

var funcs = template.FuncMap{"geninput": genInput, "titleize": strings.ToTitle}

func genInput(i abi.Input) string {
	if i.Type == "tuple" {
		var c []string
		for _, comp := range i.Components {
			c = append(c, genInput(comp))
		}
		return fmt.Sprintf("%s struct{%s}", strings.ToTitle(i.Name), strings.Join(c, "\n"))
	} else {
		return fmt.Sprintf("%s %s", strings.ToTitle(i.Name), i.Gtype)
	}
}

func Generate() ([]byte, error) {
	data := struct {
		Event abi.Event
	}{
		Event: e,
	}
	var (
		b bytes.Buffer
		t = template.Must(template.New("letter").Funcs(funcs).Parse(ct))
	)
	if err := t.Execute(&b, data); err != nil {
		return nil, err
	}
	return format.Source(b.Bytes())
}
