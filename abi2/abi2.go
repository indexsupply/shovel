// decode abi data into flat rows using a JSON ABI file for the schema
package abi2

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"unicode"

	"github.com/indexsupply/x/bint"
	"github.com/indexsupply/x/eth"
	"github.com/indexsupply/x/isxhash"
	"github.com/indexsupply/x/wctx"
	"github.com/indexsupply/x/wpg"

	"github.com/holiman/uint256"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type atype struct {
	kind   byte
	size   int
	static bool

	// query annotations
	sel bool
	pos int

	// tuple
	fields []atype

	// array
	length int
	elem   *atype
}

func (t atype) String() string {
	switch t.kind {
	case 'a':
		switch t.length {
		case 0:
			return fmt.Sprintf("[]%s", t.elem.String())
		default:
			return fmt.Sprintf("[%d]%s", t.length, t.elem.String())
		}
	case 't':
		var s strings.Builder
		s.WriteString("tuple(")
		for i := range t.fields {
			s.WriteString(t.fields[i].String())
			if i+1 != len(t.fields) {
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
		return fmt.Sprintf("unkown-type=%d", t.kind)
	}
}

func (t atype) hasSelect() bool {
	switch {
	case t.kind == 'a':
		return t.elem.hasSelect()
	case t.kind == 't':
		for i := range t.fields {
			if t.fields[i].hasSelect() {
				return true
			}
		}
		return false
	default:
		return t.sel
	}
}

func (t atype) hasKind(k byte) bool {
	switch t.kind {
	case 'a':
		for tt := t.elem; tt != nil; tt = tt.elem {
			if tt.kind == k {
				return true
			}
		}
		return false
	case 't':
		for i := range t.fields {
			if t.fields[i].kind == k || t.fields[i].hasKind(k) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func (t atype) selected() []atype {
	var res []atype
	switch t.kind {
	case 't':
		for i := range t.fields {
			res = append(res, t.fields[i].selected()...)
		}
	case 'a':
		res = append(res, t.elem.selected()...)
	default:
		if t.sel {
			res = append(res, t)
		}
	}
	return res
}

func hasStatic(t atype) bool {
	switch {
	case t.kind == 'd':
		return false
	case t.kind == 'a' && t.length == 0:
		return false
	case t.kind == 'a' && !hasStatic(*t.elem):
		return false
	case t.kind == 't':
		for _, f := range t.fields {
			if !hasStatic(f) {
				return false
			}
		}
	}
	return true
}

func sizeof(t atype) int {
	switch t.kind {
	case 's':
		return 32
	case 'd':
		return 0
	case 'a':
		return t.length * sizeof(*t.elem)
	case 't':
		var n int
		for i := range t.fields {
			n += sizeof(t.fields[i])
		}
		return n
	default:
		panic("unkown type")
	}
}

func static() atype {
	return atype{kind: 's', static: true, size: 32}
}

func dynamic() atype {
	return atype{kind: 'd', size: 0}
}

func array(e atype) atype {
	return atype{kind: 'a', elem: &e}
}

func arrayK(k int, e atype) atype {
	t := atype{kind: 'a', elem: &e, length: k}
	t.size = sizeof(t)
	t.static = hasStatic(t)
	return t
}

func tuple(fields ...atype) atype {
	t := atype{kind: 't', fields: fields}
	t.size = sizeof(t)
	t.static = hasStatic(t)
	return t
}

func sel(pos int, t atype) atype {
	t.pos = pos
	t.sel = true
	return t
}

type row [][]byte

func (r *Result) Len() int {
	return r.n
}

func (r *Result) At(i int) row {
	return r.collection[i]
}

func (r *Result) GetRow() row {
	r.n++
	if r.n >= len(r.collection) {
		r.collection = append(r.collection, make(row, r.ncols))
	}
	return r.collection[r.n-1]
}

func (r *Result) Bytes() [][][]byte {
	var res = make([][][]byte, r.Len())
	for i := 0; i < r.Len(); i++ {
		res[i] = append(res[i], r.collection[i]...)
	}
	return res
}

type Result struct {
	singleton  row
	collection []row
	t          atype
	n          int
	ncols      int
}

func NewResult(t atype) *Result {
	r := &Result{}
	r.t = t
	r.ncols = len(t.selected())
	r.singleton = make(row, r.ncols)
	return r
}

// Decodes the ABI input data according to r's type
// that was specified in [NewResult]
func (r *Result) Scan(input []byte) error {
	r.n = 0 //reset
	if err := scan(r.singleton, r, input, r.t); err != nil {
		return err
	}

	// if there weren't any variadic selections
	// then instantiate a new row and copy
	// the singleton into that row
	if r.Len() == 0 {
		r.GetRow()
	}
	for i := 0; i < r.Len(); i++ {
		for j := 0; j < len(r.singleton); j++ {
			if len(r.singleton[j]) > 0 {
				r.collection[i][j] = r.singleton[j]
			}
		}
	}
	return nil
}

func scan(r row, res *Result, input []byte, t atype) error {
	switch t.kind {
	case 's':
		if len(input) < 32 {
			return errors.New("EOF")
		}
		if t.sel {
			r[t.pos] = input[:32]
		}
	case 'd':
		length := int(bint.Decode(input[:32]))
		if length == 0 {
			return nil
		}
		if len(input) < 32+length {
			return errors.New("EOF")
		}
		if t.sel {
			r[t.pos] = input[32 : 32+length]
		}
	case 'a':
		if !t.hasSelect() {
			return nil
		}
		var length, start, pos = t.length, 0, 0
		if length <= 0 { // dynamic sized array
			if len(input) < 32 {
				return errors.New("EOF")
			}
			length, start, pos = int(bint.Decode(input[:32])), 32, 32
		}
		for i := 0; i < length; i++ {
			if !t.hasKind('a') {
				r = res.GetRow()
			}
			switch {
			case t.elem.static:
				if len(input) < pos {
					return errors.New("EOF")
				}
				err := scan(r, res, input[pos:], *t.elem)
				if err != nil {
					return err
				}
				pos += t.elem.size
			default:
				if len(input) < pos+32 {
					return errors.New("EOF")
				}
				offset := int(bint.Decode(input[pos : pos+32]))
				if len(input) < start+offset {
					return errors.New("EOF")
				}
				err := scan(r, res, input[start+offset:], *t.elem)
				if err != nil {
					return errors.New("EOF")
				}
				pos += 32
			}
		}
		return nil
	case 't':
		if !t.hasSelect() {
			return nil
		}
		var pos int
		for _, f := range t.fields {
			switch {
			case f.static:
				if len(input) < pos {
					return errors.New("EOF")
				}
				err := scan(r, res, input[pos:], f)
				if err != nil {
					return errors.New("EOF")
				}
				pos += f.size
			default:
				if len(input) < pos+32 {
					return errors.New("EOF")
				}
				offset := int(bint.Decode(input[pos : pos+32]))
				if len(input) < offset {
					return errors.New("EOF")
				}
				err := scan(r, res, input[offset:], f)
				if err != nil {
					return errors.New("EOF")
				}
				pos += 32
			}
		}
		return nil
	default:
		panic("unknown type")
	}
	return nil
}

type Input struct {
	Indexed    bool    `json:"indexed"`
	Name       string  `json:"name"`
	Type       string  `json:"type"`
	Components []Input `json:"components"`

	Column string `json:"column"`
	Pos    int    `json:"column_pos"`
	Filter
}

type Filter struct {
	Op  string   `json:"filter_op"`
	Arg []string `json:"filter_arg"`
}

func (f Filter) Accept(d []byte) bool {
	switch {
	case strings.HasSuffix(f.Op, "contains"):
		var res bool
		for i := range f.Arg {
			hb, _ := hex.DecodeString(f.Arg[i])
			if bytes.Contains(d, hb) {
				res = true
				break
			}
		}
		if strings.HasPrefix(f.Op, "!") {
			return !res
		}
		return res
	default:
		return true
	}
}

func parseArray(elm atype, s string) atype {
	if !strings.Contains(s, "]") {
		return elm
	}
	var num string
	for i := len(s) - 2; i != 0; i-- {
		if s[i] == '[' {
			break
		}
		num += string(s[i])
	}
	if len(num) == 0 {
		return array(parseArray(elm, s[:len(s)-2]))
	}
	k, err := strconv.Atoi(num)
	if err != nil {
		panic("abi/schema: array contains non-number length")
	}
	return arrayK(k, parseArray(elm, s[:len(s)-len(num)-2]))
}

func (inp Input) ABIType(pos int) (int, atype) {
	var base atype
	switch {
	case len(inp.Components) > 0:
		var fields []atype
		for i := range inp.Components {
			var f atype
			pos, f = inp.Components[i].ABIType(pos)
			fields = append(fields, f)
		}
		base = tuple(fields...)
	case strings.HasPrefix(inp.Type, "bytes"):
		switch {
		case strings.TrimSuffix(strings.TrimPrefix(inp.Type, "bytes"), "[") == "":
			base = dynamic()
		default:
			base = static()
		}
	case strings.HasPrefix(inp.Type, "string"):
		base = dynamic()
	default:
		base = static()
	}
	if len(inp.Column) > 0 {
		base.sel = true
		base.pos = pos
		pos++
	}
	return pos, parseArray(base, inp.Type)
}

func (inp Input) Signature() string {
	if !strings.HasPrefix(inp.Type, "tuple") {
		return inp.Type
	}
	var s strings.Builder
	s.WriteString("(")
	for i, c := range inp.Components {
		s.WriteString(c.Signature())
		if i+1 < len(inp.Components) {
			s.WriteString(",")
		}
	}
	s.WriteString(")")
	return strings.Replace(inp.Type, "tuple", s.String(), 1)
}

// Returns inputs that have specified a "column" field -- which indicates
// they are to be selected when ABI data is decoded using this Event/Input
// as an ABI schema.
//
// The returned list order coincides with the order of data
// in the log's Topics and the [Result]'s row.
func (inp Input) Selected() []Input {
	var res []Input
	for i := range inp.Components {
		res = append(res, inp.Components[i].Selected()...)
	}
	if len(inp.Column) > 0 {
		res = append(res, inp)
	}
	return res
}

type BlockData struct {
	Name   string `json:"name"`
	Column string `json:"column"`
	Pos    int    `json:"column_pos"`
	Filter
}

func (bd BlockData) Empty() bool {
	return len(bd.Name) == 0
}

type Event struct {
	Anon   bool    `json:"anonymous"`
	Name   string  `json:"name"`
	Type   string  `json:"type"`
	Inputs []Input `json:"inputs"`
}

func (e Event) ABIType() atype {
	var fields []atype
	var pos = 0
	for i := range e.Inputs {
		if e.Inputs[i].Indexed {
			continue
		}
		var f atype
		pos, f = e.Inputs[i].ABIType(pos)
		fields = append(fields, f)
	}
	return tuple(fields...)
}

func (e Event) SignatureHash() []byte {
	return isxhash.Keccak([]byte(e.Signature()))
}

func (e Event) Signature() string {
	var s strings.Builder
	s.WriteString(e.Name)
	s.WriteString("(")
	for i := range e.Inputs {
		s.WriteString(e.Inputs[i].Signature())
		if i+1 < len(e.Inputs) {
			s.WriteString(",")
		}
	}
	s.WriteString(")")
	return s.String()
}

func (e Event) Selected() []Input {
	var res []Input
	for i := range e.Inputs {
		res = append(res, e.Inputs[i].Selected()...)
	}
	return res
}

func (e Event) numIndexed() int {
	var res int
	for _, inp := range e.Inputs {
		if inp.Indexed {
			res++
		}
	}
	return res
}

type coldef struct {
	Input     Input
	BlockData BlockData
	Column    Column
}

// Implements the [e2pg.Integration] interface
type Integration struct {
	name    string
	Event   Event
	Block   []BlockData
	Table   Table
	Columns []string
	coldefs []coldef

	numIndexed    int
	numSelected   int
	numBDSelected int

	resultCache *Result
	sighash     []byte
}

// js must be a json encoded abi event.
//
//	{"name": "MyEent", "type": "event", "inputs": [{"indexed": true, "name": "f", "type": "t"}]}
//
// Each input may include the following field:
//
//	"column": "my_column"
//
// This indicates that the input is mapped to the database column `my_column`
//
// A top level `extra` key is required to map an event onto a database table.
//
//	{"extra": {"table": {"name": "my_table", column: [{"name": "my_column", "type": "db_type"]}}}
//
// Each column may include a `filter_op` and `filter_arg` key so
// that rows may be removed before they are inserted into the database.
// For example:
//
//	{"name": "my_column", "type": "db_type", "filter_op": "contains", "filter_arg": ["0x000"]}
func New(name string, ev Event, bd []BlockData, table Table) (Integration, error) {
	ig := Integration{
		name:  name,
		Event: ev,
		Block: bd,
		Table: table,

		numIndexed:  ev.numIndexed(),
		resultCache: NewResult(ev.ABIType()),
		sighash:     ev.SignatureHash(),
	}
	ig.addRequiredFields()
	if err := ig.validate(); err != nil {
		return ig, fmt.Errorf("validating %s: %w", name, err)
	}
	for _, input := range ev.Selected() {
		c, err := col(ig.Table, input.Column)
		if err != nil {
			return ig, err
		}
		ig.Columns = append(ig.Columns, c.Name)
		ig.coldefs = append(ig.coldefs, coldef{
			Input:  input,
			Column: c,
		})
		ig.numSelected++
	}
	for _, data := range ig.Block {
		c, err := col(ig.Table, data.Column)
		if err != nil {
			return ig, err
		}
		ig.Columns = append(ig.Columns, c.Name)
		ig.coldefs = append(ig.coldefs, coldef{
			BlockData: data,
			Column:    c,
		})
		ig.numBDSelected++
	}
	return ig, nil
}

func (ig *Integration) validate() error {
	if err := ig.validateCols(); err != nil {
		return fmt.Errorf("validating columns: %w", err)
	}
	if err := ig.validateSQL(); err != nil {
		return fmt.Errorf("validating sql input: %w", err)
	}
	return nil
}

func (ig *Integration) validateSQL() error {
	if err := validateString(ig.name); err != nil {
		return fmt.Errorf("invalid ig name %s: %w", ig.name, err)
	}
	for _, c := range ig.Table.Cols {
		if err := validateString(c.Name); err != nil {
			return fmt.Errorf("invalid col name %s: %w", c.Name, err)
		}
		if err := validateString(c.Type); err != nil {
			return fmt.Errorf("invalid col type%s: %w", c.Type, err)
		}
	}
	return nil
}

func (ig *Integration) validateCols() error {
	type config struct {
		c Column
		i Input
		b BlockData
	}
	var (
		ucols   = map[string]struct{}{}
		uinputs = map[string]struct{}{}
		ubd     = map[string]struct{}{}
	)
	for _, c := range ig.Table.Cols {
		if _, ok := ucols[c.Name]; ok {
			return fmt.Errorf("duplicate column: %s", c.Name)
		}
		ucols[c.Name] = struct{}{}
	}
	for _, inp := range ig.Event.Inputs {
		if _, ok := uinputs[inp.Name]; ok {
			return fmt.Errorf("duplicate input: %s", inp.Name)
		}
		uinputs[inp.Name] = struct{}{}
	}
	for _, bd := range ig.Block {
		if _, ok := ubd[bd.Name]; ok {
			return fmt.Errorf("duplicate block data field: %s", bd.Name)
		}
		ubd[bd.Name] = struct{}{}
	}
	// Every selected input must have a coresponding column
	for _, inp := range ig.Event.Selected() {
		var found bool
		for _, c := range ig.Table.Cols {
			if c.Name == inp.Column {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("missing column for %s", inp.Name)
		}
	}
	// Every selected block field must have a coresponding column
	for _, bd := range ig.Block {
		if len(bd.Column) == 0 {
			return fmt.Errorf("missing column for block.%s", bd.Name)
		}
		var found bool
		for _, c := range ig.Table.Cols {
			if c.Name == bd.Column {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("missing column for block.%s", bd.Name)
		}
	}
	return nil
}

func (ig *Integration) addRequiredFields() {
	hasBD := func(name string) bool {
		for _, bd := range ig.Block {
			if bd.Name == name {
				return true
			}
		}
		return false
	}
	hasCol := func(name string) bool {
		for _, c := range ig.Table.Cols {
			if c.Name == name {
				return true
			}
		}
		return false
	}
	add := func(name, t string) {
		if !hasBD(name) {
			ig.Block = append(ig.Block, BlockData{Name: name, Column: name})
		}
		if !hasCol(name) {
			ig.Table.Cols = append(ig.Table.Cols, Column{Name: name, Type: t})
		}
	}

	add("intg_name", "text")
	add("src_name", "text")
	add("block_num", "numeric")
	add("tx_idx", "int4")

	if len(ig.Event.Selected()) > 0 {
		add("log_idx", "int2")
	}
	for _, inp := range ig.Event.Selected() {
		if !inp.Indexed {
			add("abi_idx", "int2")
		}
	}
}

func validateString(s string) error {
	for _, r := range s {
		if !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-') {
			return errors.New("must be: 'a-z', 'A-Z', '0-9', '_', or '-'")
		}
	}
	return nil
}

func col(t Table, name string) (Column, error) {
	for i := range t.Cols {
		if t.Cols[i].Name == name {
			return t.Cols[i], nil
		}
	}
	return Column{}, fmt.Errorf("table %q doesn't contain column %q", t.Name, name)
}

func (ig Integration) Name() string { return ig.name }

func (ig Integration) Events(context.Context) [][]byte { return [][]byte{} }

func (ig Integration) Delete(ctx context.Context, pg wpg.Conn, n uint64) error {
	const q = `
		delete from %s
		where src_name = $1
		and intg_name = $2
		and block_num >= $3
	`
	_, err := pg.Exec(ctx,
		ig.tname(q),
		wctx.SrcName(ctx),
		ig.name,
		n,
	)
	return err
}

func (ig Integration) tname(query string) string {
	return fmt.Sprintf(query, ig.Table.Name)
}

func (ig Integration) Insert(ctx context.Context, pg wpg.Conn, blocks []eth.Block) (int64, error) {
	var (
		err  error
		skip bool
		rows [][]any
		lwc  = &logWithCtx{ctx: wctx.WithIntgName(ctx, ig.Name())}
	)
	for bidx := range blocks {
		lwc.b = &blocks[bidx]
		for ridx := range blocks[bidx].Receipts {
			lwc.r = &lwc.b.Receipts[ridx]
			lwc.t = &lwc.b.Txs[ridx]
			lwc.ridx = ridx
			rows, skip, err = ig.processTx(rows, lwc)
			if err != nil {
				return 0, fmt.Errorf("processing tx: %w", err)
			}
			if skip {
				continue
			}
			for lidx := range blocks[bidx].Receipts[ridx].Logs {
				lwc.l = &lwc.r.Logs[lidx]
				lwc.lidx = lidx
				rows, err = ig.processLog(rows, lwc)
				if err != nil {
					return 0, fmt.Errorf("processing log: %w", err)
				}
			}
		}
	}
	return pg.CopyFrom(
		ctx,
		pgx.Identifier{ig.Table.Name},
		ig.Columns,
		pgx.CopyFromRows(rows),
	)
}

type logWithCtx struct {
	ctx        context.Context
	b          *eth.Block
	t          *eth.Tx
	r          *eth.Receipt
	l          *eth.Log
	ridx, lidx int
}

func (lwc *logWithCtx) get(name string) any {
	switch name {
	case "src_name":
		return wctx.SrcName(lwc.ctx)
	case "intg_name":
		return wctx.IntgName(lwc.ctx)
	case "chain_id":
		return wctx.ChainID(lwc.ctx)
	case "block_hash":
		return lwc.b.Hash()
	case "block_num":
		return lwc.b.Num()
	case "block_time":
		return lwc.b.Time
	case "tx_hash":
		return lwc.t.Hash()
	case "tx_idx":
		return lwc.ridx
	case "tx_signer":
		d, err := lwc.t.Signer()
		if err != nil {
			slog.ErrorContext(lwc.ctx, "unable to derive signer", err)
			return nil
		}
		return d
	case "tx_to":
		return lwc.t.To.Bytes()
	case "tx_value":
		return lwc.t.Value.Dec()
	case "tx_input":
		return lwc.t.Data.Bytes()
	case "log_idx":
		return lwc.lidx
	case "log_addr":
		return lwc.l.Address.Bytes()
	default:
		return nil
	}
}

func (ig Integration) processTx(rows [][]any, lwc *logWithCtx) ([][]any, bool, error) {
	switch {
	case ig.numSelected > 0:
		return rows, false, nil
	case ig.numBDSelected > 0:
		row := make([]any, len(ig.coldefs))
		for i, def := range ig.coldefs {
			switch {
			case !def.BlockData.Empty():
				d := lwc.get(def.BlockData.Name)
				if b, ok := d.([]byte); ok && !def.BlockData.Accept(b) {
					return rows, true, nil
				}
				row[i] = d
			default:
				return rows, false, fmt.Errorf("expected only blockdata coldef")
			}
		}
		rows = append(rows, row)
	}
	return rows, true, nil
}

func (ig Integration) processLog(rows [][]any, lwc *logWithCtx) ([][]any, error) {
	switch {
	case len(lwc.l.Topics)-1 != ig.numIndexed:
		return rows, nil
	case !bytes.Equal(ig.sighash, lwc.l.Topics[0]):
		return rows, nil
	case ig.numSelected > ig.numIndexed:
		err := ig.resultCache.Scan(lwc.l.Data)
		if err != nil {
			return nil, fmt.Errorf("scanning abi data: %w", err)
		}
		for i := 0; i < ig.resultCache.Len(); i++ {
			ictr, actr := 1, 0
			row := make([]any, len(ig.coldefs))
			for j, def := range ig.coldefs {
				switch {
				case def.Input.Indexed:
					d := lwc.l.Topics[ictr]
					row[j] = dbtype(def.Input.Type, d)
					ictr++
				case !def.BlockData.Empty():
					var d any
					switch {
					case def.BlockData.Name == "abi_idx":
						d = i
					default:
						d = lwc.get(def.BlockData.Name)
						if b, ok := d.([]byte); ok && !def.BlockData.Accept(b) {
							return rows, nil
						}
					}
					row[j] = d
				default:
					d := ig.resultCache.At(i)[actr]
					if !def.Input.Accept(d) {
						return rows, nil
					}
					row[j] = dbtype(def.Input.Type, d)
					actr++
				}
			}
			rows = append(rows, row)
		}
	default:
		row := make([]any, len(ig.coldefs))
		for i, def := range ig.coldefs {
			switch {
			case def.Input.Indexed:
				d := dbtype(def.Input.Type, lwc.l.Topics[1+i])
				if b, ok := d.([]byte); ok && !def.Input.Accept(b) {
					return rows, nil
				}
				row[i] = d
			case !def.BlockData.Empty():
				d := lwc.get(def.BlockData.Name)
				if b, ok := d.([]byte); ok && !def.BlockData.Accept(b) {
					return rows, nil
				}
				row[i] = d
			default:
				return nil, fmt.Errorf("no rows for un-indexed data")
			}
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func dbtype(t string, d []byte) any {
	switch {
	case strings.HasPrefix(t, "uint"):
		bits, err := strconv.Atoi(strings.TrimPrefix(t, "uint"))
		if err != nil {
			return d
		}
		switch {
		case bits > 64:
			var x uint256.Int
			x.SetBytes(d)
			return x.Dec()
		default:
			return bint.Decode(d)
		}
	case t == "address":
		if len(d) == 32 {
			return d[12:]
		}
		return d
	case t == "bool":
		var x uint256.Int
		x.SetBytes32(d)
		return x.Dec() == "1"
	case t == "text":
		return string(d)
	default:
		return d
	}
}

func (ig Integration) Count(ctx context.Context, pg *pgxpool.Pool, chainID uint64) string {
	const q = `
		select trim(to_char(count(*), '999,999,999,999'))
		from %s
		where chain_id = $1
	`
	var (
		res string
		fq  = fmt.Sprintf(q, ig.Table.Name)
	)
	err := pg.QueryRow(ctx, fq, chainID).Scan(&res)
	if err != nil {
		return err.Error()
	}
	switch {
	case res == "0":
		return "pending"
	case strings.HasPrefix(res, "-"):
		return "pending"
	default:
		return res
	}
}

func (ig Integration) RecentRows(ctx context.Context, pgp *pgxpool.Pool, chainID uint64) []map[string]any {
	var q strings.Builder
	q.WriteString("select ")
	for i, def := range ig.coldefs {
		q.WriteString(def.Column.Name)
		q.WriteString("::text")
		if i+1 < len(ig.coldefs) {
			q.WriteString(", ")
		}
	}
	q.WriteString(" from ")
	q.WriteString(ig.Table.Name)
	q.WriteString(" where chain_id = $1")
	q.WriteString(" order by block_num desc limit 10")

	rows, _ := pgp.Query(ctx, q.String(), chainID)
	defer rows.Close()
	res, err := pgx.CollectRows(rows, pgx.RowToMap)
	if err != nil {
		slog.Error("error", fmt.Errorf("querying integration: %w", err))
		return nil
	}
	return res
}

type Column struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type Table struct {
	Name string   `json:"name"`
	Cols []Column `json:"columns"`
}

func (t *Table) Create(ctx context.Context, pg wpg.Conn) error {
	var s strings.Builder
	s.WriteString(fmt.Sprintf("create table if not exists %s(", t.Name))
	for i := range t.Cols {
		s.WriteString(fmt.Sprintf("%s %s", t.Cols[i].Name, t.Cols[i].Type))
		if i+1 == len(t.Cols) {
			s.WriteString(")")
			break
		}
		s.WriteString(",")
	}
	_, err := pg.Exec(ctx, s.String())
	return err
}

func Indexes(ctx context.Context, pg wpg.Conn, table string) []map[string]any {
	const q = `
		select indexname, indexdef
		from pg_indexes
		where tablename = $1
	`
	rows, _ := pg.Query(ctx, q, table)
	res, err := pgx.CollectRows(rows, pgx.RowToMap)
	if err != nil {
		return []map[string]any{map[string]any{"error": err.Error()}}
	}
	return res
}

func RowEstimate(ctx context.Context, pg wpg.Conn, table string) string {
	const q = `
		select trim(to_char(reltuples, '999,999,999,999'))
		from pg_class
		where relname = $1
	`
	var res string
	if err := pg.QueryRow(ctx, q, table).Scan(&res); err != nil {
		return err.Error()
	}
	switch {
	case res == "0":
		return "pending"
	case strings.HasPrefix(res, "-"):
		return "pending"
	default:
		return res
	}
}

func TableSize(ctx context.Context, pg wpg.Conn, table string) string {
	const q = `SELECT pg_size_pretty(pg_total_relation_size($1))`
	var res string
	if err := pg.QueryRow(ctx, q, table).Scan(&res); err != nil {
		return err.Error()
	}
	return res
}
