package schema

import (
	"strings"
)

type Type struct {
	Kind   byte
	Static bool
	Size   int

	// tuple
	Fields []Type

	// array
	Length int
	Elem   *Type
}

func Static() Type {
	return Type{Kind: 's', Size: 32, Static: true}
}

func Dynamic() Type {
	return Type{Kind: 'd'}
}

func Array(e Type) Type {
	return Type{
		Kind:   'a',
		Static: false,
		Size:   0,
		Elem:   &e,
		Length: 0,
	}
}

func ArrayK(k int, e Type) Type {
	return Type{
		Kind:   'a',
		Elem:   &e,
		Length: k,
	}
}

func Tuple(fields ...Type) Type {
	t := Type{Kind: 't', Static: true, Fields: fields}
	for _, f := range fields {
		if f.static() {
			t.Size += f.size()
			continue
		}
		t.Size = 0
		t.Static = false
		break
	}
	return t
}

func (t Type) static() bool {
	switch {
	case t.Kind == 'd':
		return false
	case t.Kind == 'a' && t.Length == 0:
		return false
	case t.Kind == 'a' && !t.Elem.static():
		return false
	case t.Kind == 't':
		for _, f := range t.Fields {
			if !f.static() {
				return false
			}
		}
	}
	return true
}

func (t Type) size() int {
	if !t.static() {
		return 0
	}
	switch {
	case t.Kind == 'a' && t.Elem.Kind == 's':
		return t.Length * 32
	case t.Kind == 't':
		var n int
		for _, t := range t.Fields {
			n += t.size()
		}
		return n
	default:
		return 32
	}
}

func Parse(s string) Type {
	switch {
	case strings.HasSuffix(s, "[]"):
		return Array(Parse(strings.TrimSuffix(s, "[]")))
	case strings.HasPrefix(s, "("):
		var (
			types []Type
			list  = s[1 : len(s)-1]
			depth int
		)
		for i, j := 0, 0; i < len(list); i++ {
			switch {
			case i+1 == len(list):
				types = append(types, Parse(list[j:i+1]))
			case depth == 0 && list[i] == ',':
				types = append(types, Parse(list[j:i]))
				j = i + 1
			case list[i] == '(':
				depth++
			case list[i] == ')':
				depth--
			}
		}
		return Tuple(types...)
	case s == "bytes" || s == "string":
		return Dynamic()
	default:
		return Static()
	}
}
