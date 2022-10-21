package discv4

import (
	"container/list"
	"sync"
	"time"

	"github.com/indexsupply/lib/enr"
)
const (
	maxBucketSize = 16
)

type kademliaTable struct {
	selfNode *enr.ENR
	buckets [256]kBucket
}

type bucketEntry struct {
	node *enr.ENR
	lastSeen time.Time
}

// kBucket stores an ordered list of nodes, from
// most recently seen (head) to least recently seen (tail). The size
// of the list is at most maxBucketSize.
type kBucket struct {
	mu sync.Mutex
	lru *list.List
	entriesMap map[string]*list.Element
}

// Store inserts a node into this particular k-bucket. If the k-bucket is full,
// then the least recently seen node is evicted.
func (bucket *kBucket) Store(node *enr.ENR) {
	bucket.mu.Lock()
	defer bucket.mu.Unlock()

	if bucket.lru == nil {
		bucket.lru = list.New()
	}

	if el, ok := bucket.entriesMap[node.NodeAddrHex()]; ok {
		// cache hit; update
		el.Value.(*bucketEntry).lastSeen = time.Now()
		el.Value.(*bucketEntry).node = node
		bucket.lru.MoveToFront(el)
		return
	}

	newEntry := bucket.lru.PushFront(&bucketEntry{node: node, lastSeen: time.Now()})
	bucket.entriesMap[node.NodeAddrHex()] = newEntry

	if bucket.lru.Len() > maxBucketSize {
		// evict least recently seen
		last := bucket.lru.Back()
		bucket.lru.Remove(last)
		delete(bucket.entriesMap, last.Value.(*bucketEntry).node.NodeAddrHex())
	}
}

func newKademliaTable(selfNode *enr.ENR) *kademliaTable {
	return &kademliaTable{
		selfNode: selfNode,
		buckets : [256]kBucket{},
	}
}

func (kt *kademliaTable) Insert(node *enr.ENR) {
	distance := enr.LogDistance(kt.selfNode, node)
	kt.buckets[distance].Store(node)
}