package session

import (
	"container/list"
	"fmt"
	"sync"
)

// Endpoint is a UDP endpoint in the discv5 protocol, consisting of a Node ID and UPD port number.
type Endpoint struct {
	NodeID uint64
	Port   uint16
}

func (e Endpoint) String() string {
	return fmt.Sprintf("%d:%d", e.NodeID, e.Port)
}

type cacheEntry struct {
	k Endpoint
	v interface{} // replace with session info struct
}

type Cache struct {
	mu         sync.Mutex
	lru        *list.List // Doubly-linked for storing items in LRU order
	size       int
	entriesMap map[Endpoint]*list.Element
}

const defaultMaxSize int = 100

func NewCache() *Cache {
	return &Cache{
		lru:        list.New(),
		size:       defaultMaxSize,
		entriesMap: make(map[Endpoint]*list.Element),
	}
}

func (c *Cache) StoreSession(endpoint Endpoint, session interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if el, ok := c.entriesMap[endpoint]; ok {
		// cache hit; update
		el.Value.(*cacheEntry).v = session
		c.lru.MoveToFront(el)
	} else {
		newEntry := c.lru.PushFront(&cacheEntry{k: endpoint, v: session})
		c.entriesMap[endpoint] = newEntry

		if c.lru.Len() > c.size {
			// evict
			last := c.lru.Back()
			c.lru.Remove(last)
			delete(c.entriesMap, last.Value.(*cacheEntry).k)
		}
	}
}

func (c *Cache) GetSession(endpoint Endpoint) (session interface{}, found bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	el, found := c.entriesMap[endpoint]

	if !found {
		return nil, found
	}

	c.lru.MoveToFront(el) // update LRU
	return el.Value.(*cacheEntry).v, found
}
