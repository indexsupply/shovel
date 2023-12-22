// eth_getLogs filter
package glf

import "slices"

type Filter struct {
	needs       []string
	UseHeaders  bool
	UseBlocks   bool
	UseReceipts bool
	UseLogs     bool

	Address []string
	Topics  [][]string
}

func (f *Filter) Merge(o Filter) {
	var uniqNeeds = map[string]struct{}{}
	for i := range o.needs {
		uniqNeeds[o.needs[i]] = struct{}{}
	}
	for i := range f.needs {
		uniqNeeds[f.needs[i]] = struct{}{}
	}
	var needs []string
	for need := range uniqNeeds {
		needs = append(needs, need)
	}
	f.Needs(needs)

	var uniqAddrs = map[string]struct{}{}
	for i := range f.Address {
		uniqAddrs[f.Address[i]] = struct{}{}
	}
	for i := range o.Address {
		uniqAddrs[o.Address[i]] = struct{}{}
	}
	f.Address = nil
	for addr := range uniqAddrs {
		f.Address = append(f.Address, addr)
	}
	slices.Sort(f.Address)

	if len(f.Topics) < len(o.Topics) {
		n := len(o.Topics) - len(f.Topics)
		f.Topics = append(f.Topics, make([][]string, n)...)
	}
	for i := range o.Topics {
		var uniqTopics = map[string]struct{}{}
		for j := range f.Topics[i] {
			uniqTopics[f.Topics[i][j]] = struct{}{}
		}
		for j := range o.Topics[i] {
			uniqTopics[o.Topics[i][j]] = struct{}{}
		}
		f.Topics[i] = f.Topics[i][:0] //want null not []
		for topic := range uniqTopics {
			f.Topics[i] = append(f.Topics[i], topic)
		}
		slices.Sort(f.Topics[i])
	}
}

func (f *Filter) Needs(needs []string) {
	slices.Sort(needs)
	f.needs = nil
	for i := range needs {
		f.needs = append(f.needs, needs[i])
	}
	f.UseHeaders = any(f.needs, unique(header, block, receipt, log))
	f.UseBlocks = any(f.needs, unique(block, header, receipt, log))
	f.UseReceipts = any(f.needs, unique(receipt, header, block, log))
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

func unique(ours []string, others ...[]string) []string {
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
