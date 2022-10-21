package discv4

import (
	"container/list"
	"sync"
	"time"

	"github.com/indexsupply/lib/enr"
)

const (
	maxBucketSize  = 16
	addrByteSize   = 256 // size in bytes of the node ID
	bucketsCount   = 20  // very rare we will ever encounter a node closer than log distance 20 away
	minLogDistance = addrByteSize + 1 - bucketsCount
)

type kademliaTable struct {
	selfNode *enr.ENR
	buckets  [bucketsCount]kBucket
}

type bucketEntry struct {
	node     *enr.ENR
	lastSeen time.Time
}

// kBucket stores an ordered list of nodes, from
// most recently seen (head) to least recently seen (tail). The size
// of the list is at most maxBucketSize.
type kBucket struct {
	mu         sync.Mutex // protects lru and entriesMap
	lru        *list.List
	entriesMap map[string]*list.Element
}

// Store inserts a node into this particular k-bucket. If the k-bucket is full,
// then the least recently seen node is evicted.
func (bucket *kBucket) store(node *enr.ENR) {
	bucket.mu.Lock()
	defer bucket.mu.Unlock()

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
	t := &kademliaTable{
		selfNode: selfNode,
		buckets:  [bucketsCount]kBucket{},
	}
	// init lists
	for i := 0; i < len(t.buckets); i++ {
		t.buckets[i].lru = list.New()
	}
	return t
}

// Inserts a node record into the Kademlia Table by putting it
// in the appropriate k-bucket based on distance.
func (kt *kademliaTable) Insert(node *enr.ENR) {
	distance := enr.LogDistance(kt.selfNode, node)
	// In the unlikely event that the distance is closer than
	// the mininum, put it in the closest bucket.
	if distance < minLogDistance {
		distance = minLogDistance
	}
	kt.buckets[distance-minLogDistance].store(node)
}

// FindClosest returns the n closest nodes in the local table to target.
// It does a full table scan since the actual algorithm to do this is quite complex
// and the table is not expected to be that large.
func (kt *kademliaTable) FindClosest(target *enr.ENR, count int) []*enr.ENR {
	// todo: implement
	return nil
}
