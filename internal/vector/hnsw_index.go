package vector

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

// hnswIndex is a Hierarchical Navigable Small World graph (Malkov and Yashunin, 2016) over cosine distance.
//
// It replaces github.com/coder/hnsw, whose search stops as soon as a step finds nothing closer ("no improvement in distance"),
// so that efSearch only bounds a queue and the search is greedy: measured against an exact search, it found 8% of the true ten
// nearest neighbours of 2,000 random vectors and 2% of 20,000, whatever efSearch (20 to 200) and M (16 or 32). The search here
// is the one of the paper: it goes on until no candidate is closer than the worst of the efSearch best found. It is also plain
// Go with no dependency, so the store works on Windows.
//
// A removed or replaced vector stays in the graph as a tombstone (the graph can still route through it) and is left out of the
// results; the graph is rebuilt when more than half of it is tombstones. It is not safe for concurrent writes; searches may run
// together (the store holds a read lock).
type hnswIndex struct {
	dim            int
	m              int // links of a node on a layer above the base
	mMax0          int // links of a node on the base layer
	efConstruction int
	efSearch       int
	levelMult      float64

	nodes    []hnswNode
	byKey    map[string]int32 // live nodes only
	entry    int32            // -1 when empty
	maxLevel int
	rng      *rand.Rand

	visitedPool sync.Pool
}

type hnswNode struct {
	key     string
	vec     []float32
	invNorm float32 // 1 / |vec|, 0 for a zero vector
	links   [][]int32
	deleted bool
}

type hnswCandidate struct {
	id   int32
	dist float32
}

const (
	hnswDefaultM              = 16
	hnswDefaultEfConstruction = 200
	hnswDefaultEfSearch       = 64
	hnswMagic                 = "SHNSW1\n"
)

func newHNSWIndex() *hnswIndex {
	x := &hnswIndex{
		m:              hnswDefaultM,
		mMax0:          2 * hnswDefaultM,
		efConstruction: hnswDefaultEfConstruction,
		efSearch:       hnswDefaultEfSearch,
		levelMult:      1 / math.Log(hnswDefaultM),
		byKey:          make(map[string]int32),
		entry:          -1,
		rng:            rand.New(rand.NewSource(1)), //nolint:gosec // levels need a repeatable spread, not secrecy
	}
	x.visitedPool.New = func() any { return &hnswVisited{} }
	return x
}

// Len is the number of live vectors.
func (x *hnswIndex) Len() int { return len(x.byKey) }

// Has reports whether a key is in the index.
func (x *hnswIndex) Has(key string) bool { _, ok := x.byKey[key]; return ok }

// hnswVisited marks the nodes a search has seen, without clearing an array for each search: a node is seen when its mark is the
// current epoch.
type hnswVisited struct {
	marks []uint32
	epoch uint32
}

func (v *hnswVisited) reset(n int) {
	if len(v.marks) < n {
		v.marks = make([]uint32, n+n/4+16)
		v.epoch = 0
	}
	v.epoch++
	if v.epoch == 0 { // wrapped around
		for i := range v.marks {
			v.marks[i] = 0
		}
		v.epoch = 1
	}
}

func (v *hnswVisited) seen(id int32) bool { return v.marks[id] == v.epoch }
func (v *hnswVisited) mark(id int32)      { v.marks[id] = v.epoch }

func invNorm(vec []float32) float32 {
	var sum float64
	for _, x := range vec {
		sum += float64(x) * float64(x)
	}
	if sum == 0 {
		return 0
	}
	return float32(1 / math.Sqrt(sum))
}

func dot(a, b []float32) float32 {
	b = b[:len(a)]
	var s0, s1, s2, s3 float32
	i := 0
	for ; i+4 <= len(a); i += 4 {
		s0 += a[i] * b[i]
		s1 += a[i+1] * b[i+1]
		s2 += a[i+2] * b[i+2]
		s3 += a[i+3] * b[i+3]
	}
	for ; i < len(a); i++ {
		s0 += a[i] * b[i]
	}
	return s0 + s1 + s2 + s3
}

// distTo is the cosine distance (0 identical, 1 orthogonal, 2 opposite) between a query and a node.
func (x *hnswIndex) distTo(q []float32, qInv float32, id int32) float32 {
	n := &x.nodes[id]
	return 1 - dot(q, n.vec)*qInv*n.invNorm
}

func (x *hnswIndex) distNodes(a, b int32) float32 {
	na, nb := &x.nodes[a], &x.nodes[b]
	return 1 - dot(na.vec, nb.vec)*na.invNorm*nb.invNorm
}

// ---- heaps ------------------------------------------------------------------------------------------------------------
//
// Two binary heaps of candidates, written out (container/heap boxes every value it moves, and a search moves thousands).

// minHeap keeps the closest candidate on top.
type minHeap []hnswCandidate

func (h *minHeap) push(c hnswCandidate) {
	*h = append(*h, c)
	s := *h
	i := len(s) - 1
	for i > 0 {
		parent := (i - 1) / 2
		if s[parent].dist <= s[i].dist {
			break
		}
		s[parent], s[i] = s[i], s[parent]
		i = parent
	}
}

func (h *minHeap) pop() hnswCandidate {
	s := *h
	top := s[0]
	last := len(s) - 1
	s[0] = s[last]
	s = s[:last]
	*h = s
	i := 0
	for {
		l, r, small := 2*i+1, 2*i+2, i
		if l < len(s) && s[l].dist < s[small].dist {
			small = l
		}
		if r < len(s) && s[r].dist < s[small].dist {
			small = r
		}
		if small == i {
			break
		}
		s[i], s[small] = s[small], s[i]
		i = small
	}
	return top
}

// maxHeap keeps the farthest candidate on top.
type maxHeap []hnswCandidate

func (h *maxHeap) push(c hnswCandidate) {
	*h = append(*h, c)
	s := *h
	i := len(s) - 1
	for i > 0 {
		parent := (i - 1) / 2
		if s[parent].dist >= s[i].dist {
			break
		}
		s[parent], s[i] = s[i], s[parent]
		i = parent
	}
}

func (h *maxHeap) pop() hnswCandidate {
	s := *h
	top := s[0]
	last := len(s) - 1
	s[0] = s[last]
	s = s[:last]
	*h = s
	i := 0
	for {
		l, r, large := 2*i+1, 2*i+2, i
		if l < len(s) && s[l].dist > s[large].dist {
			large = l
		}
		if r < len(s) && s[r].dist > s[large].dist {
			large = r
		}
		if large == i {
			break
		}
		s[i], s[large] = s[large], s[i]
		i = large
	}
	return top
}

// searchLayer is the search of one layer: from the entry points, it follows the links towards the query and keeps the ef
// closest nodes seen; it stops when the closest candidate left is farther than the worst of them. Best first.
func (x *hnswIndex) searchLayer(q []float32, qInv float32, entries []hnswCandidate, ef, level int, vis *hnswVisited) []hnswCandidate {
	candidates := make(minHeap, 0, ef)
	results := make(maxHeap, 0, ef+1)
	for _, e := range entries {
		vis.mark(e.id)
		candidates.push(e)
		results.push(e)
		if len(results) > ef {
			results.pop()
		}
	}
	for len(candidates) > 0 {
		c := candidates.pop()
		if len(results) >= ef && c.dist > results[0].dist {
			break
		}
		node := &x.nodes[c.id]
		if level >= len(node.links) {
			continue
		}
		for _, nb := range node.links[level] {
			if vis.seen(nb) {
				continue
			}
			vis.mark(nb)
			d := x.distTo(q, qInv, nb)
			if len(results) < ef || d < results[0].dist {
				candidates.push(hnswCandidate{nb, d})
				results.push(hnswCandidate{nb, d})
				if len(results) > ef {
					results.pop()
				}
			}
		}
	}
	out := make([]hnswCandidate, len(results))
	copy(out, results)
	sort.Slice(out, func(i, j int) bool {
		if out[i].dist != out[j].dist {
			return out[i].dist < out[j].dist
		}
		return out[i].id < out[j].id
	})
	return out
}

// selectNeighbors picks at most m links for a node from candidates (best first, distances to that node): a candidate is kept
// when it is closer to the node than to any neighbour already kept, which spreads the links over the directions around the
// node instead of piling them on one cluster; if that leaves fewer than m, the nearest rejected ones fill up.
func (x *hnswIndex) selectNeighbors(candidates []hnswCandidate, m int) []int32 {
	if len(candidates) <= m {
		ids := make([]int32, len(candidates))
		for i, c := range candidates {
			ids[i] = c.id
		}
		return ids
	}
	kept := make([]hnswCandidate, 0, m)
	var rejected []hnswCandidate
	for _, c := range candidates {
		if len(kept) >= m {
			break
		}
		good := true
		for _, k := range kept {
			if x.distNodes(c.id, k.id) < c.dist {
				good = false
				break
			}
		}
		if good {
			kept = append(kept, c)
		} else {
			rejected = append(rejected, c)
		}
	}
	for _, c := range rejected {
		if len(kept) >= m {
			break
		}
		kept = append(kept, c)
	}
	ids := make([]int32, len(kept))
	for i, c := range kept {
		ids[i] = c.id
	}
	return ids
}

func (x *hnswIndex) randomLevel() int {
	u := 1 - x.rng.Float64() // (0, 1]
	return int(-math.Log(u) * x.levelMult)
}

// Add inserts a vector under a key; a key that is already there is replaced.
func (x *hnswIndex) Add(key string, vec []float32) error {
	if len(vec) == 0 {
		return errors.New("hnsw: empty vector")
	}
	if x.dim == 0 {
		x.dim = len(vec)
	} else if len(vec) != x.dim {
		return fmt.Errorf("hnsw: vector of %d dimensions in an index of %d", len(vec), x.dim)
	}
	if old, ok := x.byKey[key]; ok {
		x.nodes[old].deleted = true
		delete(x.byKey, key)
	}
	x.insert(key, vec)
	x.compactIfNeeded()
	return nil
}

func (x *hnswIndex) insert(key string, vec []float32) {
	id := int32(len(x.nodes))
	level := x.randomLevel()
	node := hnswNode{key: key, vec: append([]float32(nil), vec...), links: make([][]int32, level+1)}
	node.invNorm = invNorm(node.vec)
	x.nodes = append(x.nodes, node)
	x.byKey[key] = id
	if x.entry < 0 {
		x.entry, x.maxLevel = id, level
		return
	}

	vis := x.visitedPool.Get().(*hnswVisited)
	defer x.visitedPool.Put(vis)
	q, qInv := x.nodes[id].vec, x.nodes[id].invNorm
	eps := []hnswCandidate{{x.entry, x.distTo(q, qInv, x.entry)}}
	for l := x.maxLevel; l > level; l-- {
		vis.reset(len(x.nodes))
		eps = x.searchLayer(q, qInv, eps, 1, l, vis)[:1]
	}
	for l := min(level, x.maxLevel); l >= 0; l-- {
		vis.reset(len(x.nodes))
		found := x.searchLayer(q, qInv, eps, x.efConstruction, l, vis)
		// the node itself is in the graph already (its slot exists) but has no links, so no search reaches it
		neighbors := x.selectNeighbors(found, x.m)
		x.nodes[id].links[l] = neighbors
		maxLinks := x.m
		if l == 0 {
			maxLinks = x.mMax0
		}
		for _, nb := range neighbors {
			x.link(nb, id, l, maxLinks)
		}
		eps = found
	}
	if level > x.maxLevel {
		x.entry, x.maxLevel = id, level
	}
}

// link adds a link from -> to on a layer; a node over its limit keeps the best spread of links, chosen the same way as for a
// new node.
func (x *hnswIndex) link(from, to int32, level, maxLinks int) {
	n := &x.nodes[from]
	if level >= len(n.links) {
		return
	}
	n.links[level] = append(n.links[level], to)
	if len(n.links[level]) <= maxLinks {
		return
	}
	cands := make([]hnswCandidate, len(n.links[level]))
	for i, id := range n.links[level] {
		cands[i] = hnswCandidate{id, x.distNodes(from, id)}
	}
	sort.Slice(cands, func(i, j int) bool {
		if cands[i].dist != cands[j].dist {
			return cands[i].dist < cands[j].dist
		}
		return cands[i].id < cands[j].id
	})
	n.links[level] = x.selectNeighbors(cands, maxLinks)
}

// Delete removes a key; it reports whether the key was there.
func (x *hnswIndex) Delete(key string) bool {
	id, ok := x.byKey[key]
	if !ok {
		return false
	}
	x.nodes[id].deleted = true
	delete(x.byKey, key)
	x.compactIfNeeded()
	return true
}

// compactIfNeeded rebuilds the graph without its tombstones when they are more than half of it.
func (x *hnswIndex) compactIfNeeded() {
	dead := len(x.nodes) - len(x.byKey)
	if dead < 64 || dead <= len(x.nodes)/2 {
		return
	}
	old := x.nodes
	x.nodes = nil
	x.byKey = make(map[string]int32, len(old)-dead)
	x.entry, x.maxLevel = -1, 0
	for i := range old {
		if !old[i].deleted {
			x.insert(old[i].key, old[i].vec)
		}
	}
}

// Vector returns the vector stored under a key.
func (x *hnswIndex) Vector(key string) ([]float32, bool) {
	id, ok := x.byKey[key]
	if !ok {
		return nil, false
	}
	return x.nodes[id].vec, true
}

// hnswHit is a result of Search.
type hnswHit struct {
	Key string
	Vec []float32
	// Dist is the cosine distance to the query.
	Dist float32
}

// Search returns the k live vectors closest to the query, closest first. efSearch (the size of the list kept while searching,
// at least k) trades time for recall.
func (x *hnswIndex) Search(q []float32, k, efSearch int) ([]hnswHit, error) {
	if x.entry < 0 || k <= 0 {
		return nil, nil
	}
	if len(q) != x.dim {
		return nil, fmt.Errorf("hnsw: query of %d dimensions in an index of %d", len(q), x.dim)
	}
	if efSearch <= 0 {
		efSearch = x.efSearch
	}
	// tombstones take room in the list without being results
	ef := max(efSearch, k) + (len(x.nodes) - len(x.byKey))
	ef = min(ef, len(x.nodes))
	vis := x.visitedPool.Get().(*hnswVisited)
	defer x.visitedPool.Put(vis)
	qInv := invNorm(q)
	eps := []hnswCandidate{{x.entry, x.distTo(q, qInv, x.entry)}}
	for l := x.maxLevel; l > 0; l-- {
		vis.reset(len(x.nodes))
		eps = x.searchLayer(q, qInv, eps, 1, l, vis)[:1]
	}
	vis.reset(len(x.nodes))
	found := x.searchLayer(q, qInv, eps, ef, 0, vis)
	hits := make([]hnswHit, 0, k)
	for _, c := range found {
		n := &x.nodes[c.id]
		if n.deleted {
			continue
		}
		hits = append(hits, hnswHit{Key: n.key, Vec: n.vec, Dist: c.dist})
		if len(hits) == k {
			break
		}
	}
	return hits, nil
}

// ---- persistence ------------------------------------------------------------------------------------------------------

// Save writes the index to path, through a temporary file renamed over it, so that a crash leaves the old file.
func (x *hnswIndex) Save(path string) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	w := bufio.NewWriterSize(tmp, 1<<20)
	if err := x.writeTo(w); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := w.Flush(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func (x *hnswIndex) writeTo(w io.Writer) error {
	var buf [4]byte
	u32 := func(v uint32) error {
		binary.LittleEndian.PutUint32(buf[:], v)
		_, err := w.Write(buf[:])
		return err
	}
	if _, err := io.WriteString(w, hnswMagic); err != nil {
		return err
	}
	for _, v := range []uint32{uint32(x.dim), uint32(x.m), uint32(x.efConstruction), uint32(x.efSearch), uint32(x.entry), uint32(x.maxLevel), uint32(len(x.nodes))} {
		if err := u32(v); err != nil {
			return err
		}
	}
	for i := range x.nodes {
		n := &x.nodes[i]
		if err := u32(uint32(len(n.key))); err != nil {
			return err
		}
		if _, err := io.WriteString(w, n.key); err != nil {
			return err
		}
		flag := byte(0)
		if n.deleted {
			flag = 1
		}
		if _, err := w.Write([]byte{flag}); err != nil {
			return err
		}
		for _, f := range n.vec {
			if err := u32(math.Float32bits(f)); err != nil {
				return err
			}
		}
		if err := u32(uint32(len(n.links))); err != nil {
			return err
		}
		for _, level := range n.links {
			if err := u32(uint32(len(level))); err != nil {
				return err
			}
			for _, id := range level {
				if err := u32(uint32(id)); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// errNotHNSWIndex is returned by loadHNSWIndex for a file that does not start with the magic of this format.
var errNotHNSWIndex = errors.New("not an hnsw index file of this format")

// loadHNSWIndex reads an index written by Save. Every count read is checked against what the file can hold, so that a damaged
// file is an error and not an allocation of gigabytes.
func loadHNSWIndex(path string) (*hnswIndex, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	size := uint64(info.Size())
	r := bufio.NewReaderSize(f, 1<<20)
	magic := make([]byte, len(hnswMagic))
	if _, err := io.ReadFull(r, magic); err != nil || string(magic) != hnswMagic {
		return nil, errNotHNSWIndex
	}
	var buf [4]byte
	var readErr error
	u32 := func() uint32 {
		if readErr != nil {
			return 0
		}
		if _, err := io.ReadFull(r, buf[:]); err != nil {
			readErr = err
			return 0
		}
		return binary.LittleEndian.Uint32(buf[:])
	}
	x := newHNSWIndex()
	x.dim = int(u32())
	x.m = int(u32())
	x.efConstruction = int(u32())
	x.efSearch = int(u32())
	x.entry = int32(u32())
	x.maxLevel = int(u32())
	count := uint64(u32())
	if readErr != nil {
		return nil, fmt.Errorf("read hnsw header: %w", readErr)
	}
	if x.dim <= 0 || x.dim > 1<<16 || x.m <= 0 || x.m > 1024 || count > size/9 || x.maxLevel > 64 {
		return nil, fmt.Errorf("hnsw index file is damaged (dim %d, m %d, nodes %d, levels %d)", x.dim, x.m, count, x.maxLevel)
	}
	x.mMax0 = 2 * x.m
	x.levelMult = 1 / math.Log(float64(x.m))
	x.nodes = make([]hnswNode, count)
	for i := range x.nodes {
		n := &x.nodes[i]
		keyLen := uint64(u32())
		if readErr != nil || keyLen > size {
			return nil, fmt.Errorf("hnsw index file is damaged at node %d", i)
		}
		key := make([]byte, keyLen)
		if _, err := io.ReadFull(r, key); err != nil {
			return nil, fmt.Errorf("read hnsw node %d: %w", i, err)
		}
		n.key = string(key)
		flag, err := r.ReadByte()
		if err != nil {
			return nil, fmt.Errorf("read hnsw node %d: %w", i, err)
		}
		n.deleted = flag == 1
		n.vec = make([]float32, x.dim)
		for j := range n.vec {
			n.vec[j] = math.Float32frombits(u32())
		}
		n.invNorm = invNorm(n.vec)
		levels := uint64(u32())
		if readErr != nil || levels > 65 {
			return nil, fmt.Errorf("hnsw index file is damaged at node %d", i)
		}
		n.links = make([][]int32, levels)
		for l := range n.links {
			c := uint64(u32())
			if readErr != nil || c > count {
				return nil, fmt.Errorf("hnsw index file is damaged at node %d", i)
			}
			n.links[l] = make([]int32, c)
			for k := range n.links[l] {
				id := u32()
				if uint64(id) >= count {
					return nil, fmt.Errorf("hnsw index file is damaged at node %d", i)
				}
				n.links[l][k] = int32(id)
			}
		}
		if readErr != nil {
			return nil, fmt.Errorf("read hnsw node %d: %w", i, readErr)
		}
		if !n.deleted {
			x.byKey[n.key] = int32(i)
		}
	}
	if count > 0 && (x.entry < 0 || uint64(x.entry) >= count) {
		return nil, errors.New("hnsw index file is damaged (entry point)")
	}
	if count == 0 {
		x.entry = -1
	}
	return x, nil
}
