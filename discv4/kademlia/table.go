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

type Table struct {
	mu       sync.Mutex // protects buckets
	selfNode *enr.Record
	buckets  [bucketsCount]kBucket
}

type bucketEntry struct {
	node     *enr.Record
	lastSeen time.Time
}

// kBucket stores an ordered list of nodes, from
// most recently seen (head) to least recently seen (tail). The size
// of the list is at most maxBucketSize.
type kBucket struct {
	lru        list.List
	entriesMap map[string]*list.Element
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
func (bucket *kBucket) store(node *enr.REcord) {
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

func New(selfNode *enr.Record) *Table {
	t := &Table{
		selfNode: selfNode,
		buckets:  [bucketsCount]kBucket{},
	}
	return t
}

// Inserts a node record into the Kademlia Table by putting it
// in the appropriate k-bucket based on distance.
func (kt *Table) Insert(node *enr.Record) {
	kt.mu.Lock()
	defer kt.mu.Unlock()

	distance := logDistance(kt.selfNode, node)
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
func (kt *Table) FindClosest(target *enr.Record, count int) []*enr.Record {
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
	return s.nodes[:count]
}

// Implement the sort.Interface for a slice of node records using
// the xor distance metric from the target node as a way to compare.
type enrSorter struct {
	target *enr.Record
	nodes  []*enr.Record
}

func (s *enrSorter) Len() int {
	return len(s.nodes)
}

func (s *enrSorter) Less(i, j int) bool {
	return logDistance(s.nodes[i], s.target) < logDistance(s.nodes[j], s.target)
}

func (s *enrSorter) Swap(i, j int) {
	temp := s.nodes[i]
	s.nodes[i] = s.nodes[j]
	s.nodes[j] = temp
}

// computes the distance between two ENRs defined as
// log_2 (keccak256(n1) XOR keccak256(n2))
func logDistance(n1, n2 *enr.Record) int {
	addr1 := n1.NodeAddr()
	addr2 := n2.NodeAddr()

	var xorResult uint8
	distance := len(addr1) * 8
	for idx := 0; idx < len(addr1); idx++ {
		xorResult = addr1[idx] ^ addr2[idx]
		if xorResult != 0 {
			return distance - bits.LeadingZeros8(xorResult)
		}
		distance -= 8
	}
	return distance
}
