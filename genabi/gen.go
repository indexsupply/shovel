package genabi

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"go/format"
	"os"
	"strings"
	"text/template"
	"unicode"

	"github.com/indexsupply/x/isxerrors"
	"github.com/indexsupply/x/isxhash"
)

//go:embed template.txt
var abitemp string

func imports(descriptors []Descriptor) []string {
	var types = map[string]struct{}{}
	for _, desc := range descriptors {
		for t, _ := range templateType(desc.Inputs) {
			types[t] = struct{}{}
		}
		for t, _ := range templateType(desc.Outputs) {
			types[t] = struct{}{}
		}
	}
	var imports []string
	for t, _ := range types {
		switch t {
		case "*big.Int":
			imports = append(imports, "math/big")
		case "uint256.Int":
			imports = append(imports, "github.com/holiman/uint256")
		}
	}
	for _, desc := range descriptors {
		if desc.StateMutability == "view" {
			imports = append(imports, "github.com/indexsupply/x/jrpc")
			break
		}
	}
	imports = append(imports, "bytes")
	imports = append(imports, "github.com/indexsupply/x/g2pg")
	imports = append(imports, "github.com/indexsupply/x/abi")
	imports = append(imports, "github.com/indexsupply/x/abi/schema")
	return imports
}

func templateType(fields []Field) map[string]struct{} {
	var types = map[string]struct{}{}
	for _, input := range fields {
		if len(input.Components) == 0 {
			types[goType(input.Type, input.Name)] = struct{}{}
			continue
		}
		for t := range templateType(input.Components) {
			types[t] = struct{}{}
		}
	}
	return types
}

func itemFunc(t string) string {
	switch t, _, _ = strings.Cut(t, "["); t {
	case "address":
		return "Address"
	case "bool":
		return "Bool"
	case "bytes":
		return "Bytes"
	case "bytes32":
		return "Bytes32"
	case "bytes4":
		return "Bytes4"
	case "string":
		return "String"
	case "uint8":
		return "Uint8"
	case "uint16":
		return "Uint16"
	case "uint24", "uint32":
		return "Uint32"
	case "uint40", "uint48", "uint56", "uint64":
		return "Uint64"
	case "uint72", "uint80", "uint88", "uint96", "uint104",
		"uint112", "uint120", "uint128", "uint136", "uint144",
		"uint152", "uint160", "uint168", "uint176", "uint184",
		"uint192", "uint200", "uint208", "uint216", "uint224",
		"uint232", "uint240", "uint248", "uint256":
		return "BigInt"
	default:
		panic(fmt.Sprintf("unkown type: %s", t))
	}
}

func goType(t, s string) string {
	switch {
	case strings.HasSuffix(t, "tuple"):
		return camel(s)
	case strings.HasSuffix(t, "]"):
		var (
			elem, _, _ = strings.Cut(t, "[")
			dims       string
		)
		// reverse array notation
		for i := 0; i <= strings.Count(t, "]"); i++ {
			o, c := strings.Index(t, "["), strings.Index(t, "]")
			dims, t = t[o:c+1]+dims, t[c+1:]
		}
		return dims + goType(elem, s)
	}
	switch t {
	case "address":
		return "[20]byte"
	case "bool":
		return "bool"
	case "bytes":
		return "[]byte"
	case "bytes32":
		return "[32]byte"
	case "bytes4":
		return "[4]byte"
	case "string":
		return "string"
	case "uint8":
		return "uint8"
	case "uint16":
		return "uint16"
	case "uint24", "uint32":
		return "uint32"
	case "uint40", "uint48", "uint56", "uint64":
		return "uint64"
	case "uint72", "uint80", "uint88", "uint96", "uint104",
		"uint112", "uint120", "uint128", "uint136", "uint144",
		"uint152", "uint160", "uint168", "uint176", "uint184",
		"uint192", "uint200", "uint208", "uint216", "uint224",
		"uint232", "uint240", "uint248", "uint256":
		return "*big.Int"
	default:
		panic(fmt.Sprintf("unkown type: %s", t))
	}
}

func hasTuple(input Field) bool {
	return strings.HasPrefix(input.Type, "tuple")
}

func isTuple(input Field) bool {
	return strings.HasSuffix(input.Type, "tuple")
}

func isArray(input Field) bool {
	return strings.HasSuffix(input.Type, "]")
}

// used to aggregate template data in template.txt
type arrayHelper struct {
	Field      Field
	StructName string
	Index      int
	Nested     bool
	inputIndex int
}

func (ah arrayHelper) HasNext() bool {
	var dimension int
	for _, c := range ah.Field.Type {
		if c == rune('[') {
			dimension++
		}
	}
	return dimension > ah.Index+1
}

func (ah arrayHelper) ItemIndex() int {
	if ah.Nested {
		return ah.Index
	}
	return ah.inputIndex
}

func (ah arrayHelper) Next() arrayHelper {
	return arrayHelper{
		Nested:     true,
		Field:      ah.Field,
		Index:      ah.Index + 1,
		StructName: ah.StructName,
	}
}

func (ah arrayHelper) Type(gt string) string {
	for i := 0; i < ah.Index; i++ {
		k := strings.Index(gt, "]")
		gt = gt[k+1:]
	}
	return gt
}

func (ah arrayHelper) FixedLength() bool {
	for i := 0; i < len(ah.Field.Type); i++ {
		if ah.Field.Type[i] == ']' && ah.Field.Type[i-1] != '[' {
			return true
		}
	}
	return false
}

// ABI Fields can share tuples so when we create a struct
// from a tuple, we need to ensure we only define the struct once.
var definedStructs = map[string]struct{}{}

// We generate structs for Descriptor.Fields and
// Field.Components. Since both Fields and Components
// are just a []Field, structHelper abstracts Descriptor
// and Field away so that the struct template can
// simply iterate over structHelper.Fields.
// Name can either be Descriptor.Name or Field.Name.
type structHelper struct {
	Name   string
	Fields []Field
}

func (sh structHelper) AlreadyDefined() bool {
	_, exists := definedStructs[sh.Name]
	if exists {
		return true
	}
	definedStructs[sh.Name] = struct{}{}
	return false
}

type Descriptor struct {
	Name            string
	Type            string //event, function, or error
	StateMutability string
	Anonymous       bool
	Inputs          []Field
	Outputs         []Field
}

func (desc Descriptor) InputSignature() string {
	var s strings.Builder
	s.WriteString(desc.Name)
	s.WriteString("(")
	for i := range desc.Inputs {
		s.WriteString(desc.Inputs[i].Signature())
		if i+1 < len(desc.Inputs) {
			s.WriteString(",")
		}
	}
	s.WriteString(")")
	return s.String()
}

func (desc Descriptor) OutputSignature() string {
	var s strings.Builder
	s.WriteString("(")
	for i := range desc.Outputs {
		s.WriteString(desc.Outputs[i].Signature())
		if i+1 < len(desc.Outputs) {
			s.WriteString(",")
		}
	}
	s.WriteString(")")
	return s.String()
}

func keccak(s string) [32]byte {
	return isxhash.Keccak32([]byte(s))
}

func schema(fields []Field) string {
	var s strings.Builder
	s.WriteString("(")
	for i, f := range fields {
		s.WriteString(f.Signature())
		if i+1 < len(fields) {
			s.WriteString(",")
		}
	}
	s.WriteString(")")
	return s.String()
}

func hashLiteral(h [32]byte) string {
	s := "[]byte{"
	for i := range h {
		s += fmt.Sprintf("0x%x", h[i])
		if i != len(h) {
			s += ", "
		}
	}
	return s + "}"
}

type Field struct {
	Indexed    bool
	Name       string
	Type       string
	Components []Field
}

func (f Field) Signature() string {
	if !strings.HasPrefix(f.Type, "tuple") {
		return f.Type
	}
	var s strings.Builder
	s.WriteString("(")
	for i, c := range f.Components {
		s.WriteString(c.Signature())
		if i+1 < len(f.Components) {
			s.WriteString(",")
		}
	}
	s.WriteString(")")
	return strings.Replace(f.Type, "tuple", s.String(), 1)
}

func lower(str string) string {
	c := camel(str)
	u := unicode.ToLower(rune(c[0:1][0]))
	return string(u) + c[1:]
}

func camel(str string) string {
	var (
		in  = []rune(str)
		res []rune
	)
	for i, r := range in {
		switch {
		case r == '_' && i > 0:
			//omit underscore from result unless
			//the underscore is the first char in
			//the string.
		case i == 0 || in[i-1] == '_':
			res = append(res, unicode.ToUpper(r))
		default:
			res = append(res, r)
		}
	}
	return string(res)
}

func unindexed(fields []Field) []Field {
	var res []Field
	for _, f := range fields {
		if f.Indexed {
			continue
		}
		res = append(res, f)
	}
	return res
}

func indexed(fields []Field) []Field {
	var res []Field
	for _, f := range fields {
		if !f.Indexed {
			continue
		}
		res = append(res, f)
	}
	return res
}

func updateFieldName(m map[string]int, f *Field) {
	if f.Name == "" {
		f.Name = "output"
	}
	n, exists := m[f.Name]
	if exists {
		n++
		f.Name = fmt.Sprintf("%s%d", f.Name, n)
		m[f.Name] = n
	}
	for _, c := range f.Components {
		updateFieldName(m, &c)
	}
}

// Reads file at path and calls [Gen]
func GenFile(pkgName string, path string) ([]byte, error) {
	js, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Gen(pkgName, path, js)
}

// Generates formatted source code with log matchers and function callers
// based on the JSON format for a contract’s interface.
//
// pkgName is the package name that will be used when generating code
//
// inputFileName is used to generate a package comment to indicate
// the source of the generated code.
//
// js the the parsed abi json file
func Gen(pkgName string, inputFileName string, js []byte) ([]byte, error) {
	descriptors := []Descriptor{}
	err := json.Unmarshal(js, &descriptors)
	if err != nil {
		return nil, isxerrors.Errorf("parsing abi json: %w", err)
	}

	if len(descriptors) == 0 {
		return nil, fmt.Errorf("no descriptors in json file %q", inputFileName)
	}

	// Some functions will have the same name
	// but different inputs/outputs. In this case
	// we will append a sequential integer onto the name
	// eg Transfer, Transfer2, Transfer3, etc...
	unique := map[string]int{}
	for i, d := range descriptors {
		n, exists := unique[d.Name]
		if !exists {
			unique[d.Name] = 1
			continue
		}
		n++
		descriptors[i].Name = fmt.Sprintf("%s%d", d.Name, n)
		unique[d.Name] = n
	}
	unique = map[string]int{}
	for i := range descriptors {
		// Some inputs/outputs will have a single item
		// schema and that single item will have a blank
		// name. In those cases, we reuse the descriptor
		// name. Eg totalSupply with a single, unnamed
		// uint256 output will be renamed to totalSupply.
		if len(descriptors[i].Inputs) == 1 && descriptors[i].Inputs[0].Name == "" {
			descriptors[i].Inputs[0].Name = descriptors[i].Name
		}
		if len(descriptors[i].Outputs) == 1 && descriptors[i].Outputs[0].Name == "" {
			descriptors[i].Outputs[0].Name = descriptors[i].Name
		}
		// If a field name is duplicated, we will
		// append a sequential integer onto the name.
		for j := range descriptors[i].Inputs {
			updateFieldName(unique, &descriptors[i].Inputs[j])
		}
		for j := range descriptors[i].Outputs {
			updateFieldName(unique, &descriptors[i].Outputs[j])
		}
	}

	// Some inputs/outputs will have a single item
	t := template.New("abi").Funcs(template.FuncMap{
		"camel":       camel,
		"lower":       lower,
		"hashLiteral": hashLiteral,
		"keccak":      keccak,
		"schema":      schema,
		"goType":      goType,
		"itemFunc":    itemFunc,
		"hasTuple":    hasTuple,
		"isTuple":     isTuple,
		"isArray":     isArray,
		"unindexed":   unindexed,
		"indexed":     indexed,
		"sub": func(x, y int) int {
			return x - y
		},
		"add": func(x, y int) int {
			return x + y
		},
		"arrayHelper": func(i int, sn string, f Field) arrayHelper {
			return arrayHelper{StructName: sn, Field: f, inputIndex: i}
		},
		"structHelper": func(name string, fields []Field) structHelper {
			return structHelper{Name: name, Fields: fields}
		},
	})
	t, err = t.Parse(abitemp)
	if err != nil {
		return nil, isxerrors.Errorf("parsing template: %w", err)
	}

	var b bytes.Buffer
	err = t.ExecuteTemplate(&b, "package", struct {
		PackageName string
		InputFile   string
	}{pkgName, inputFileName})
	if err != nil {
		return nil, isxerrors.Errorf("executing package template: %w", err)
	}
	err = t.ExecuteTemplate(&b, "imports", imports(descriptors))
	if err != nil {
		return nil, isxerrors.Errorf("executing imports template: %w", err)
	}

	for _, desc := range descriptors {
		switch desc.Type {
		case "event":
			err = t.ExecuteTemplate(&b, "event", desc)
			if err != nil {
				return nil, isxerrors.Errorf("executing event template: %w", err)
			}
		case "function":
			err = t.ExecuteTemplate(&b, "function", desc)
			if err != nil {
				return nil, isxerrors.Errorf("executing function template: %w", err)
			}
		}
	}
	code, err := format.Source(b.Bytes())
	if err != nil {
		return b.Bytes(), isxerrors.Errorf("formatting source: %w", err)
	}
	return code, nil
}
