package session

import (
	"testing"
)

func TestCache_NoEviction(t *testing.T) {
	cache := NewCache()

	e1 := Endpoint{NodeID: 129031, Port: 9999}
	e2 := Endpoint{NodeID: 129032, Port: 9999}
	e3 := Endpoint{NodeID: 129033, Port: 9999}
	cache.StoreSession(e1, "foo1")
	cache.StoreSession(e2, "foo2")
	cache.StoreSession(e3, "foo3")

	if res, found := cache.GetSession(e1); !found || res.(string) != "foo1" {
		t.Error("Expected cache to return 'foo1' for endpoint 1")
	}
	if res, found := cache.GetSession(e2); !found || res.(string) != "foo2" {
		t.Error("Expected cache to return 'foo2' for endpoint 2")
	}
	if res, found := cache.GetSession(e3); !found || res.(string) != "foo3" {
		t.Error("Expected cache to return 'foo3' for endpoint 3")
	}

	randomEndpoint := Endpoint{NodeID: 87654, Port: 1111}

	if _, found := cache.GetSession(randomEndpoint); found {
		t.Error("Expected cache to miss for random endpoint. Got a hit.")
	}
}

func TestCache_WithEvictions(t *testing.T) {
	cache := NewCache()
	cache.size = 3 // override the size of the cache

	e1 := Endpoint{NodeID: 129031, Port: 9999}
	e2 := Endpoint{NodeID: 129032, Port: 9999}
	e3 := Endpoint{NodeID: 129033, Port: 9999}
	e4 := Endpoint{NodeID: 129034, Port: 9999}
	e5 := Endpoint{NodeID: 129035, Port: 9999}

	cache.StoreSession(e1, "foo1")
	cache.StoreSession(e2, "foo2")
	cache.StoreSession(e3, "foo3")
	cache.StoreSession(e4, "foo4")

	// assert 1 got evicted
	if _, found := cache.GetSession(e1); found {
		t.Error("Expected cache to have evicted endpoint 1. Got a hit.")
	}
	// access endpoint 2, making 3 the oldest so it is evicted
	cache.GetSession(e2)
	cache.StoreSession(e5, "foo5")
	if res, found := cache.GetSession(e2); !found || res.(string) != "foo2" {
		t.Error("Expected cache to return 'foo2' for endpoint 2")
	}
	if _, found := cache.GetSession(e3); found {
		t.Error("Expected cache to have evicted endpoint 3. Got a hit.")
	}
}
