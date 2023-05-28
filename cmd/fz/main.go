/*
fz a command for interacting with Geth's freezer files

Commands:

	file [flags]

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

	"github.com/indexsupply/x/freezer"
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

	cmdFile := flag.NewFlagSet("file", flag.ExitOnError)

	cmdFile.StringVar(&fpath, "f", "/storage/geth/geth/chaindata/ancient/chain/", "path to freezer files")
	cmdFile.StringVar(&table, "t", "headers", "table ∈ {headers, bodies, receipts}")
	cmdFile.Uint64Var(&start, "start", 1, "starting block (inclusive)")
	cmdFile.Uint64Var(&end, "end", 1, "ending block (inclusive)")

	if len(os.Args) < 2 {
		check(fmt.Errorf(`unknown command. possible commands: 'which'`))
	}

	switch arg := os.Args[1]; arg {
	case "file":
		cmdFile.Parse(os.Args[2:])
		file(fpath, table, start, end)
	default:
		check(fmt.Errorf(`unknown command %q. possible commands: 'which'`, arg))
	}
}

func file(fpath, table string, start, end uint64) {
	fz := freezer.New(fpath)
	for i := start; i <= end; i++ {
		f, length, offset, err := fz.File(table, i)
		check(err)
		fstat, err := f.Stat()
		check(err)
		fmt.Printf("block: %d file: %s length: %d offset: %d\n", i, fstat.Name(), length, offset)
	}
}
