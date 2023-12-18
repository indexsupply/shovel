package config

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/indexsupply/x/dig"
	"github.com/indexsupply/x/wos"
	"github.com/indexsupply/x/wpg"
	"github.com/indexsupply/x/wstrings"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Root struct {
	Dashboard    Dashboard     `json:"dashboard"`
	PGURL        string        `json:"pg_url"`
	Sources      []Source      `json:"eth_sources"`
	Integrations []Integration `json:"integrations"`
}

func (conf Root) CheckUserInput() error {
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
	Name        string        `json:"name"`
	ChainID     uint64        `json:"chain_id"`
	URL         wos.EnvString `json:"url"`
	Start       uint64        `json:"start"`
	Stop        uint64        `json:"stop"`
	Concurrency int           `json:"concurrency"`
	BatchSize   int           `json:"batch_size"`
}

func Sources(ctx context.Context, pgp *pgxpool.Pool) ([]Source, error) {
	var res []Source
	const q = `select name, chain_id, url from shovel.sources`
	rows, err := pgp.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("querying sources: %w", err)
	}
	for rows.Next() {
		var s Source
		if err := rows.Scan(&s.Name, &s.ChainID, &s.URL); err != nil {
			return nil, fmt.Errorf("scanning source: %w", err)
		}
		res = append(res, s)
	}
	return res, nil
}

type Compiled struct {
	Name   string          `json:"name"`
	Config json.RawMessage `json:"config"`
}

type Integration struct {
	Name     string          `json:"name"`
	Enabled  bool            `json:"enabled"`
	Sources  []Source        `json:"sources"`
	Table    wpg.Table       `json:"table"`
	Compiled Compiled        `json:"compiled"`
	Block    []dig.BlockData `json:"block"`
	Event    dig.Event       `json:"event"`
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

func (conf Root) IntegrationsBySource(ctx context.Context, pg wpg.Conn) (map[string][]Integration, error) {
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
	res := make(map[string][]Integration)
	for _, ig := range uniq {
		for _, src := range ig.Sources {
			res[src.Name] = append(res[src.Name], ig)
		}
	}
	return res, nil
}

func (conf Root) AllSources(ctx context.Context, pgp *pgxpool.Pool) ([]Source, error) {
	indb, err := Sources(ctx, pgp)
	if err != nil {
		return nil, fmt.Errorf("loading db integrations: %w", err)
	}

	var uniq = map[uint64]Source{}
	for _, src := range indb {
		uniq[src.ChainID] = src
	}
	for _, src := range conf.Sources {
		uniq[src.ChainID] = src
	}

	var res []Source
	for _, src := range uniq {
		res = append(res, src)
	}
	slices.SortFunc(res, func(a, b Source) int {
		return cmp.Compare(a.ChainID, b.ChainID)
	})
	return res, nil
}
