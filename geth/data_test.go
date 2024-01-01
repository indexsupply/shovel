package geth

import (
	"encoding/hex"
	"testing"

	"github.com/indexsupply/x/eth"
	"github.com/indexsupply/x/geth/gethtest"
	"kr.dev/diff"
)

func h2b(s string) []byte {
	b, _ := hex.DecodeString(s)
	return b
}

func TestLoad(t *testing.T) {
	gtest := gethtest.New(t, "http://hera:8545")
	gtest.SetLatest(16000000, h2b("3dc4ef568ae2635db1419c5fec55c4a9322c05302ae527cd40bff380c1d465dd"))
	gtest.SetFreezerMax(16000000)
	defer gtest.Done()

	// requests before 16000000 will go to freezer and others will go to rpc
	// freezer
	bufs := []Buffer{Buffer{Number: 16000000}, Buffer{Number: 16000001}}
	err := Load(nil, bufs, gtest.FileCache, gtest.Client)
	diff.Test(t, t.Fatalf, err, nil)
	diff.Test(t, t.Errorf, eth.Keccak(bufs[0].h), h2b(`3dc4ef568ae2635db1419c5fec55c4a9322c05302ae527cd40bff380c1d465dd`))
	diff.Test(t, t.Errorf, eth.Keccak(bufs[1].h), h2b(`c2beedf91127b83563d2b1a44b9f8a5510febb028440599b3f16cf436637930e`))

	// rpc
	bufs = []Buffer{Buffer{Number: 17461634}}
	err = Load(nil, bufs, gtest.FileCache, gtest.Client)
	diff.Test(t, t.Fatalf, err, nil)
	diff.Test(t, t.Errorf, eth.Keccak(bufs[0].h), h2b(`93e42a0cbc22e03aa237812f774e90675c60e2fbd70af0b8a711673ec27aad37`))
}
