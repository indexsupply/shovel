package schema

import (
	"fmt"
	"strconv"
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

func (t Type) String() string {
	switch t.Kind {
	case 'a':
		switch t.Length {
		case 0:
			return fmt.Sprintf("[]%s", t.Elem.String())
		default:
			return fmt.Sprintf("[%d]%s", t.Length, t.Elem.String())
		}
	case 't':
		var s strings.Builder
		s.WriteString("tuple(")
		for i := range t.Fields {
			s.WriteString(t.Fields[i].String())
			if i+1 != len(t.Fields) {
				s.WriteString(",")
			}
		}
		s.WriteString(")")
		return s.String()
	case 's':
		return "static"
	case 'd':
		return "dynamic"
	default:
		return fmt.Sprintf("unkown-type=%d", t.Kind)
	}
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
	t := Type{Kind: 'a', Elem: &e, Length: k}
	if !e.static() {
		return t
	}
	t.Static = true
	t.Size = k * e.size()
	return t
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
	case t.Kind == 'a' && t.Elem.static():
		return t.Length * t.Elem.size()
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

func (t Type) Contains(kind byte) bool {
	switch {
	case t.Kind == 'a':
		return t.Elem.Contains(kind)
	case t.Kind == 't':
		for i := range t.Fields {
			if t.Fields[i].Kind == kind || t.Fields[i].Contains(kind) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func Parse(s string) Type {
	s = strings.ReplaceAll(s, " ", "")
	switch {
	case strings.HasSuffix(s, "]"):
		var num string
		for i := len(s) - 2; i != 0; i-- {
			if s[i] == '[' {
				break
			}
			num += string(s[i])
		}
		if len(num) == 0 {
			return Array(Parse(s[:len(s)-2]))
		}
		k, err := strconv.Atoi(num)
		if err != nil {
			panic("abi/schema: array contains non-number length")
		}
		return ArrayK(k, Parse(s[:len(s)-len(num)-2]))
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
