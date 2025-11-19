package config

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/indexsupply/shovel/dig"
	"github.com/indexsupply/shovel/wos"
	"github.com/indexsupply/shovel/wpg"
	"github.com/indexsupply/shovel/wstrings"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Root struct {
	Dashboard    Dashboard     `json:"dashboard"`
	PGURL        string        `json:"pg_url"`
	Sources      []Source      `json:"eth_sources"`
	Integrations []Integration `json:"integrations"`
}

func union(a, b wpg.Table) wpg.Table {
	for i := range b.Columns {
		var found bool
		for j := range a.Columns {
			if b.Columns[i].Name == a.Columns[j].Name {
				found = true
				break
			}
		}
		if !found {
			a.Columns = append(a.Columns, wpg.Column{
				Name: b.Columns[i].Name,
				Type: b.Columns[i].Type,
			})
		}
	}
	return a
}

func Migrate(ctx context.Context, pg wpg.Conn, conf Root) error {
	for _, ig := range conf.Integrations {
		if err := ig.Table.Migrate(ctx, pg); err != nil {
			return fmt.Errorf("migrating integration: %s: %w", ig.Name, err)
		}
	}
	return nil
}

func DDL(conf Root) []string {
	var tables = map[string]wpg.Table{}
	for i := range conf.Integrations {
		nt := conf.Integrations[i].Table
		et, exists := tables[nt.Name]
		if exists {
			nt = union(nt, et)
		}
		tables[nt.Name] = nt
	}
	var res []string
	for _, t := range tables {
		for _, stmt := range t.DDL() {
			res = append(res, stmt)
		}
	}
	return res
}

func ValidateFix(conf *Root) error {
	if err := CheckUserInput(*conf); err != nil {
		return fmt.Errorf("checking config for dangerous strings: %w", err)
	}
	if err := ValidateFilterRefs(conf); err != nil {
		return fmt.Errorf("checking config for filter_refs: %w", err)
	}
	for i := range conf.Integrations {
		if conf.Integrations[i].FilterAGG == "" {
			conf.Integrations[i].FilterAGG = "or"
		}
		if !slices.Contains([]string{"and", "or", ""}, conf.Integrations[i].FilterAGG) {
			return fmt.Errorf("filter_agg must be one of: and, or. got: %s", conf.Integrations[i].FilterAGG)
		}
		conf.Integrations[i].AddRequiredFields()
		AddUniqueIndex(&conf.Integrations[i].Table)
		if err := ValidateColRefs(conf.Integrations[i]); err != nil {
			return fmt.Errorf("checking config for references: %w", err)
		}
	}
	return nil
}

// Checks each integration for a filter_ref and ensures that the referenced
// integration exists and has the specified column.
// Also ensures that the referenced table has an index on the column.
func ValidateFilterRefs(conf *Root) error {
	var igs = map[string]*Integration{}
	for i := range conf.Integrations {
		igs[conf.Integrations[i].Name] = &conf.Integrations[i]
	}
	igexists := func(key string) error {
		if _, ok := igs[key]; ok {
			return nil
		}
		return fmt.Errorf("%q not found", key)
	}

	var cols = map[string]map[string]bool{}
	for _, ig := range conf.Integrations {
		if _, ok := cols[ig.Table.Name]; !ok {
			cols[ig.Table.Name] = map[string]bool{}
		}
		for _, col := range ig.Table.Columns {
			cols[ig.Table.Name][col.Name] = true
		}
	}
	colexists := func(table, col string) error {
		if _, ok := cols[table][col]; ok {
			return nil
		}
		return fmt.Errorf("table %q column %q not found", table, col)
	}

	check := func(ref *dig.Ref) (bool, error) {
		switch {
		case len(ref.Integration) > 0:
			if err := igexists(ref.Integration); err != nil {
				const tag = "filter_ref depends on %q: %w"
				return false, fmt.Errorf(tag, ref.Integration, err)
			}
			if len(ref.Column) == 0 {
				return false, fmt.Errorf("missing filter_ref column")
			}
			var table = igs[ref.Integration].Table.Name
			if err := colexists(table, ref.Column); err != nil {
				return false, fmt.Errorf("filter_ref depends on %q: %w", ref.Column, err)
			}
			ref.Table = table
			return true, nil
		case len(ref.Table) > 0 || len(ref.Column) > 0:
			return false, fmt.Errorf("filter_ref requires integration field")
		default:
			return false, nil
		}
	}
	for i := range conf.Integrations {
		for j := range conf.Integrations[i].Event.Inputs {
			ok, err := check(&conf.Integrations[i].Event.Inputs[j].Filter.Ref)
			if err != nil {
				return err
			}
			if !ok {
				continue
			}
			var (
				refName = conf.Integrations[i].Event.Inputs[j].Filter.Ref.Integration
				refCol  = conf.Integrations[i].Event.Inputs[j].Filter.Ref.Column
			)
			conf.Integrations[i].Dependencies = append(
				conf.Integrations[i].Dependencies,
				refName,
			)
			igs[refName].Table.Index = append(igs[refName].Table.Index, []string{refCol})
		}
		for j := range conf.Integrations[i].Block {
			ok, err := check(&conf.Integrations[i].Block[j].Filter.Ref)
			if err != nil {
				return fmt.Errorf("field %q: %w", conf.Integrations[i].Block[j].Name, err)
			}
			if !ok {
				continue
			}
			var (
				refName = conf.Integrations[i].Block[j].Filter.Ref.Integration
				refCol  = conf.Integrations[i].Block[j].Filter.Ref.Column
			)
			conf.Integrations[i].Dependencies = append(
				conf.Integrations[i].Dependencies,
				refName,
			)
			igs[refName].Table.Index = append(igs[refName].Table.Index, []string{refCol})
		}
	}
	return nil
}

func ValidateColRefs(ig Integration) error {
	var (
		ucols   = map[string]struct{}{}
		uinputs = map[string]struct{}{}
		ubd     = map[string]struct{}{}
	)
	for _, c := range ig.Table.Columns {
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
		for _, c := range ig.Table.Columns {
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
		for _, c := range ig.Table.Columns {
			if c.Name == bd.Column {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("missing column for block.%s", bd.Name)
		}
	}
	// Every notification column must have a coresponding column
	for _, colName := range ig.Notification.Columns {
		var found bool
		for _, c := range ig.Table.Columns {
			if c.Name == colName {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("missing column for notification.%s", colName)
		}
	}
	return nil
}

// sets default unique columns unless already set by user
func AddUniqueIndex(table *wpg.Table) {
	if len(table.Unique) > 0 {
		return
	}
	possible := []string{
		"ig_name",
		"src_name",
		"block_num",
		"tx_idx",
		"log_idx",
		"abi_idx",
		"trace_action_idx",
	}
	var uidx []string
	for i := range possible {
		var found bool
		for j := range table.Columns {
			if table.Columns[j].Name == possible[i] {
				found = true
				break
			}
		}
		if found {
			uidx = append(uidx, possible[i])
		}
	}
	if len(uidx) > 0 {
		table.Unique = append(table.Unique, uidx)
	}
}

func CheckUserInput(conf Root) error {
	var (
		err   error
		check = func(name, val string) {
			if err != nil {
				return
			}
			err = wstrings.Safe(val)
			if err != nil {
				err = fmt.Errorf("%q %w", val, err)
			}
		}
	)
	for _, ig := range conf.Integrations {
		check("integration name", ig.Name)
		check("table name", ig.Table.Name)
		for _, c := range ig.Table.Columns {
			check("column name", c.Name)
			check("column type", c.Type)
		}
		for _, name := range ig.Notification.Columns {
			check("notification column name", name)
		}
		for _, inp := range ig.Event.Inputs {
			check("referenced column name", inp.Filter.Ref.Column)
		}
		for _, bd := range ig.Block {
			check("referenced column name", bd.Filter.Ref.Column)
		}
	}
	for _, sc := range conf.Sources {
		check("source name", sc.Name)
	}
	return err
}

type Dashboard struct {
	EnableLoopbackAuthn bool          `json:"enable_loopback_authn"`
	DisableAuthn        bool          `json:"disable_authn"`
	RootPassword        wos.EnvString `json:"root_password"`
}

type Source struct {
	Name            string
	ChainID         uint64
	URLs            []string
	WSURL           string
	Start           uint64
	Stop            uint64
	PollDuration    time.Duration
	Concurrency     int
	BatchSize       int
	Consensus       Consensus
	ReceiptVerifier ReceiptVerifier
	Audit        Audit
}

type Consensus struct {
	Providers    int           `json:"providers"`
	Threshold    int           `json:"threshold"`
	RetryBackoff time.Duration `json:"retry_backoff"`
	MaxBackoff   time.Duration `json:"max_backoff"`
}

// Validate checks that Consensus configuration is valid
func (c *Consensus) Validate() error {
	if c.Threshold < 1 {
		return fmt.Errorf("threshold must be >= 1, got %d", c.Threshold)
	}
	if c.Providers < c.Threshold {
		return fmt.Errorf("providers (%d) must be >= threshold (%d)", c.Providers, c.Threshold)
	}
	if c.RetryBackoff < 0 {
		return fmt.Errorf("retry_backoff must be non-negative")
	}
	if c.MaxBackoff < c.RetryBackoff {
		return fmt.Errorf("max_backoff must be >= retry_backoff")
	}
	return nil
}

type ReceiptVerifier struct {
	Provider string `json:"provider"`
	Enabled  bool   `json:"enabled"`
}

type Audit struct {
	ProvidersPerBlock int           `json:"providers_per_block"`
	Confirmations     uint64        `json:"confirmations"`
	Parallelism       int           `json:"parallelism"`
	CheckInterval     time.Duration `json:"check_interval"`
	Enabled           bool          `json:"enabled"`
}

func (s *Source) UnmarshalJSON(d []byte) error {
	x := struct {
		Name         wos.EnvString   `json:"name"`
		ChainID      wos.EnvUint64   `json:"chain_id"`
		URL          wos.EnvString   `json:"url"`
		URLs         []wos.EnvString `json:"urls"`
		WSURL        wos.EnvString   `json:"ws_url"`
		Start        wos.EnvUint64   `json:"start"`
		Stop         wos.EnvUint64   `json:"stop"`
		PollDuration wos.EnvString   `json:"poll_duration"`
		Concurrency  wos.EnvInt      `json:"concurrency"`
		BatchSize    wos.EnvInt      `json:"batch_size"`
		Consensus    struct {
			Providers    wos.EnvInt    `json:"providers"`
			Threshold    wos.EnvInt    `json:"threshold"`
			RetryBackoff wos.EnvString `json:"retry_backoff"`
			MaxBackoff   wos.EnvString `json:"max_backoff"`
		} `json:"consensus"`
		ReceiptVerifier struct {
			Provider wos.EnvString `json:"provider"`
			Enabled  bool          `json:"enabled"`
		} `json:"receipt_verifier"`
		Audit struct {
			ProvidersPerBlock wos.EnvInt    `json:"providers_per_block"`
			Confirmations     wos.EnvUint64 `json:"confirmations"`
			Parallelism       wos.EnvInt    `json:"parallelism"`
			CheckInterval     wos.EnvString `json:"check_interval"`
			Enabled           bool          `json:"enabled"`
		} `json:"audit"`
	}{}
	if err := json.Unmarshal(d, &x); err != nil {
		return err
	}
	s.Name = string(x.Name)
	s.ChainID = uint64(x.ChainID)
	s.WSURL = string(x.WSURL)
	s.Start = uint64(x.Start)
	s.Stop = uint64(x.Stop)
	s.Concurrency = int(x.Concurrency)
	s.BatchSize = int(x.BatchSize)
	s.Consensus.Providers = int(x.Consensus.Providers)
	s.Consensus.Threshold = int(x.Consensus.Threshold)
	s.ReceiptVerifier.Provider = string(x.ReceiptVerifier.Provider)
	s.ReceiptVerifier.Enabled = x.ReceiptVerifier.Enabled
	s.Audit.ProvidersPerBlock = int(x.Audit.ProvidersPerBlock)
	s.Audit.Confirmations = uint64(x.Audit.Confirmations)
	s.Audit.Parallelism = int(x.Audit.Parallelism)
	s.Audit.Enabled = x.Audit.Enabled
	if s.Consensus.Providers == 0 {
		s.Consensus.Providers = 1
	}
	if s.Consensus.Threshold == 0 {
		s.Consensus.Threshold = 1
	}

	var urls []string
	urls = append(urls, string(x.URL))
	for _, url := range x.URLs {
		urls = append(urls, string(url))
	}

	for _, u := range urls {
		if len(u) == 0 {
			continue
		}
		s.URLs = append(s.URLs, u)
	}

	s.PollDuration = time.Second
	if len(x.PollDuration) > 0 {
		var err error
		s.PollDuration, err = time.ParseDuration(string(x.PollDuration))
		if err != nil {
			const tag = "unable to parse poll_duration value: %s"
			return fmt.Errorf(tag, string(x.PollDuration))
		}
	}

	s.Consensus.RetryBackoff = 2 * time.Second
	if len(x.Consensus.RetryBackoff) > 0 {
		var err error
		s.Consensus.RetryBackoff, err = time.ParseDuration(string(x.Consensus.RetryBackoff))
		if err != nil {
			const tag = "unable to parse retry_backoff value: %s"
			return fmt.Errorf(tag, string(x.Consensus.RetryBackoff))
		}
	}

	s.Consensus.MaxBackoff = 30 * time.Second
	if len(x.Consensus.MaxBackoff) > 0 {
		var err error
		s.Consensus.MaxBackoff, err = time.ParseDuration(string(x.Consensus.MaxBackoff))
		if err != nil {
			const tag = "unable to parse max_backoff value: %s"
			return fmt.Errorf(tag, string(x.Consensus.MaxBackoff))
		}
	}

	// Audit defaults and parsing, mirroring the style used for PollDuration
	// and consensus backoff fields above.
	s.Audit.CheckInterval = 5 * time.Second
	if len(x.Audit.CheckInterval) > 0 {
		var err error
		s.Audit.CheckInterval, err = time.ParseDuration(string(x.Audit.CheckInterval))
		if err != nil {
			const tag = "unable to parse check_interval value: %s"
			return fmt.Errorf(tag, string(x.Audit.CheckInterval))
		}
	}

	if s.Audit.ProvidersPerBlock == 0 {
		s.Audit.ProvidersPerBlock = 2
	}
	if s.Audit.Confirmations == 0 {
		s.Audit.Confirmations = 128
	}
	if s.Audit.Parallelism == 0 {
		s.Audit.Parallelism = 4
	}

	if len(s.URLs) > 0 && len(s.URLs) < s.Consensus.Providers {
		return fmt.Errorf("configured %d consensus providers but only %d URLs provided", s.Consensus.Providers, len(s.URLs))
	}
	if s.Consensus.Threshold > s.Consensus.Providers {
		return fmt.Errorf("consensus threshold (%d) cannot exceed providers (%d)", s.Consensus.Threshold, s.Consensus.Providers)
	}

	if s.ReceiptVerifier.Enabled {
		if s.ReceiptVerifier.Provider == "" {
			return fmt.Errorf("receipt_verifier.provider must be specified when enabled")
		}
		for _, u := range s.URLs {
			if u == s.ReceiptVerifier.Provider {
				return fmt.Errorf("receipt_verifier.provider must differ from consensus providers")
			}
		}
	}

	return nil
}

func Sources(ctx context.Context, pgp *pgxpool.Pool) ([]Source, error) {
	var res []Source
	const q = `select name, chain_id, url from shovel.sources`
	rows, err := pgp.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("querying sources: %w", err)
	}
	for rows.Next() {
		var (
			s      Source
			urlStr string
		)
		if err := rows.Scan(&s.Name, &s.ChainID, &urlStr); err != nil {
			return nil, fmt.Errorf("scanning source: %w", err)
		}
		s.URLs = append(s.URLs, urlStr)
		res = append(res, s)
	}
	return res, nil
}

type Compiled struct {
	Name   string          `json:"name"`
	Config json.RawMessage `json:"config"`
}

type Integration struct {
	Name         string           `json:"name"`
	Enabled      bool             `json:"enabled"`
	Sources      []Source         `json:"sources"`
	Table        wpg.Table        `json:"table"`
	FilterAGG    string           `json:"filter_agg"`
	Notification dig.Notification `json:"notification"`
	Compiled     Compiled         `json:"compiled"`
	Block        []dig.BlockData  `json:"block"`
	Event        dig.Event        `json:"event"`
	Dependencies []string
}

func (ig *Integration) AddRequiredFields() {
	hasBD := func(name string) bool {
		for _, bd := range ig.Block {
			if bd.Name == name {
				return true
			}
		}
		return false
	}
	hasCol := func(name string) bool {
		for _, c := range ig.Table.Columns {
			if c.Name == name {
				return true
			}
		}
		return false
	}
	add := func(name, t string) {
		if !hasBD(name) {
			ig.Block = append(ig.Block, dig.BlockData{Name: name, Column: name})
		}
		if !hasCol(name) {
			ig.Table.Columns = append(ig.Table.Columns, wpg.Column{
				Name: name,
				Type: t,
			})
		}
	}
	add("ig_name", "text")
	add("src_name", "text")
	add("block_num", "numeric")
	add("tx_idx", "int")
	if len(ig.Event.Selected()) > 0 {
		add("log_idx", "int")
	}
	for _, inp := range ig.Event.Selected() {
		if !inp.Indexed {
			add("abi_idx", "int2")
		}
	}
	for _, bd := range ig.Block {
		if strings.HasPrefix(bd.Name, "trace_") {
			add("trace_action_idx", "int2")
		}
	}
}

func Integrations(ctx context.Context, pg wpg.Conn) ([]Integration, error) {
	var res []Integration
	const q = `select conf from shovel.integrations`
	rows, err := pg.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("querying integrations: %w", err)
	}
	for rows.Next() {
		var buf = []byte{}
		if err := rows.Scan(&buf); err != nil {
			return nil, fmt.Errorf("scanning integration: %w", err)
		}
		var ig Integration
		if err := json.Unmarshal(buf, &ig); err != nil {
			return nil, fmt.Errorf("unmarshaling integration: %w", err)
		}
		res = append(res, ig)
	}
	return res, nil
}

func (ig Integration) Source(name string) (Source, error) {
	for _, sc := range ig.Sources {
		if sc.Name == name {
			return sc, nil
		}
	}
	return Source{}, fmt.Errorf("missing source config for: %s", name)
}

func (conf Root) AllIntegrations(ctx context.Context, pg wpg.Conn) ([]Integration, error) {
	indb, err := Integrations(ctx, pg)
	if err != nil {
		return nil, fmt.Errorf("loading db integrations: %w", err)
	}
	var uniq = map[string]Integration{}
	for _, ig := range indb {
		uniq[ig.Name] = ig
	}
	for _, ig := range conf.Integrations {
		uniq[ig.Name] = ig
	}

	var res []Integration
	for _, ig := range uniq {
		res = append(res, ig)
	}
	return res, nil
}

func (conf Root) AllSources(ctx context.Context, pgp *pgxpool.Pool) ([]Source, error) {
	indb, err := Sources(ctx, pgp)
	if err != nil {
		return nil, fmt.Errorf("loading db integrations: %w", err)
	}

	var uniq = map[string]Source{}
	for _, src := range indb {
		uniq[src.Name] = src
	}
	for _, src := range conf.Sources {
		uniq[src.Name] = src
	}

	var res []Source
	for _, src := range uniq {
		res = append(res, src)
	}
	slices.SortFunc(res, func(a, b Source) int {
		return cmp.Compare(a.Name, b.Name)
	})
	return res, nil
}

func (conf Root) AllSourcesByName(ctx context.Context, pgp *pgxpool.Pool) (map[string]Source, error) {
	sources, err := conf.AllSources(ctx, pgp)
	if err != nil {
		return nil, fmt.Errorf("loading all sources: %w", err)
	}
	res := make(map[string]Source)
	for _, sc := range sources {
		res[sc.Name] = sc
	}
	return res, nil
}
