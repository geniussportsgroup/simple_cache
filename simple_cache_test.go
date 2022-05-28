package simple_cache

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"strconv"
	"testing"
	"time"
)

const Capacity = 100
const Factor = 0.2
const TTL = 2 // in seconds

func TestSimpleCache(t *testing.T) {

	ttl, _ := time.ParseDuration(fmt.Sprintf("%ds", TTL))

	cache := New(Capacity, Factor, ttl, func(key interface{}) (string, error) {
		return strconv.Itoa(key.(int)), nil
	})

	for i := 0; i < Capacity; i++ {
		err := cache.InsertOrUpdate(i, i)
		assert.Nil(t, err)
	}

	for it := cache.NewCacheIt(); it.HasCurr(); it.Next() {
		entry := it.GetCurr()
		assert.Equal(t, entry.state, BUSY)
		assert.False(t, entry.hasExpired(time.Now()))
	}

	assert.Equal(t, cache.NumEntries(), Capacity)

	key, mruValue, err := cache.GetMRU()
	assert.Nil(t, err)
	assert.Equal(t, key, strconv.Itoa(Capacity-1))
	assert.Equal(t, mruValue.(int), 99)

	value, err := cache.Read(Capacity / 2)
	assert.Nil(t, err)
	assert.Equal(t, value.(int), Capacity/2)

	key, mruValue, err = cache.GetMRU()
	assert.Nil(t, err)
	assert.Equal(t, key, strconv.Itoa(Capacity/2))
	assert.Equal(t, mruValue.(int), Capacity/2)

	time.Sleep(ttl) // I need to test that TTL works

	value, err = cache.Read(Capacity / 2)
	assert.NotNil(t, err)

	currTime := time.Now()
	for it := cache.NewCacheIt(); it.HasCurr(); it.Next() {
		entry := it.GetCurr()
		assert.True(t, entry.hasExpired(currTime))
	}

	for i := 0; i < Capacity; i++ {
		err = cache.InsertOrUpdate(i, i)
		assert.Nil(t, err)
	}

	value, err = cache.Read(Capacity - 1)
	assert.Equal(t, value.(int), 99)
	assert.Nil(t, err)

	key, mruValue, err = cache.GetMRU()
	assert.Nil(t, err)
	assert.Equal(t, key, strconv.Itoa(Capacity-1))
	assert.Equal(t, mruValue.(int), 99)

	for it := cache.NewCacheIt(); it.HasCurr(); it.Next() {
		entry := it.GetCurr()
		assert.Equal(t, entry.state, BUSY)
		assert.False(t, entry.hasExpired(time.Now()))
	}

	assert.Equal(t, cache.NumEntries(), Capacity)

	halfTtl, err := time.ParseDuration(fmt.Sprintf("%ds", TTL/2))
	time.Sleep(halfTtl)

	err = cache.InsertOrUpdate(Capacity, Capacity)
	assert.NotNil(t, err)

	time.Sleep(halfTtl) // after elapsing one more half ttl I should be able to insert a new entry

	err = cache.InsertOrUpdate(Capacity, Capacity)
	assert.Nil(t, err)

	key, mruValue, err = cache.GetMRU()
	assert.Nil(t, err)
	assert.Equal(t, key, strconv.Itoa(Capacity))
	assert.Equal(t, mruValue.(int), Capacity)
}
