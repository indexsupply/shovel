// declarative integration
//
// Pass a set of declarative config to [New] and receive
// an Integration that is compatabile with [shovel.Destination]
package dig

import (
	"bytes"
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/indexsupply/shovel/bint"
	"github.com/indexsupply/shovel/eth"
	"github.com/indexsupply/shovel/shovel/glf"
	"github.com/indexsupply/shovel/wctx"
	"github.com/indexsupply/shovel/wpg"

	"github.com/holiman/uint256"
	"github.com/jackc/pgx/v5"
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
	clear(r.collection[r.n-1])
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
	//reset
	r.n = 0
	clear(r.singleton)

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
	Filter
}

type Ref struct {
	Integration string `json:"integration"`
	Table       string `json:"table"`
	Column      string `json:"column"`
}

type Filter struct {
	Op  string   `json:"filter_op"`
	Arg []string `json:"filter_arg"`
	Ref Ref      `json:"filter_ref"`
}

func (f Filter) Accept(ctx context.Context, pgmut *sync.Mutex, pg wpg.Conn, d any, frs *filterResults) error {
	if len(f.Arg) == 0 && len(f.Ref.Integration) == 0 {
		return nil
	}

	switch v := d.(type) {
	case eth.Bytes:
		d = []byte(v)
	case eth.Uint64:
		d = uint64(v)
	}
	switch v := d.(type) {
	case []byte:
		var res bool
		switch {
		case strings.HasSuffix(f.Op, "contains"):
			switch {
			case len(f.Ref.Table) > 0:
				q := fmt.Sprintf(
					`select true from %s where %s = $1`,
					f.Ref.Table,
					f.Ref.Column,
				)
				pgmut.Lock()
				defer pgmut.Unlock()
				err := pg.QueryRow(ctx, q, v).Scan(&res)
				switch {
				case errors.Is(err, pgx.ErrNoRows):
					res = false
				case err != nil:
					const tag = "filter using reference (%s %s): %w"
					return fmt.Errorf(tag, f.Ref.Table, f.Ref.Column, err)
				}
			default:
				for i := range f.Arg {
					if bytes.Contains(v, eth.DecodeHex(f.Arg[i])) {
						res = true
						break
					}
				}
			}
			if strings.HasPrefix(f.Op, "!") {
				res = !res
			}
			frs.add(res)
		case f.Op == "eq" || f.Op == "ne":
			for i := range f.Arg {
				if bytes.Equal(v, eth.DecodeHex(f.Arg[i])) {
					res = true
					break
				}
			}
			if f.Op == "ne" {
				res = !res
			}
		default:
			res = true
		}
		frs.add(res)
	case string:
		switch f.Op {
		case "contains":
			frs.add(slices.Contains(f.Arg, v))
		case "!contains":
			frs.add(!slices.Contains(f.Arg, v))
		case "eq":
			frs.add(v == f.Arg[0])
		case "ne":
			frs.add(v != f.Arg[0])
		}
	case uint64:
		i, err := strconv.ParseUint(f.Arg[0], 10, 64)
		if err != nil {
			return fmt.Errorf("unable to convert filter arg to int: %q", f.Arg[0])
		}
		switch f.Op {
		case "eq":
			frs.add(v == i)
		case "ne":
			frs.add(v != i)
		case "gt":
			frs.add(v > i)
		case "lt":
			frs.add(v < i)
		}
	case *uint256.Int:
		i := &uint256.Int{}
		if err := i.SetFromDecimal(f.Arg[0]); err != nil {
			return fmt.Errorf("unable to convert filter arg dec to uint256: %q", f.Arg[0])
		}
		switch f.Op {
		case "eq":
			frs.add(v.Cmp(i) == 0)
		case "ne":
			frs.add(v.Cmp(i) != 0)
		case "gt":
			frs.add(v.Cmp(i) == 1)
		case "lt":
			frs.add(v.Cmp(i) == -1)
		}
	}
	return nil
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
	return eth.Keccak([]byte(e.Signature()))
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

type Notification struct {
	Columns []string `json:"columns"`
}

type coldef struct {
	Input     Input
	BlockData BlockData
	Column    wpg.Column
	Notify    bool
}

// Implements the [shovel.Integration] interface
type Integration struct {
	name         string
	Event        Event
	Block        []BlockData
	Table        wpg.Table
	Notification Notification
	filterAGG    string

	Columns []string
	coldefs []coldef

	indexing         indexingOP
	numIndexed       int
	numSelected      int
	numBDSelected    int
	numTraceSelected int
	numNotify        int

	resultCache *Result
	sighash     []byte
}

type indexingOP byte

const (
	indexTx indexingOP = iota
	indexTrace
	indexLog
)

func New(name string, ev Event, bd []BlockData, table wpg.Table, notif Notification, filterAGG string) (Integration, error) {
	ig := Integration{
		name:         name,
		Event:        ev,
		Block:        bd,
		Table:        table,
		Notification: notif,

		filterAGG:   strings.ToLower(filterAGG),
		numNotify:   len(notif.Columns),
		numIndexed:  ev.numIndexed(),
		resultCache: NewResult(ev.ABIType()),
		sighash:     ev.SignatureHash(),
	}
	ig.setCols()
	ig.setIndexing()
	return ig, nil
}

func (ig *Integration) setIndexing() {
	if ig.numBDSelected > 0 {
		ig.indexing = indexTx
	}
	if ig.numSelected > 0 {
		ig.indexing = indexLog
	}
	if ig.numTraceSelected > 0 {
		ig.indexing = indexTrace
	}
}

func (ig *Integration) setCols() {
	getCol := func(name string) wpg.Column {
		for _, c := range ig.Table.Columns {
			if c.Name == name {
				return c
			}
		}
		return wpg.Column{}
	}
	for _, input := range ig.Event.Selected() {
		c := getCol(input.Column)
		ig.Columns = append(ig.Columns, c.Name)
		ig.coldefs = append(ig.coldefs, coldef{
			Input:  input,
			Column: c,
			Notify: slices.Contains(ig.Notification.Columns, c.Name),
		})
		ig.numSelected++
	}
	for _, bd := range ig.Block {
		c := getCol(bd.Column)
		ig.Columns = append(ig.Columns, c.Name)
		ig.coldefs = append(ig.coldefs, coldef{
			BlockData: bd,
			Column:    c,
			Notify:    slices.Contains(ig.Notification.Columns, c.Name),
		})
		ig.numBDSelected++
		if strings.HasPrefix(c.Name, "trace_") {
			ig.numTraceSelected++
		}
	}
}

func (ig Integration) Name() string { return ig.name }

func (ig Integration) Filter() glf.Filter {
	var (
		fields []string
		addrs  []string
	)
	for i := range ig.Block {
		fields = append(fields, ig.Block[i].Name)

		if ig.Block[i].Name == "log_addr" && len(ig.Block[i].Filter.Arg) > 0 {
			for _, arg := range ig.Block[i].Filter.Arg {
				addrs = append(addrs, eth.EncodeHex(eth.DecodeHex(arg)))
			}
		}
	}
	return *glf.New(fields, addrs, [][]string{{eth.EncodeHex(ig.sighash)}})
}

func (ig Integration) Delete(ctx context.Context, pg wpg.Conn, n uint64) error {
	const q = `
		delete from %s
		where src_name = $1
		and ig_name = $2
		and block_num >= $3
	`
	_, err := pg.Exec(ctx,
		fmt.Sprintf(q, ig.Table.Name),
		wctx.SrcName(ctx),
		ig.name,
		n,
	)
	return err
}

func (ig Integration) Insert(ctx context.Context, pgmut *sync.Mutex, pg wpg.Conn, blocks []eth.Block) (int64, error) {
	var (
		err  error
		skip bool
		rows [][]any
		lwc  = &logWithCtx{ctx: wctx.WithIGName(ctx, ig.Name())}
	)
	for bidx := range blocks {
		lwc.b = &blocks[bidx]
		for tidx := range blocks[bidx].Txs {
			lwc.t = &lwc.b.Txs[tidx]
			switch ig.indexing {
			case indexTx:
				rows, skip, err = ig.processTx(rows, lwc, pgmut, pg)
				if err != nil {
					return 0, fmt.Errorf("processing tx: %w", err)
				}
				if skip {
					continue
				}
			case indexTrace:
				for taidx := range blocks[bidx].Txs[tidx].TraceActions {
					lwc.ta = &lwc.t.TraceActions[taidx]
					rows, _, err = ig.processTx(rows, lwc, pgmut, pg)
					if err != nil {
						return 0, fmt.Errorf("processing log: %w", err)
					}
				}
			case indexLog:
				for lidx := range blocks[bidx].Txs[tidx].Logs {
					lwc.l = &lwc.t.Logs[lidx]
					rows, err = ig.processLog(rows, lwc, pgmut, pg)
					if err != nil {
						return 0, fmt.Errorf("processing log: %w", err)
					}
				}
			}
		}
	}
	pgmut.Lock()
	defer pgmut.Unlock()

	nr, err := pg.CopyFrom(
		ctx,
		pgx.Identifier{ig.Table.Name},
		ig.Columns,
		pgx.CopyFromRows(rows),
	)
	if err != nil {
		return 0, err
	}
	if ig.numNotify == 0 {
		return nr, nil
	}
	if err := ig.notify(lwc, pg, rows); err != nil {
		slog.ErrorContext(lwc.ctx, "sending notifications", "error", err)
	}
	return nr, nil
}

func (ig *Integration) notify(lwc *logWithCtx, pg wpg.Conn, rows [][]any) error {
	q := fmt.Sprintf(
		`select pg_notify('%s-%s', $1)`,
		lwc.get("src_name"),
		lwc.get("ig_name"),
	)
	for i := range rows {
		var payload []string
		for j := range ig.Notification.Columns {
			for k := range ig.coldefs {
				if !ig.coldefs[k].Notify {
					continue
				}
				if ig.coldefs[k].Column.Name != ig.Notification.Columns[j] {
					continue
				}
				switch v := rows[i][k].(type) {
				case int:
					payload = append(payload, strconv.Itoa(v))
				case uint64:
					payload = append(payload, strconv.FormatUint(v, 10))
				case eth.Uint64:
					payload = append(payload, strconv.FormatUint(uint64(v), 10))
				case string:
					payload = append(payload, v)
				case []byte:
					payload = append(payload, eth.EncodeHex(v))
				case *uint256.Int:
					payload = append(payload, v.Dec())
				default:
					return fmt.Errorf("unknown type for notification: %T", rows[i][k])
				}
			}
		}
		if _, err := pg.Exec(lwc.ctx, q, strings.Join(payload, ",")); err != nil {
			return fmt.Errorf("sending pg notification: %w", err)
		}
	}
	return nil
}

type logWithCtx struct {
	ctx context.Context
	b   *eth.Block
	t   *eth.Tx
	l   *eth.Log
	ta  *eth.TraceAction
}

func (lwc *logWithCtx) get(name string) any {
	switch name {
	case "src_name":
		return wctx.SrcName(lwc.ctx)
	case "ig_name":
		return wctx.IGName(lwc.ctx)
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
		return lwc.t.Idx
	case "tx_signer":
		d, err := lwc.t.Signer()
		if err != nil {
			slog.ErrorContext(lwc.ctx, "unable to derive signer", "error", err)
			return nil
		}
		return d
	case "tx_to":
		return lwc.t.To.Bytes()
	case "tx_value":
		return &lwc.t.Value
	case "tx_input":
		return lwc.t.Data.Bytes()
	case "tx_type":
		return lwc.t.Type
	case "tx_status":
		return lwc.t.Receipt.Status
	case "log_idx":
		return lwc.l.Idx
	case "tx_gas_used":
		return lwc.t.GasUsed
	case "tx_gas_price":
		return &lwc.t.GasPrice
	case "tx_effective_gas_price":
		return &lwc.t.EffectiveGasPrice
	case "tx_contract_address":
		return lwc.t.ContractAddress.Bytes()
	case "tx_max_priority_fee_per_gas":
		return &lwc.t.MaxPriorityFeePerGas
	case "tx_max_fee_per_gas":
		return &lwc.t.MaxFeePerGas
	case "tx_nonce":
		return lwc.t.Nonce
	case "tx_l1_base_fee_scalar":
		return lwc.t.L1BaseFeeScalar
	case "tx_l1_blob_base_fee":
		return lwc.t.L1BlobBaseFee
	case "tx_l1_blob_base_fee_scalar":
		return lwc.t.L1BlobBaseFeeScalar
	case "tx_l1_fee":
		return lwc.t.L1Fee
	case "tx_l1_gas_price":
		return lwc.t.L1GasPrice
	case "tx_l1_gas_used":
		return lwc.t.L1GasUsed
	case "log_addr":
		return lwc.l.Address.Bytes()
	case "trace_action_call_type":
		return lwc.ta.CallType
	case "trace_action_idx":
		return lwc.ta.Idx
	case "trace_action_from":
		return lwc.ta.From.Bytes()
	case "trace_action_to":
		return lwc.ta.To.Bytes()
	case "trace_action_value":
		return &lwc.ta.Value
	default:
		return nil
	}
}

type filterResults struct {
	kind string
	set  bool
	val  bool
}

func (fr *filterResults) add(b bool) {
	if !fr.set {
		fr.set = true
		fr.val = b
		return
	}
	switch fr.kind {
	case "and":
		fr.val = fr.val && b
	default:
		fr.val = fr.val || b
	}
}

func (fr *filterResults) accept() bool {
	if !fr.set {
		return true
	}
	return fr.val
}

func (ig Integration) processTx(rows [][]any, lwc *logWithCtx, pgmut *sync.Mutex, pg wpg.Conn) ([][]any, bool, error) {
	switch {
	case ig.numSelected > 0:
		return rows, false, nil
	case ig.numBDSelected > 0:
		frs := filterResults{kind: ig.filterAGG}
		row := make([]any, len(ig.coldefs))
		for i, def := range ig.coldefs {
			switch {
			case !def.BlockData.Empty():
				d := lwc.get(def.BlockData.Name)
				if err := def.BlockData.Accept(lwc.ctx, pgmut, pg, d, &frs); err != nil {
					return nil, false, fmt.Errorf("checking filter: %w", err)
				}
				row[i] = d
			default:
				return rows, false, fmt.Errorf("expected only blockdata coldef")
			}
		}
		if frs.accept() {
			rows = append(rows, row)
		}
	}
	return rows, true, nil
}

func (ig Integration) processLog(rows [][]any, lwc *logWithCtx, pgmut *sync.Mutex, pg wpg.Conn) ([][]any, error) {
	switch {
	case len(lwc.l.Topics)-1 != ig.numIndexed:
		return rows, nil
	case !bytes.Equal(ig.sighash, lwc.l.Topics[0]):
		return rows, nil
	case len(lwc.l.Data) > 0:
		err := ig.resultCache.Scan(lwc.l.Data)
		if err != nil {
			return nil, fmt.Errorf("scanning abi data: %w", err)
		}
		for i := 0; i < ig.resultCache.Len(); i++ {
			ictr, actr := 1, 0
			frs := filterResults{kind: ig.filterAGG}
			row := make([]any, len(ig.coldefs))
			for j, def := range ig.coldefs {
				switch {
				case def.Input.Indexed:
					d := dbtype(def.Input.Type, lwc.l.Topics[ictr])
					if err := def.Input.Accept(lwc.ctx, pgmut, pg, d, &frs); err != nil {
						return nil, fmt.Errorf("checking filter: %w", err)
					}
					row[j] = d
					ictr++
				case !def.BlockData.Empty():
					var d any
					switch {
					case def.BlockData.Name == "abi_idx":
						d = i
					default:
						d = lwc.get(def.BlockData.Name)
						if err := def.BlockData.Accept(lwc.ctx, pgmut, pg, d, &frs); err != nil {
							return nil, fmt.Errorf("checking filter: %w", err)
						}
					}
					row[j] = d
				default:
					d := dbtype(def.Input.Type, ig.resultCache.At(i)[actr])
					if err := def.Input.Accept(lwc.ctx, pgmut, pg, d, &frs); err != nil {
						return nil, fmt.Errorf("checking filter: %w", err)
					}
					row[j] = d
					actr++
				}
			}
			if frs.accept() {
				rows = append(rows, row)
			}
		}
	default:
		frs := filterResults{kind: ig.filterAGG}
		row := make([]any, len(ig.coldefs))
		for i, def := range ig.coldefs {
			switch {
			case def.Input.Indexed:
				d := dbtype(def.Input.Type, lwc.l.Topics[1+i])
				if err := def.Input.Accept(lwc.ctx, pgmut, pg, d, &frs); err != nil {
					return nil, fmt.Errorf("checking filter: %w", err)
				}
				row[i] = d
			case !def.BlockData.Empty():
				d := lwc.get(def.BlockData.Name)
				if err := def.BlockData.Accept(lwc.ctx, pgmut, pg, d, &frs); err != nil {
					return nil, fmt.Errorf("checking filter: %w", err)
				}
				row[i] = d
			default:
				return nil, fmt.Errorf("no rows for un-indexed data")
			}
		}
		if frs.accept() {
			rows = append(rows, row)
		}
	}
	return rows, nil
}

type negInt struct {
	i *uint256.Int
}

func (ni *negInt) Value() (driver.Value, error) {
	v, err := ni.i.Value()
	if ni.i.Sign() < 0 {
		x := uint256.NewInt(0).Neg(ni.i)
		v, err = x.Value()
		return "-" + v.(string), err
	}
	return v, err
}

func dbtype(abitype string, d []byte) any {
	switch {
	case strings.HasPrefix(abitype, "int"):
		x := &uint256.Int{}
		x.SetBytes(d)
		return &negInt{x}
	case strings.HasPrefix(abitype, "uint"):
		var x uint256.Int
		x.SetBytes(d)
		return &x
	case strings.HasPrefix(abitype, "address"):
		if len(d) == 32 {
			return d[12:]
		}
		return d
	case abitype == "bool":
		if len(d) == 32 {
			return d[31] == 0x01
		}
		return false
	case abitype == "string":
		return string(d)
	case abitype == "bytes":
		if len(d) == 0 {
			return []byte{}
		}
		return d
	default:
		return d
	}
}
