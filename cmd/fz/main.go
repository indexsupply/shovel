/*
fz a command for interacting with Geth's freezer files

Commands:

	which [flags]

		A sub-command that indicates which data file contains the
		requested block range.

The flags are:

	-f
		The path where the freezer files are located. This is your geth datadir
		+ /chaindata/ancient/chain/
	-t
		The table that you would like to query. Tables include: headers,
		bodies, and receipts.
	-start
		The block to bein with. (inclusive)
	-end
		The block to end with. (inclusive)

*/

package main

import (
	"flag"
	"fmt"
	"os"
	"sort"

	"github.com/indexsupply/x/gethdb"
)

func check(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err.Error())
		os.Exit(1)
	}
}

func main() {
	var (
		start, end uint64
		table      string
		fpath      string
	)

	cmdWhich := flag.NewFlagSet("which", flag.ExitOnError)

	cmdWhich.StringVar(&fpath, "f", "/storage/geth/geth/chaindata/ancient/chain/", "path to freezer files")
	cmdWhich.StringVar(&table, "t", "headers", "table ∈ {headers, bodies, receipts}")
	cmdWhich.Uint64Var(&start, "start", 1, "starting block (inclusive)")
	cmdWhich.Uint64Var(&end, "end", 1, "ending block (inclusive)")

	if len(os.Args) < 2 {
		check(fmt.Errorf(`unknown command. possible commands: 'which'`))
	}

	switch arg := os.Args[1]; arg {
	case "which":
		cmdWhich.Parse(os.Args[2:])
		which(fpath, table, start, end)
	default:
		check(fmt.Errorf(`unknown command %q. possible commands: 'which'`, arg))
	}
}

func which(fpath, table string, start, end uint64) {
	fz := gethdb.NewFreezer(fpath)

	var m = map[uint16]struct{}{}
	for i := start; i <= end; i++ {
		cur, _, nex, _ := fz.FileNum(table, i)
		m[cur] = struct{}{}
		m[nex] = struct{}{}
	}

	var files []int
	for fn, _ := range m {
		files = append(files, int(fn))
	}
	sort.Ints(files)
	for _, fn := range files {
		fmt.Printf("%d\n", fn)
	}
}
