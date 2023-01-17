package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/indexsupply/x/abi"
)

var (
	input   = flag.String("i", "", "input file")
	output  = flag.String("o", "", "output `file` (default stdout)")
	pkgName = flag.String("p", "", "package name for generated code")
)

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func main() {
	flag.Parse()
	if *input == "" {
		check(errors.New("missing -i (input file) arg"))
	}
	if *pkgName == "" {
		check(errors.New("missing -p (package name) arg"))
	}

	js, err := os.ReadFile(*input)
	check(err)
	code, err := abi.Gen(*pkgName, js)
	check(err)

	if *output != "" {
		check(os.WriteFile(*output, code, 0644))
		return
	}
	fmt.Printf("%s\n", code)
}