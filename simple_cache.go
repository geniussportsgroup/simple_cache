package simple_cache

import (
	"sync"
	"time"
)

type SimpleCacheEntry struct {
	key            string
	timestamp      time.Time
	expirationTime time.Time
	prev           *SimpleCacheEntry
	next           *SimpleCacheEntry
}

type SimpleCache struct {
	table            map[string]*SimpleCacheEntry
	missCount        int
	hitCount         int
	ttl              time.Duration
	head             SimpleCacheEntry // sentinel header node
	lock             sync.Mutex
	capacity         int
	extendedCapacity int
	numEntries       int
}
