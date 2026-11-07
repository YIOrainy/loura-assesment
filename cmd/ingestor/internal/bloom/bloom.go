package bloom

import (
	"hash/fnv"
	"math"
	"sync"
)

type Filter struct {
	mu   sync.RWMutex
	bits []uint64
	m    uint64
	k    uint64
}

func New(n uint, p float64) *Filter {
	m := uint64(math.Ceil(-float64(n) * math.Log(p) / (math.Ln2 * math.Ln2)))
	k := uint64(math.Max(1, math.Round(float64(m)/float64(n)*math.Ln2)))
	return &Filter{bits: make([]uint64, (m+63)/64), m: m, k: k}
}

func (f *Filter) Add(key string) {
	h1, h2 := hashes(key)
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := uint64(0); i < f.k; i++ {
		idx := (h1 + i*h2) % f.m
		f.bits[idx/64] |= 1 << (idx % 64)
	}
}

func (f *Filter) Test(key string) bool {
	h1, h2 := hashes(key)
	f.mu.RLock()
	defer f.mu.RUnlock()
	for i := uint64(0); i < f.k; i++ {
		idx := (h1 + i*h2) % f.m
		if f.bits[idx/64]&(1<<(idx%64)) == 0 {
			return false
		}
	}
	return true
}

func hashes(key string) (uint64, uint64) {
	a := fnv.New64a()
	a.Write([]byte(key))
	b := fnv.New64()
	b.Write([]byte(key))
	return a.Sum64(), b.Sum64() | 1
}
