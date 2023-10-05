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
	Sources  []string         `json:"sources"`
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
	var (
		err     error
		tasks   []*e2pg.Task
		pgp     *pgxpool.Pool
		sources = map[string]e2pg.Node{}
	)
	pgp, err = pgxpool.New(context.Background(), Env(conf.PGURL))
	if err != nil {
		return nil, fmt.Errorf("dburl invalid: %w", err)
	}

	for _, es := range conf.EthSources {
		_, ok := sources[es.Name]
		if !ok {
			sources[es.Name] = parseNode(Env(es.URL))
		}
	}

	intgsBySource := map[string][]e2pg.Integration{}
	for _, ig := range conf.Integrations {
		switch {
		case len(ig.Compiled.Name) > 0:
			cig, ok := compiled[ig.Name]
			if !ok {
				return nil, fmt.Errorf("unable to find compiled integration: %s", ig.Name)
			}
			for _, srcName := range ig.Sources {
				_, ok := sources[srcName]
				if !ok {
					return nil, fmt.Errorf("unable to find source: %s", srcName)
				}
				intgsBySource[srcName] = append(intgsBySource[srcName], cig)
			}
		default:
			aig, err := abi2.New(ig.Event, ig.Block, ig.Table)
			if err != nil {
				return nil, fmt.Errorf("building abi integration: %w", err)
			}

			if err := abi2.CreateTable(context.Background(), pgp, aig.Table); err != nil {
				return nil, fmt.Errorf("setting up table for abi integration: %w", err)
			}
			for _, srcName := range ig.Sources {
				if _, ok := sources[srcName]; !ok {
					return nil, fmt.Errorf("unable to find source: %s", srcName)
				}
				intgsBySource[srcName] = append(intgsBySource[srcName], aig)
			}
		}
	}

	var taskID uint64 = 1
	for srcName, intgs := range intgsBySource {
		node, ok := sources[srcName]
		if !ok {
			return nil, fmt.Errorf("unkown source: %s", srcName)
		}
		var chainID uint64
		for i := range conf.EthSources {
			if conf.EthSources[i].Name == srcName {
				chainID = conf.EthSources[i].ChainID
			}
		}
		if chainID == 0 {
			return nil, fmt.Errorf("unkown chain id for source: %s", srcName)
		}
		fmt.Printf("new task: %d %d %s\n", taskID, chainID, srcName)
		tasks = append(tasks, e2pg.NewTask(
			taskID,
			chainID,
			srcName,
			1,
			1,
			node,
			pgp,
			0,
			0,
			intgs...,
		))
		taskID++
	}
	return tasks, nil
}

func parseNode(url string) e2pg.Node {
	switch {
	case strings.Contains(url, "rlps"):
		return rlps.NewClient(url)
	case strings.HasPrefix(url, "http"):
		return jrpc2.New(url)
	default:
		// TODO add back support for local node
		return nil
	}
}
