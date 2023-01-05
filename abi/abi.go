// ABI encoding/decoding with log parsing
//
// Implementation based on the [ABI Spec].
//
// [ABI Spec]: https://docs.soliditylang.org/en/latest/abi-spec.html
package abi

import (
	"math/big"
	"strings"

	"github.com/indexsupply/x/abi/abit"
	"github.com/indexsupply/x/bint"
	"github.com/indexsupply/x/isxhash"
)

type Input struct {
	Name    string
	Type    abit.Type
	Indexed bool
}

type Event struct {
	sig     string
	sigHash [32]byte

	Name      string
	Type      string
	Anonymous bool
	Inputs    []Input
}

// Retruns new [Event] setting Name.
func NamedEvent(name string) *Event {
	return &Event{Name: name}
}

// Adds indexed input to e and returns e for successive calling
func (e *Event) Indexed(name string, t abit.Type) *Event {
	e.Inputs = append(e.Inputs, Input{
		Name:    name,
		Type:    t,
		Indexed: true,
	})
	return e
}

// Adds un-indexed input to e and returns e for successive calling
func (e *Event) UnIndexed(name string, t abit.Type) *Event {
	e.Inputs = append(e.Inputs, Input{
		Name:    name,
		Type:    t,
		Indexed: false,
	})
	return e
}

// Computes signature (eg name(type1,type2)). Caches result on e
func (e *Event) Signature() string {
	if e.sig != "" {
		return e.sig
	}
	var s strings.Builder
	s.WriteString(e.Name)
	s.WriteString("(")
	for i := range e.Inputs {
		s.WriteString(e.Inputs[i].Type.Name())
		if i+1 < len(e.Inputs) {
			s.WriteString(",")
		}
	}
	s.WriteString(")")
	e.sig = s.String()
	return e.sig
}

// Computes keccak hash over [Event.Signature]. Caches result on e
func (e *Event) SignatureHash() [32]byte {
	if e.sigHash != [32]byte{} {
		return e.sigHash
	}
	e.sigHash = isxhash.Keccak32([]byte(e.Signature()))
	return e.sigHash
}

type Log struct {
	Address [20]byte
	Topics  [4][32]byte
	Data    []byte
}

// Matches event data in a log. Returns a map with input names for keys
// and [Item] for types.
//
// A false return value indicates the first log topic doesn't match
// the event's [Event.SignatureHash].
func Match(l Log, e *Event) (map[string]Item, bool) {
	if e.SignatureHash() != l.Topics[0] {
		return nil, false
	}

	var (
		items     = map[string]Item{}
		unindexed []Input
	)
	for i, input := range e.Inputs {
		if !input.Indexed {
			unindexed = append(unindexed, input)
			continue
		}
		items[input.Name] = Bytes(l.Topics[i+1][:])
	}

	var types []abit.Type
	for _, input := range unindexed {
		types = append(types, input.Type)
	}
	item := Decode(l.Data, abit.Tuple(types...))
	for i, input := range unindexed {
		items[input.Name] = item.At(i)
	}
	return items, true
}

type Item struct {
	abit.Type

	// must be d XOR l
	d []byte
	l []Item
}

func Bytes(d []byte) Item {
	return Item{Type: abit.Bytes, d: d}
}

func (it Item) Bytes() []byte {
	return it.d
}

func (it Item) Address() [20]byte {
	if len(it.d) < 32 {
		return [20]byte{}
	}
	return *(*[20]byte)(it.d[12:])
}

func String(s string) Item {
	return Item{Type: abit.String, d: []byte(s)}
}

func (it Item) String() string {
	return string(it.d)
}

func Bool(b bool) Item {
	var d [32]byte
	if b {
		d[31] = 1
	}
	return Item{Type: abit.Bool, d: d[:]}
}

func (it Item) Bool() bool {
	if len(it.d) < 32 {
		return false
	}
	return it.d[31] == 1
}

func BigInt(i *big.Int) Item {
	var b [32]byte
	i.FillBytes(b[:])
	return Item{
		Type: abit.Uint256,
		d:    b[:],
	}
}

func (it Item) BigInt() *big.Int {
	x := &big.Int{}
	x.SetBytes(it.d)
	return x
}

func (it Item) BigIntSlice() []*big.Int {
	var res []*big.Int
	for i := range it.l {
		res = append(res, it.l[i].BigInt())
	}
	return res
}

func Uint64(i uint64) Item {
	var b [32]byte
	bint.Encode(b[:], i)
	return Item{Type: abit.Uint64, d: b[:]}
}

func (it Item) Uint64() uint64 {
	return bint.Decode(it.d)
}

func List(items ...Item) Item {
	return Item{
		Type: abit.List(items[0].Type),
		l:    items,
	}
}

func (it Item) At(i int) Item {
	if len(it.l) < i {
		return Item{}
	}
	return it.l[i]
}

// Returns length of list, tuple, or bytes depending
// on how the item was constructed.
func (it Item) Len() int {
	if len(it.l) > 0 {
		return len(it.l)
	}
	return len(it.d)
}

func Tuple(items ...Item) Item {
	var types []abit.Type
	for i := range items {
		types = append(types, items[i].Type)
	}
	return Item{
		Type: abit.Tuple(types...),
		l:    items,
	}
}

func rpad(l int, d []byte) []byte {
	n := len(d) % l
	if n == 0 {
		return d
	}
	return append(d, make([]byte, l-n)...)
}

// ABI encoding. Not packed.
func Encode(it Item) []byte {
	switch it.Kind {
	case abit.S:
		return it.d
	case abit.D:
		var c [32]byte
		bint.Encode(c[:], uint64(len(it.d)))
		return append(c[:], rpad(32, it.d)...)
	case abit.L:
		var c [32]byte
		bint.Encode(c[:], uint64(len(it.l)))
		return append(c[:], Encode(Tuple(it.l...))...)
	case abit.T:
		var head, tail []byte
		for i := range it.l {
			switch it.l[i].Kind {
			case abit.S:
				head = append(head, Encode(it.l[i])...)
			default:
				var offset [32]byte
				bint.Encode(offset[:], uint64(len(it.l)*32+len(tail)))
				head = append(head, offset[:]...)
				tail = append(tail, Encode(it.l[i])...)
			}
		}
		return append(head, tail...)
	default:
		panic("abi: encode: unkown type")
	}
}

// Decodes ABI encoded bytes into an [Item] according to
// the 'schema' defined by t. For example:
//	Decode(b, abit.Tuple(abit.String, abit.Uint256))
func Decode(input []byte, t abit.Type) Item {
	switch t.Kind {
	case abit.S:
		return Item{Type: t, d: input[:32]}
	case abit.D:
		count := bint.Decode(input[:32])
		return Item{Type: t, d: input[32 : 32+count]}
	case abit.L:
		count := bint.Decode(input[:32])
		items := make([]Item, count)
		for i := uint64(0); i < count; i++ {
			n := 32 + (32 * i) //skip count (head)
			switch t.Elem.Kind {
			case abit.S:
				items[i] = Decode(input[n:], *t.Elem)
			default:
				offset := 32 + bint.Decode(input[n:n+32])
				items[i] = Decode(input[offset:], *t.Elem)
			}
		}
		return List(items...)
	case abit.T:
		items := make([]Item, len(t.Fields))
		for i, f := range t.Fields {
			n := 32 * i
			switch f.Kind {
			case abit.S:
				items[i] = Decode(input[n:n+32], *f)
			default:
				offset := bint.Decode(input[n : n+32])
				items[i] = Decode(input[offset:], *f)
			}
		}
		return Tuple(items...)
	default:
		panic("abi: encode: unkown type")
	}
}
