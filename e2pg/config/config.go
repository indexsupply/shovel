package config

import (
	"context"
	"fmt"
	"strings"

	"github.com/indexsupply/x/e2pg"
	"github.com/indexsupply/x/freezer"
	"github.com/indexsupply/x/integrations/erc1155"
	"github.com/indexsupply/x/integrations/erc721"
	"github.com/indexsupply/x/jrpc"
	"github.com/indexsupply/x/rlps"
	"github.com/jackc/pgx/v5/pgxpool"
)

var Integrations = map[string]e2pg.Integration{
	"erc721":  erc721.Integration,
	"erc1155": erc1155.Integration,
}

type Config struct {
	Name         string   `json:"name"`
	ID           uint64   `json:"id"`
	ChainID      uint64   `json:"chain"`
	Concurrency  uint64   `json:"concurrency"`
	Batch        uint64   `json:"batch"`
	ETHURL       string   `json:"eth"`
	PGURL        string   `json:"pg"`
	FreezerPath  string   `json:"freezer"`
	Integrations []string `json:"integrations"`
	Begin        uint64   `json:"begin"`
	End          uint64   `json:"end"`
}

func (conf Config) Empty() bool {
	return conf.ETHURL == "" || conf.PGURL == ""
}

func NewTasks(confs ...Config) ([]*e2pg.Task, error) {
	var (
		err   error
		tasks []*e2pg.Task
		nodes = map[string]e2pg.Node{}
		dbs   = map[string]*pgxpool.Pool{}
	)
	for _, conf := range confs {
		pgp, ok := dbs[conf.PGURL]
		if !ok {
			pgp, err = pgxpool.New(context.Background(), conf.PGURL)
			if err != nil {
				return nil, fmt.Errorf("%s dburl invalid: %w", conf.Name, err)
			}
			dbs[conf.PGURL] = pgp
		}
		node, ok := nodes[conf.ETHURL]
		if !ok {
			node, err = parseNode(conf.ETHURL, conf.FreezerPath)
			if err != nil {
				return nil, fmt.Errorf("%s ethurl invalid: %w", conf.Name, err)
			}
			nodes[conf.ETHURL] = node
		}

		var intgs []e2pg.Integration
		switch {
		case len(conf.Integrations) == 0:
			for _, ig := range Integrations {
				intgs = append(intgs, ig)
			}
		default:
			for _, name := range conf.Integrations {
				ig, ok := Integrations[name]
				if !ok {
					return nil, fmt.Errorf("unable to find integration: %q", name)
				}
				intgs = append(intgs, ig)
			}
		}

		tasks = append(tasks, e2pg.NewTask(
			conf.ID,
			conf.ChainID,
			conf.Name,
			conf.Batch,
			conf.Concurrency,
			node,
			pgp,
			conf.Begin,
			conf.End,
			intgs...,
		))
	}
	return tasks, nil
}

func parseNode(url, fpath string) (e2pg.Node, error) {
	switch {
	case strings.Contains(url, "rlps"):
		return rlps.NewClient(url), nil
	case strings.HasPrefix(url, "http"):
		rc, err := jrpc.New(jrpc.WithHTTP(url))
		if err != nil {
			return nil, fmt.Errorf("new http rpc client: %w", err)
		}
		return e2pg.NewGeth(freezer.New(fpath), rc), nil
	default:
		rc, err := jrpc.New(jrpc.WithSocket(url))
		if err != nil {
			return nil, fmt.Errorf("new unix rpc client: %w", err)
		}
		return e2pg.NewGeth(freezer.New(fpath), rc), nil
	}
}
