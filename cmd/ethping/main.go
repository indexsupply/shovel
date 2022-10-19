package main

import (
	"fmt"

	"github.com/indexsupply/lib/discv5"
)

func main() {
	go func() {
		err := discv5.Serve()
		if err != nil {
			fmt.Printf("server error: %s\n", err)
		}
	}()
	res, err := discv5.Random()
	fmt.Println(err, res)
	select {}
}
