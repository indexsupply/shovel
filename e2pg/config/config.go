package config

import (
	"context"
	"fmt"
	"os"
	"strings"

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

var Integrations = map[string]e2pg.Integration{
	"erc20":    erc20.Integration,
	"erc721":   erc721.Integration,
	"erc1155":  erc1155.Integration,
	"erc4337":  erc4337.Integration,
	"txinputs": txinputs.Integration,
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
			pgp, err = pgxpool.New(context.Background(), Env(conf.PGURL))
			if err != nil {
				return nil, fmt.Errorf("%s dburl invalid: %w", conf.Name, err)
			}
			dbs[conf.PGURL] = pgp
		}
		node, ok := nodes[conf.ETHURL]
		if !ok {
			node, err = parseNode(Env(conf.ETHURL), conf.FreezerPath)
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
		return jrpc2.New(url), nil
	default:
		// TODO add back support for local node
		return nil, fmt.Errorf("unable to create node for %s", url)
	}
}
