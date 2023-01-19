package abi

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"go/format"
	"text/template"
	"unicode"

	"github.com/indexsupply/x/isxerrors"
)

//go:embed template.txt
var abitemp string

// used to aggregate template data in template.txt
type accessor struct {
	Index int
	Event Event
	Input *Input
}

func coalesce(idx int, e Event, i *Input) accessor {
	return accessor{
		Index: idx,
		Event: e,
		Input: i,
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
		"camel":    camel,
		"coalesce": coalesce,
	})
	t, err = t.Parse(abitemp)
	if err != nil {
		return nil, isxerrors.Errorf("parsing template: %w", err)
	}

	var b bytes.Buffer
	err = t.ExecuteTemplate(&b, "header", pkg)
	if err != nil {
		return nil, isxerrors.Errorf("executing header template: %w", err)
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
		return nil, isxerrors.Errorf("formatting source: %w", err)
	}
	return code, nil
}
