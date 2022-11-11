package main

import (
	"time"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/indexsupply/x/enr"
	"github.com/indexsupply/x/devp2p/rlpx"
)

func main() {
	prv, err := secp256k1.GeneratePrivateKey()
	must(err)
	localGeth, err := enr.ParseV4("enode://262e65ab0c0f10f4b00b64dc7284bfacb2ae2a5cb0d99e61f7544be378cfd7c0bb2cd83e2270e7ff41440b9a834cd9187fe504266a575d7c8c980c12696654be@127.0.0.1:30303")
	must(err)

	conn, err := rlpx.Dial(prv, localGeth)
	must(err)

	err = conn.Handshake()
	must(err)

	go func() {
		for ; ; time.Sleep(5 * time.Second) {
			conn.Listen()
		}
	}()

	select{}
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
