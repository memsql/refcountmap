package refcountmap

import (
	"sync"

	"github.com/muir/gwrap"
)

type counter[K comparable, V any] struct {
	lock       sync.Mutex
	value      V
	references int
	k          K
	m          *Map[K, V]
}

// Map is a thread-safe map that with reference-counted values.
// Values are obtained with Get() and discarded when no longer needed.
// New values are created as needed.
type Map[K comparable, V any] struct {
	gwrap.SyncMap[K, *counter[K, V]]
	makeNew func() V
}

// New will use the newValue function to create
// new values when one does not already exist. Some return
// values from newValue() may be discarded without ever being
// returned.
//
// Usually V should be a pointer. The type of V can be derived
// from the function, but the type of K cannot so the usuall
// invocation is like
//
//	m := refcountmap.New[keyType](func() valueType { return &valueType{} })
func New[K comparable, V any](newValue func() V) *Map[K, V] {
	return &Map[K, V]{
		makeNew: newValue,
	}
}

// return true if allocation is successful
func (v *counter[K, V]) allocate() bool {
	v.lock.Lock()
	defer v.lock.Unlock()
	if v.references <= 0 {
		return false
	}
	v.references++
	return true
}

// Load returns the current value for the key. It will not create
// a value if one does not exist. If loaded is false, the value
// is invalid. Load does not increase the reference count on
// the value returned.
func (m *Map[K, V]) Load(k K) (value V, loaded bool) {
	v, ok := m.SyncMap.Load(k)
	if !ok {
		var value V
		return value, false
	}
	v.lock.Lock()
	defer v.lock.Unlock()
	return v.value, v.references > 0
}

// Get either returns an existing value for the given key (if there
// is an existing value) or it creates a new value and returns it.
//
// The release() function that must be called when you
// are done with the returned value:
//
//	item, release, loaded := m.Get("key")
//	defer release()
func (m *Map[K, V]) Get(k K) (value V, release func(), loaded bool) {
	v, ok := m.SyncMap.Load(k)
	if ok {
		if v.allocate() {
			return v.value, v.release, true
		}
	}
	newV := &counter[K, V]{
		k:          k,
		value:      m.makeNew(),
		m:          m,
		references: 1, // allocate() not called on this counter
	}
	// This can loop because the loaded counter could be locked and in
	// the process of deleting itself. This is a spin loop but should be
	// very fast because it's waiting on release() which is running at
	// the same time and allocate() will block until the release is
	// complete.
	//
	// There is no guarantee of only going through the loop once -- there
	// could be another counter allocated and released before the loop
	// comes around again.
	for {
		v, loaded = m.SyncMap.LoadOrStore(k, newV)
		if loaded && !v.allocate() {
			continue
		}
		return v.value, v.release, loaded
	}
}

func (v *counter[K, V]) release() {
	v.lock.Lock()
	defer v.lock.Unlock()
	v.references--
	if v.references <= 0 {
		v.m.Delete(v.k)
	}
}

// Range is like [sync.Map.Range](). It iterates over the
// keys in the map. There is no guarantee that a key won't be removed
// during iteration. The reference count for the returned key is
// not incremented by Range.
//
// [sync.Map.Range]: https://pkg.go.dev/sync#Map.Range
func (m *Map[K, V]) Range(f func(K, V) bool) {
	m.SyncMap.Range(func(k K, v *counter[K, V]) bool {
		return f(k, v.value)
	})
}
