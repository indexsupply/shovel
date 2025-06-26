// eth_getLogs filter
package glf

import "strings"

type Filter struct {
	needs       []string
	UseHeaders  bool
	UseBlocks   bool
	UseReceipts bool
	UseLogs     bool
	UseTraces   bool

	addresses []string
	topics    [][]string
}

func New(needs, addresses []string, topics [][]string) *Filter {
	f := &Filter{}
	if any(needs, difference(receipt, block, log)) {
		f.UseReceipts = true
		needs = difference(needs, receipt)
	}
	if any(needs, difference(log, block)) {
		f.UseLogs = true
		needs = difference(needs, log)
	}
	if any(needs, difference(trace, block)) {
		f.UseTraces = true
		needs = difference(needs, trace)
	}
	if any(needs, difference(block, header)) {
		f.UseBlocks = true
		needs = difference(needs, block)
	}
	if any(needs, header) {
		f.UseHeaders = true
		needs = difference(needs, header)
	}
	f.addresses = append([]string(nil), addresses...)
	f.topics = append([][]string(nil), topics...)
	return f
}

func (f *Filter) Addresses() []string { return f.addresses }
func (f *Filter) Topics() [][]string  { return f.topics }

func (f *Filter) String() string {
	var opts = make([]string, 0, 7)
	if f.UseLogs {
		opts = append(opts, "l")
	}
	if f.UseHeaders {
		opts = append(opts, "h")
	}
	if f.UseBlocks {
		opts = append(opts, "b")
	}
	if f.UseReceipts {
		opts = append(opts, "r")
	}
	if f.UseTraces {
		opts = append(opts, "t")
	}
	return strings.Join(opts, ",")
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

// returns the strings in ours that aren't in others
func difference(ours []string, others ...[]string) []string {
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
		"block_time",
		"tx_hash",
		"tx_idx",
		"tx_nonce",
		"tx_signer",
		"tx_to",
		"tx_input",
		"tx_value",
		"tx_type",
		"tx_max_priority_fee_per_gas",
		"tx_max_fee_per_gas",
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
		"tx_l1_base_fee_scalar",
		"tx_l1_blob_base_fee",
		"tx_l1_blob_base_fee_scalar",
		"tx_l1_fee",
		"tx_l1_gas_price",
		"tx_l1_gas_used",
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
	trace = []string{
		"trace_action_call_type",
		"trace_action_from",
		"trace_action_to",
		"trace_action_value",
	}
)
