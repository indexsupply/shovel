// eth_getLogs filter
package glf

type Filter struct {
	needs       []string
	UseBlocks   bool
	UseTxs      bool
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
	}
}

func (f *Filter) Needs(needs []string) {
	f.needs = nil
	for i := range needs {
		f.needs = append(f.needs, needs[i])
	}

	f.UseReceipts = any(f.needs, onlyr)
	f.UseTxs = any(f.needs, onlyt)
	f.UseBlocks = any(f.needs, onlyb) || f.UseTxs
	f.UseLogs = any(f.needs, ld) && !f.UseReceipts
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
	rd = []string{
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
	ld = []string{
		"block_hash",
		"block_num",
		"tx_hash",
		"tx_idx",
		"log_addr",
		"log_idx",
	}
	bd = []string{
		"block_hash",
		"block_num",
		"block_time",
	}
	td = []string{
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
	onlyr = unique(rd, bd, td, ld)
	onlyt = unique(td, bd, rd, ld)
	onlyb = unique(bd, td, rd, ld)
)
