package config

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/indexsupply/x/abi2"
	"github.com/indexsupply/x/e2pg"
	"github.com/indexsupply/x/integrations/erc1155"
	"github.com/indexsupply/x/integrations/erc20"
	"github.com/indexsupply/x/integrations/erc4337"
	"github.com/indexsupply/x/integrations/erc721"
	"github.com/indexsupply/x/integrations/txinputs"
	"github.com/indexsupply/x/jrpc2"
	"github.com/indexsupply/x/rlps"
	"github.com/jackc/pgx/v5/pgxpool"
)

var compiled = map[string]e2pg.Integration{
	"erc20":    erc20.Integration,
	"erc721":   erc721.Integration,
	"erc1155":  erc1155.Integration,
	"erc4337":  erc4337.Integration,
	"txinputs": txinputs.Integration,
}

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
	intgsBySource := map[Source][]e2pg.Integration{}
	for _, ig := range conf.Integrations {
		if !ig.Enabled {
			continue
		}
		eig, err := getIntegration(pgp, ig)
		if err != nil {
			return nil, fmt.Errorf("unable to build integration: %s", ig.Name)
		}
		for _, src := range ig.Sources {
			intgsBySource[src] = append(intgsBySource[src], eig)
		}
	}

	var (
		taskID uint64 = 1
		tasks  []*e2pg.Task
	)
	// Start per-source main tasks
	for src, intgs := range intgsBySource {
		chainID, node, err := getNode(conf.EthSources, src.Name)
		if err != nil {
			return nil, fmt.Errorf("unkown source: %s", src.Name)
		}
		tasks = append(tasks, e2pg.NewTask(
			taskID,
			chainID,
			src.Name,
			1,
			1,
			node,
			pgp,
			src.Start,
			0,
			intgs...,
		))
		taskID++
	}
	return tasks, nil
}

func getIntegration(pgp *pgxpool.Pool, ig Integration) (e2pg.Integration, error) {
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

func getNode(srcs []EthSource, name string) (uint64, e2pg.Node, error) {
	for _, src := range srcs {
		if src.Name != name {
			continue
		}
		switch {
		case strings.Contains(src.URL, "rlps"):
			return src.ChainID, rlps.NewClient(src.URL), nil
		case strings.HasPrefix(src.URL, "http"):
			return src.ChainID, jrpc2.New(src.URL), nil
		default:
			// TODO add back support for local node
			return 0, nil, fmt.Errorf("unsupported src type: %v", src)
		}
	}
	return 0, nil, fmt.Errorf("unable to find src for %s", name)
}
