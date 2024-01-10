// eth_getLogs filter
package glf

import (
	"hash/fnv"
	"slices"
	"strings"
)

type Filter struct {
	needs       []string
	UseHeaders  bool
	UseBlocks   bool
	UseReceipts bool
	UseLogs     bool

	addresses []string
	topics    [][]string
}

func New(needs, addresses []string, topics [][]string) *Filter {
	f := &Filter{}
	f.Needs(needs)
	f.addresses = append([]string(nil), addresses...)
	f.topics = append([][]string(nil), topics...)
	return f
}

func (f *Filter) ID() uint64 {
	var b strings.Builder
	if f.UseHeaders {
		b.WriteString("headers,")
	}
	if f.UseBlocks {
		b.WriteString("blocks,")
	}
	if f.UseReceipts {
		b.WriteString("receipts,")
	}
	if f.UseLogs {
		b.WriteString("logs,")
	}
	for i := range f.addresses {
		b.WriteString(f.addresses[i])
	}
	for i := range f.topics {
		for j := range f.topics[i] {
			b.WriteString(f.topics[i][j] + ",")
		}
	}

	h := fnv.New64a()
	if _, err := h.Write([]byte(b.String())); err != nil {
		panic(err)
	}
	return h.Sum64()
}

func (f *Filter) Addresses() []string { return f.addresses }
func (f *Filter) Topics() [][]string  { return f.topics }
func (f *Filter) Empty() bool         { return len(f.needs) == 0 }

func (f *Filter) Merge(o Filter) {
	switch {
	case f.Empty() && len(o.addresses) > 0:
		f.addresses = make([]string, len(o.addresses))
		copy(f.addresses, o.addresses)
	case len(f.addresses) > 0 && len(o.addresses) > 0:
		f.addresses = unique(f.addresses, o.addresses)
	default:
		f.addresses = nil
	}
	f.Needs(unique(f.needs, o.needs))
	if len(f.topics) < len(o.topics) {
		n := len(o.topics) - len(f.topics)
		f.topics = append(f.topics, make([][]string, n)...)
	}
	for i := range o.topics {
		f.topics[i] = unique(f.topics[i], o.topics[i])
	}
}

func (f *Filter) Needs(needs []string) {
	f.needs = unique(needs)
	f.UseHeaders = any(f.needs, distinct(header, block, receipt, log))
	f.UseBlocks = any(f.needs, distinct(block, header, receipt, log))
	f.UseReceipts = any(f.needs, distinct(receipt, header, block, log))
	f.UseLogs = !f.UseReceipts && any(f.needs, log)
}

func any(a, b []string) bool {
	for i := range a {
		for j := range b {
			if a[i] == b[j] {
				return true
			}
		}
	}
	return false
}

// returns unique, sorted elements in all of x
func unique(x ...[]string) []string {
	var u = map[string]struct{}{}
	for i := range x {
		for j := range x[i] {
			u[x[i][j]] = struct{}{}
		}
	}
	var res []string
	for k := range u {
		res = append(res, k)
	}
	slices.Sort(res)
	return res
}

// returns the strings in ours that aren't in others
func distinct(ours []string, others ...[]string) []string {
	var uniqueOthers = map[string]struct{}{}
	for i := range others {
		for j := range others[i] {
			uniqueOthers[others[i][j]] = struct{}{}
		}
	}

	var res []string
	for i := range ours {
		_, ok := uniqueOthers[ours[i]]
		if !ok {
			res = append(res, ours[i])
		}
	}
	return res
}

var (
	header = []string{
		"block_hash",
		"block_num",
		"block_time",
	}
	block = []string{
		"block_hash",
		"block_num",
		"tx_hash",
		"tx_idx",
		"tx_nonce",
		"tx_signer",
		"tx_to",
		"tx_input",
		"tx_value",
		"tx_type",
	}
	receipt = []string{
		"block_hash",
		"block_num",
		"tx_hash",
		"tx_idx",
		"tx_signer",
		"tx_to",
		"tx_type",
		"tx_status",
		"tx_gas_used",
		"tx_contract_address",
		"log_addr",
		"log_idx",
	}
	log = []string{
		"block_hash",
		"block_num",
		"tx_hash",
		"tx_idx",
		"log_addr",
		"log_idx",
	}
)
