package config

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/indexsupply/x/abi2"
	"github.com/indexsupply/x/e2pg"
	"github.com/indexsupply/x/jrpc2"
	"github.com/indexsupply/x/rlps"
	"github.com/jackc/pgx/v5/pgxpool"
)

var compiled = map[string]e2pg.Destination{}

type EthSource struct {
	Name    string `json:"name"`
	ChainID uint64 `json:"chain_id"`
	URL     string `json:"url"`
}

type Source struct {
	Name  string `json:"name"`
	Start uint64 `json:"start"`
	Stop  uint64 `json:"stop"`
}

type Compiled struct {
	Name   string          `json:"name"`
	Config json.RawMessage `json:"config"`
}

type Integration struct {
	Name     string           `json:"name"`
	Enabled  bool             `json:"enabled"`
	Start    string           `json:"start"`
	Stop     string           `json:"stop"`
	Backfill bool             `json:"backfill"`
	Sources  []Source         `json:"sources"`
	Table    abi2.Table       `json:"table"`
	Compiled Compiled         `json:"compiled"`
	Block    []abi2.BlockData `json:"block"`
	Event    abi2.Event       `json:"event"`
}

type Config struct {
	PGURL        string        `json:"pg_url"`
	EthSources   []EthSource   `json:"eth_sources"`
	Integrations []Integration `json:"integrations"`
}

func (conf Config) Empty() bool {
	return conf.PGURL == ""
}

// If s has a $ prefix then we assume
// that it is a placeholder url and the actual
// url is in an env variable.
//
// # If there is no env var for s then the program will crash with an error
//
// if there is no $ prefix then s is returned
func Env(s string) string {
	if strings.HasPrefix(s, "$") {
		v := os.Getenv(strings.ToUpper(strings.TrimPrefix(s, "$")))
		if v == "" {
			fmt.Printf("expected database url in env: %q\n", s)
			os.Exit(1)
		}
		return v
	}
	return s
}

func NewTasks(conf Config) ([]*e2pg.Task, error) {
	pgp, err := pgxpool.New(context.Background(), Env(conf.PGURL))
	if err != nil {
		return nil, fmt.Errorf("dburl invalid: %w", err)
	}
	destsBySource := map[Source][]e2pg.Destination{}
	for _, ig := range conf.Integrations {
		if !ig.Enabled {
			continue
		}
		eig, err := getIntegration(pgp, ig)
		if err != nil {
			return nil, fmt.Errorf("unable to build integration %s: %w", ig.Name, err)
		}
		for _, src := range ig.Sources {
			destsBySource[src] = append(destsBySource[src], eig)
		}
	}

	// Start per-source main tasks
	var tasks []*e2pg.Task
	for src, dests := range destsBySource {
		node, err := getNode(conf.EthSources, src.Name)
		if err != nil {
			return nil, fmt.Errorf("unkown source: %s", src.Name)
		}
		tasks = append(tasks, e2pg.NewTask(
			e2pg.WithName(src.Name),
			e2pg.WithSource(node),
			e2pg.WithPG(pgp),
			e2pg.WithRange(src.Start, src.Stop),
			e2pg.WithDestinations(dests...),
		))
	}
	return tasks, nil
}

func getIntegration(pgp *pgxpool.Pool, ig Integration) (e2pg.Destination, error) {
	switch {
	case len(ig.Compiled.Name) > 0:
		cig, ok := compiled[ig.Name]
		if !ok {
			return nil, fmt.Errorf("unable to find compiled integration: %s", ig.Name)
		}
		return cig, nil
	default:
		aig, err := abi2.New(ig.Event, ig.Block, ig.Table)
		if err != nil {
			return nil, fmt.Errorf("building abi integration: %w", err)
		}
		if err := abi2.CreateTable(context.Background(), pgp, aig.Table); err != nil {
			return nil, fmt.Errorf("setting up table for abi integration: %w", err)
		}
		return aig, nil
	}
}

func getNode(srcs []EthSource, name string) (e2pg.Source, error) {
	for _, src := range srcs {
		if src.Name != name {
			continue
		}
		switch {
		case strings.Contains(src.URL, "rlps"):
			return rlps.NewClient(src.ChainID, src.URL), nil
		case strings.HasPrefix(src.URL, "http"):
			return jrpc2.New(src.ChainID, src.URL), nil
		default:
			// TODO add back support for local node
			return nil, fmt.Errorf("unsupported src type: %v", src)
		}
	}
	return nil, fmt.Errorf("unable to find src for %s", name)
}
