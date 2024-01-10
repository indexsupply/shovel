package cache

import (
	"fmt"
	"sort"
	"sync"

	"github.com/indexsupply/x/eth"
)

type segment struct {
	sync.Mutex
	done   bool
	blocks []eth.Block
}

type key struct {
	blockNum uint64
	lim      uint64
	filterID uint64
}

type Cache struct {
	sync.Mutex

	segments   map[key]*segment
	segmentLen uint64
}

func New(n uint64) *Cache {
	return &Cache{
		segments:   make(map[key]*segment),
		segmentLen: n,
	}
}

func (c *Cache) prune() {
	const n = 100
	if len(c.segments) < n {
		return
	}
	var keys = make([]key, 0, len(c.segments))
	for k := range c.segments {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		return keys[i].blockNum > keys[j].blockNum
	})
	for i := range keys[n:] {
		delete(c.segments, keys[n+i])
	}
}

type getter func([]eth.Block) error

func (c *Cache) Get(fid, blockNum, lim uint64, f getter) ([]eth.Block, error) {
	c.Lock()
	c.prune()
	seg, ok := c.segments[key{fid, blockNum, lim}]
	if !ok {
		seg = &segment{}
		c.segments[key{fid, blockNum, lim}] = seg
	}
	c.Unlock()

	seg.Lock()
	if seg.done {
		seg.Unlock()
		fmt.Printf("hit  %d %d\n", blockNum, lim)
		return seg.blocks, nil
	}
	fmt.Printf("miss %d %d\n", blockNum, lim)
	seg.blocks = make([]eth.Block, lim)
	for i := uint64(0); i < lim; i++ {
		seg.blocks[i] = eth.Block{Header: eth.Header{
			Number: eth.Uint64(blockNum + i),
		}}
	}
	if err := f(seg.blocks); err != nil {
		return nil, err
	}
	seg.done = true
	seg.Unlock()
	return seg.blocks, nil
}
