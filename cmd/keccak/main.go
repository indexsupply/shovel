package main

import (
	"encoding/hex"
	"fmt"
	"io"
	"os"

	"github.com/indexsupply/shovel/eth"
)

func main() {
	switch len(os.Args) {
	case 1:
		input, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		b, err := hex.DecodeString(string(input[:len(input)-1]))
		if err != nil {
			fmt.Println("unable to hex decode stdin")
			os.Exit(1)
		}
		fmt.Printf("%x\n", eth.Keccak(b))
	case 2:
		b, err := hex.DecodeString(os.Args[1])
		if err != nil {
			fmt.Println("unable to hex decode argument")
			os.Exit(1)
		}
		fmt.Printf("%x\n", eth.Keccak(b))
	default:
		fmt.Printf("keccak reads from stdin or through first argument")
	}
}
