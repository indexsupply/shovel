package kademlia

import (
	"container/list"
	"math/bits"
	"sort"
	"sync"
	"time"

	"github.com/indexsupply/x/enr"
)

const (
	maxBucketSize  = 16
	addrByteSize   = 256 // size in bytes of the node ID
	bucketsCount   = 20  // very rare we will ever encounter a node closer than log distance 20 away
	minLogDistance = addrByteSize + 1 - bucketsCount
)

func New() *Table {
	t := new(Table)
	for i := 0; i < bucketsCount; i++ {
		t.buckets[i] = &kBucket{
			lru:     list.List{},
			entries: map[[32]byte]*list.Element{},
		}
	}
	return t
}

type Table struct {
	mu      sync.Mutex // protects buckets
	buckets [bucketsCount]*kBucket
}

type bucketEntry struct {
	node     *enr.Record
	lastSeen time.Time
}

// kBucket stores an ordered list of nodes, from
// most recently seen (head) to least recently seen (tail). The size
// of the list is at most maxBucketSize.
type kBucket struct {
	lru     list.List
	entries map[[32]byte]*list.Element
}

// nodes returns a slice of all the nodes (ENR) stored in this bucket.
func (bucket *kBucket) nodes() []*enr.Record {
	var result []*enr.Record
	for element := bucket.lru.Front(); element != nil; element = element.Next() {
		result = append(result, element.Value.(*bucketEntry).node)
	}
	return result
}

// Store inserts a node into this particular k-bucket. If the k-bucket is full,
// then the least recently seen node is evicted.
func (bucket *kBucket) store(node *enr.Record) {
	if el, ok := bucket.entries[node.ID()]; ok {
		// cache hit; update
		el.Value.(*bucketEntry).lastSeen = time.Now()
		el.Value.(*bucketEntry).node = node
		bucket.lru.MoveToFront(el)
		return
	}

	newEntry := bucket.lru.PushFront(&bucketEntry{node: node, lastSeen: time.Now()})
	bucket.entries[node.ID()] = newEntry

	if bucket.lru.Len() > maxBucketSize {
		// evict least recently seen
		last := bucket.lru.Back()
		bucket.lru.Remove(last)
		delete(bucket.entries, last.Value.(*bucketEntry).node.ID())
	}
}

// Inserts a node record into the Kademlia Table by putting it
// in the appropriate k-bucket based on distance.
func (kt *Table) Insert(self, node *enr.Record) {
	kt.mu.Lock()
	defer kt.mu.Unlock()

	distance := logDistance(self.ID(), node.ID())
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
func (kt *Table) FindClosest(target [32]byte, count int) []*enr.Record {
	kt.mu.Lock()
	defer kt.mu.Unlock()

	s := &enrSorter{
		nodes:  []*enr.Record{},
		target: target,
	}
	for _, b := range kt.buckets {
		s.nodes = append(s.nodes, b.nodes()...)
	}
	sort.Sort(s)
	if len(s.nodes) < count {
		count = len(s.nodes)
	}
	return s.nodes[:count]
}

// Implement the sort.Interface for a slice of node records using
// the xor distance metric from the target node as a way to compare.
type enrSorter struct {
	target [32]byte
	nodes  []*enr.Record
}

func (s *enrSorter) Len() int {
	return len(s.nodes)
}

func (s *enrSorter) Less(i, j int) bool {
	a := logDistance(s.nodes[i].ID(), s.target)
	b := logDistance(s.nodes[j].ID(), s.target)
	return a < b
}

func (s *enrSorter) Swap(i, j int) {
	temp := s.nodes[i]
	s.nodes[i] = s.nodes[j]
	s.nodes[j] = temp
}

// computes the distance between two ENRs defined as
// log_2 (keccak256(n1) XOR keccak256(n2))
func logDistance(h1, h2 [32]byte) int {
	var xorResult uint8
	distance := len(h1) * 8
	for idx := 0; idx < len(h1); idx++ {
		xorResult = h1[idx] ^ h2[idx]
		if xorResult != 0 {
			return distance - bits.LeadingZeros8(xorResult)
		}
		distance -= 8
	}
	return distance
}
