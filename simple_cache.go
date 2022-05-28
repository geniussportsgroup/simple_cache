package simple_cache

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"
)

// State that a cache entry could have
const (
	AVAILABLE = iota
	BUSY
)

type SimpleCacheEntry struct {
	key            string
	value          interface{}
	timestamp      time.Time
	expirationTime time.Time
	prev           *SimpleCacheEntry
	next           *SimpleCacheEntry
	state          int // AVAILABLE or BUSY
}

type SimpleCache struct {
	table map[string]*SimpleCacheEntry

	missCount        int
	hitCount         int
	ttl              time.Duration
	head             SimpleCacheEntry // sentinel header node
	lock             sync.Mutex
	capacity         int
	extendedCapacity int
	numEntries       int

	toMapKey func(key interface{}) (string, error)
}

func (cache *SimpleCache) MissCount() int {
	return cache.missCount
}

func (cache *SimpleCache) HitCount() int {
	return cache.hitCount
}

func (cache *SimpleCache) Ttl() time.Duration {
	return cache.ttl
}

func (cache *SimpleCache) Capacity() int {
	return cache.capacity
}

func (cache *SimpleCache) ExtendedCapacity() int {
	return cache.extendedCapacity
}

func (cache *SimpleCache) NumEntries() int {
	return cache.numEntries
}

// New Creates a new cache. Parameters are:
//
// capacity: maximum number of entries that cache can manage without evicting the least recently used
//
// capFactor is a number in (0.1, 3] that indicates how long the cache should be oversize in order to avoid rehashing
//
// ttl: time to live of a cache entry
//
// toMapKey is a function in charge of transforming the request into a string
//
func New(capacity int, capFactor float64, ttl time.Duration,
	toMapKey func(key interface{}) (string, error)) *SimpleCache {

	if capFactor < 0.1 || capFactor > 3.0 {
		panic(fmt.Sprintf("invalid capFactor %f. It should be in [0.1, 3]",
			capFactor))
	}

	extendedCapacity := math.Ceil((1.0 + capFactor) * float64(capacity))
	ret := &SimpleCache{
		missCount:        0,
		hitCount:         0,
		capacity:         capacity,
		extendedCapacity: int(extendedCapacity),
		numEntries:       0,
		ttl:              ttl,
		table:            make(map[string]*SimpleCacheEntry, int(extendedCapacity)),
		toMapKey:         toMapKey,
	}
	ret.head.prev = &ret.head
	ret.head.next = &ret.head

	return ret
}

func (entry *SimpleCacheEntry) hasExpired(currTime time.Time) bool {
	return entry.expirationTime.Before(currTime)
}

func (cache *SimpleCache) getMRU() *SimpleCacheEntry {

	ret := cache.head.next
	if ret != &cache.head {
		return ret
	}
	return nil
}

func (cache *SimpleCache) getLRU() *SimpleCacheEntry {

	ret := cache.head.prev
	if ret != &cache.head {
		return ret
	}
	return nil
}

func (cache *SimpleCache) isEmpty() bool {

	if cache.numEntries == 0 {
		return false
	}

	return !cache.getMRU().hasExpired(time.Now())
}

// Insert entry as the first item of cache (mru)
func (cache *SimpleCache) insertAsMru(entry *SimpleCacheEntry) {
	entry.prev = &cache.head
	entry.next = cache.head.next
	cache.head.next.prev = entry
	cache.head.next = entry
}

// Auto deletion of lru queue
func (entry *SimpleCacheEntry) selfDeleteFromLRUList() {
	entry.prev.next = entry.next
	entry.next.prev = entry.prev
}

func (cache *SimpleCache) becomeMru(entry *SimpleCacheEntry) {
	entry.selfDeleteFromLRUList()
	cache.insertAsMru(entry)
}

// Rewove the last item in the list (lru); mutex must be taken. The entry becomes AVAILABLE
func (cache *SimpleCache) evictLruEntry() (*SimpleCacheEntry, error) {
	entry := cache.head.prev // <-- LRU entry
	if !entry.hasExpired(time.Now()) && entry.state == BUSY {
		return nil, errors.New("cache is full")
	}
	entry.selfDeleteFromLRUList()
	entry.state = AVAILABLE
	delete(cache.table, entry.key) // Key evicted
	return entry, nil
}

func (cache *SimpleCache) allocateEntry(key string) (entry *SimpleCacheEntry, err error) {

	if cache.numEntries == cache.capacity {
		entry, err = cache.evictLruEntry()
		if err != nil {
			return nil, err
		}
	} else {
		entry = new(SimpleCacheEntry)
		cache.numEntries++
	}

	cache.insertAsMru(entry)
	entry.key = key
	entry.state = BUSY
	cache.table[key] = entry

	return entry, nil
}

// InsertOrUpdate Insert into the cache the pair key,value. If the cache already contains the
// key, then the associated value is updated.
// It could return error if ths stringification of the key fails or if the cache is full
func (cache *SimpleCache) InsertOrUpdate(key interface{}, value interface{}) error {

	stringKey, err := cache.toMapKey(key)
	if err != nil {
		return err
	}

	currTime := time.Now()

	defer cache.lock.Unlock()
	cache.lock.Lock()

	entry := cache.table[stringKey]
	if entry == nil {
		cache.missCount++
		entry, err = cache.allocateEntry(stringKey)
		if err != nil {
			return err
		}
	}

	cache.hitCount++
	entry.value = value
	entry.timestamp = currTime
	entry.expirationTime = currTime.Add(cache.ttl)
	return nil
}

// Read Retrieves the associates value to key. Return error if the key stringification fails,
// the key is not in the cache, or if the key has expired
func (cache *SimpleCache) Read(key interface{}) (interface{}, error) {

	stringKey, err := cache.toMapKey(key)
	if err != nil {
		return nil, err
	}

	currTime := time.Now()

	defer cache.lock.Unlock()
	cache.lock.Lock()

	entry := cache.table[stringKey]
	if entry == nil {
		cache.missCount++
		return nil, fmt.Errorf("stringficated key %s not found", stringKey)
	}

	if entry.hasExpired(currTime) {
		cache.missCount++
		return entry.value, fmt.Errorf("stringficated key %s found but ttl expired", stringKey)
	}

	cache.hitCount++
	entry.expirationTime = currTime.Add(cache.ttl)
	cache.becomeMru(entry)

	return entry.value, nil
}

// GetMRU Return the most recently used entry in the cache. The method do not refresh the entry
func (cache *SimpleCache) GetMRU() (string, interface{}, error) {

	defer cache.lock.Unlock()
	cache.lock.Lock()

	if cache.numEntries == 0 {
		return "", nil, errors.New("empty cache")
	}

	entry := cache.getMRU()
	if entry.hasExpired(time.Now()) || entry.state == AVAILABLE {
		return entry.key, entry.value, errors.New("MRU entry has expired")
	}

	return entry.key, entry.value, nil
}

// SimpleCacheIt Iterator on cache entries. Go from MUR to LRU
type SimpleCacheIt struct {
	cachePtr *SimpleCache
	curr     *SimpleCacheEntry
}

func (cache *SimpleCache) NewCacheIt() *SimpleCacheIt {
	return &SimpleCacheIt{cachePtr: cache, curr: cache.head.next}
}

func (it *SimpleCacheIt) HasCurr() bool {
	return it.curr != &it.cachePtr.head
}

func (it *SimpleCacheIt) GetCurr() *SimpleCacheEntry {
	return it.curr
}

func (it *SimpleCacheIt) Next() *SimpleCacheEntry {
	if !it.HasCurr() {
		return nil
	}
	it.curr = it.curr.next
	return it.curr
}

type CacheState struct {
	MissCount  int
	HitCount   int
	TTL        time.Duration
	Capacity   int
	NumEntries int
}

// GetState Return a json containing the cache state. Use the internal mutex. Be careful with a deadlock
func (cache *SimpleCache) GetState() (string, error) {

	cache.lock.Lock()
	defer cache.lock.Unlock()

	state := CacheState{
		MissCount:  cache.missCount,
		HitCount:   cache.hitCount,
		TTL:        cache.ttl,
		Capacity:   cache.capacity,
		NumEntries: cache.numEntries,
	}

	buf, err := json.MarshalIndent(&state, "", "  ")
	if err != nil {
		return "", err
	}
	return string(buf), nil
}

// helper that does not take lock
func (cache *SimpleCache) clean() error {

	// Now that we know that we can clean safely, we pass again and mark all the entries as AVAILABLE
	for it := cache.NewCacheIt(); it.HasCurr(); it.Next() {
		it.GetCurr().state = AVAILABLE
	}

	// At this point all the entries are marked as AVAILABLE ==> we reset
	cache.numEntries = 0
	cache.hitCount = 0
	cache.missCount = 0

	return nil
}

// Clean Clean the cache. All the entries are deleted and counters reset.
//
// Uses internal lock
//
func (cache *SimpleCache) Clean() error {

	cache.lock.Lock()
	defer cache.lock.Unlock()

	return cache.clean()
}
