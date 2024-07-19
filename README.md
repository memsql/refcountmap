# refcountmap - a reference counting generic map

[![GoDoc](https://godoc.org/github.com/memsql/refcountmap?status.svg)](https://pkg.go.dev/github.com/memsql/refcountmap)
![unit tests](https://github.com/memsql/refcountmap/actions/workflows/go.yml/badge.svg)
[![report card](https://goreportcard.com/badge/github.com/memsql/refcountmap)](https://goreportcard.com/report/github.com/memsql/refcountmap)
[![codecov](https://codecov.io/gh/memsql/refcountmap/branch/main/graph/badge.svg)](https://codecov.io/gh/memsql/refcountmap)

Install:

	go get github.com/memsql/refcountmap

---

The idea with refcountmap is that the value for each key is a singleton. When you ask for a value
for a specific key, if that key isn't already in the map, then the value is created. If you ask
for that same key again, you get another copy of that value. When you're done with a value, you
release it. When all copies of the value have been released, the key is removed from the map.

The map is thread-safe.

## An example

```go
m := refcountmap.New[string](func() *Thing {
	return &Thing{}
})

thing1 := m.Get("hat")
thing2 := m.Get("hat")

// thing1 and thing2 should be pointers to the same Thing
	
```
