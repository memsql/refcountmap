package refcountmap_test

import (
	"fmt"
	"math"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/muir/gwrap"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/memsql/refcountmap"
)

const testCount = 5

func TestNewValueFromKey(t *testing.T) {
	t.Parallel()
	type thingy struct {
		key string
	}
	m := refcountmap.NewValueFromKey(func(k string) thingy {
		return thingy{key: k}
	})
	v, _, _ := m.Get("foo")
	assert.Equal(t, "foo", v.key)
}

func TestRefCountMapNonThreaded(t *testing.T) {
	t.Parallel()
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	for i := 0; i < testCount; i++ {
		testNonThreaded(t, i+1, r.Int63(), 100000, 100)
	}
}

func testNonThreaded(t *testing.T, count int, seed int64, actionCount int, buckets int) {
	type thingy struct {
		gwrap.PQItemEmbed[float64]
		bucket  int
		release func()
		isNew   bool
	}
	t.Run(fmt.Sprintf("iteration%d", count), func(t *testing.T) {
		t.Parallel()
		t.Logf("seed:%d actions:%d buckets: %d", seed, actionCount, buckets)
		var downToZero int
		var upFromZero int
		expectedCounts := make(map[int]int)
		r := rand.New(rand.NewSource(seed))
		q := gwrap.NewPriorityQueue[float64, *thingy]()
		m := refcountmap.New[int](func() *thingy {
			return &thingy{isNew: true}
		})
		for i := 0; i < actionCount; i++ {
			if r.Intn(2) == 0 {
				if q.Len() > 0 {
					item := q.Dequeue()
					require.Greater(t, expectedCounts[item.bucket], 0)
					expectedCounts[item.bucket]--
					item.release()
					if expectedCounts[item.bucket] == 0 {
						downToZero++
					}
				}
				continue
			}
			bucket := int(math.Sqrt(float64(r.Intn(buckets * buckets))))
			item, release, loaded := m.Get(bucket)
			assert.Equal(t, !loaded, item.isNew, "loaded == isNew")
			if item.isNew {
				if _, ok := expectedCounts[bucket]; ok {
					upFromZero++
				}
				assert.Equal(t, 0, expectedCounts[bucket], "new item implies current count is zero")
				item.bucket = bucket
				item.isNew = false
			} else {
				assert.Greater(t, expectedCounts[bucket], 0, "existing item implies current count is not zero")
				assert.Equal(t, bucket, item.bucket, "existing item bucket")
				item = &thingy{
					bucket: bucket,
				}
			}
			item.release = release
			expectedCounts[bucket]++
			q.Enqueue(item, r.Float64())
		}
		t.Log("down to zero", downToZero, "up from zero", upFromZero)
	})
}

const threadCount = 50

const actionCount = 1000

func TestRefCountMapThreaded(t *testing.T) {
	t.Parallel()
	now := time.Now().UnixNano()
	const buckets = 100

	type thingy struct {
		gwrap.PQItemEmbed[float64]
		bucket  int
		release func()
		isNew   bool
		mu      sync.Mutex
	}

	m := refcountmap.New[int](func() *thingy {
		item := &thingy{isNew: true}
		item.mu.Lock()
		return item
	})

	var downToZero int
	var upFromZero int
	expectedCounts := make(map[int]int)
	var metaLock sync.Mutex
	doLocked := func(f func()) {
		metaLock.Lock()
		defer metaLock.Unlock()
		f()
	}

	var wg sync.WaitGroup
	for i := 0; i < threadCount; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := rand.New(rand.NewSource(now + int64(i)))
			q := make([]*thingy, 0, actionCount)
			for j := 0; j < actionCount; j++ {
				if r.Intn(2) == 0 {
					if len(q) > 0 {
						item := q[0]
						q = q[1:]
						doLocked(func() {
							require.Greater(t, expectedCounts[item.bucket], 0)
							expectedCounts[item.bucket]--
							if expectedCounts[item.bucket] == 0 {
								downToZero++
							}
						})
						item.release()
					}
					continue
				}
				bucket := int(math.Sqrt(float64(r.Intn(buckets * buckets))))
				item, release, loaded := m.Get(bucket)
				if loaded {
					item.mu.Lock()
					assert.False(t, item.isNew, "loaded == isNew")
					doLocked(func() {
						assert.GreaterOrEqual(t, expectedCounts[bucket], 0, "existing item implies current count is not zero")
						expectedCounts[bucket]++
					})
					item.mu.Unlock()
					assert.Equal(t, bucket, item.bucket, "existing item bucket")
					item = &thingy{
						bucket:  bucket,
						release: release,
					}
				} else {
					// We know that the item is locked.
					assert.True(t, item.isNew, "loaded == isNew")
					doLocked(func() {
						if _, ok := expectedCounts[bucket]; ok {
							upFromZero++
						}
						assert.Equal(t, 0, expectedCounts[bucket], "new item implies current count is zero")
						expectedCounts[bucket]++
					})
					item.bucket = bucket
					item.isNew = false
					item.release = release
					item.mu.Unlock()
				}
				q = append(q, item)
			}
		}()
	}
	wg.Wait()
	t.Log("down to zero", downToZero, "up from zero", upFromZero)
}
