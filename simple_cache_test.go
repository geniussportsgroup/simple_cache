package simple_cache

import (
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/assert"
	"strconv"
	"testing"
	"time"
)

const Capacity = 100
const Factor = 0.2
const TTL = 2 * time.Second

func TestSimpleCache(t *testing.T) {

	ttl := TTL
	fmt.Printf("ttl = %s\n", ttl)

	cache := New(Capacity, Factor, ttl, func(key interface{}) (string, error) {
		return strconv.Itoa(key.(int)), nil
	})

	for i := 0; i < Capacity; i++ {
		entry, err := cache.InsertOrUpdate(i, i)
		assert.Nil(t, err)
		assert.Equal(t, entry.(int), i)
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

	fmt.Printf("Wait for ttl = %s\n", ttl)

	time.Sleep(ttl) // I need to test that TTL works

	value, err = cache.Read(Capacity / 2)
	assert.NotNil(t, err)

	currTime := time.Now()
	for it := cache.NewCacheIt(); it.HasCurr(); it.Next() {
		entry := it.GetCurr()
		assert.True(t, entry.hasExpired(currTime))
	}

	for i := 0; i < Capacity; i++ {
		entry, err := cache.InsertOrUpdate(i, i)
		assert.Nil(t, err)
		assert.Equal(t, entry.(int), i)
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

	elapsedTime := ttl
	fmt.Printf("wait for %s\n", elapsedTime)
	time.Sleep(elapsedTime)

	entry, err := cache.InsertOrUpdate(Capacity, Capacity)
	assert.Nil(t, err)
	assert.Equal(t, entry.(int), Capacity)

	fmt.Printf("wait for %s\n", elapsedTime)
	time.Sleep(elapsedTime) // after elapsing one more half ttl I should be able to insert a new entry

	entry, err = cache.InsertOrUpdate(Capacity, Capacity)
	assert.Nil(t, err)
	assert.Equal(t, entry.(int), Capacity)

	key, mruValue, err = cache.GetMRU()
	assert.Nil(t, err)
	assert.Equal(t, key, strconv.Itoa(Capacity))
	assert.Equal(t, mruValue.(int), Capacity)
}

type ValueType struct {
	Num  int
	Text string
}

func TestCompress(t *testing.T) {

	cache := NewWithCompression(Capacity, Factor, 5*time.Hour,
		func(key interface{}) (string, error) {
			return strconv.Itoa(key.(int)), nil
		}, func(value interface{}) ([]byte, error) {
			content := value.(*ValueType)
			b, err := json.Marshal(content)
			if err != nil {
				return nil, err
			}
			return b, nil
		},
		func(buf []byte) (interface{}, error) {
			value := &ValueType{}
			err := json.Unmarshal(buf, value)
			if err != nil {
				return nil, err
			}
			return value, nil
		})

	for i := 0; i < Capacity; i++ {
		str := fmt.Sprintf("This is the %d-th string", i)
		_, err := cache.InsertOrUpdate(i, &ValueType{
			Num:  i,
			Text: str,
		})
		assert.NoError(t, err)
	}

	for i := 0; i < Capacity; i++ {
		expStr := fmt.Sprintf("This is the %d-th string", i)
		inter, err := cache.Read(i)
		assert.NoError(t, err)
		value := inter.(*ValueType)
		assert.NotNil(t, value)
		assert.Equal(t, expStr, value.Text)
	}
}
