package main

import (
	"encoding/json"
	"sync"
	"time"
)

type CacheEntry struct {
	Data      json.RawMessage
	UpdatedAt time.Time
	TTL       time.Duration
}

type RoomCache struct {
	mu      sync.RWMutex
	entries map[string]*CacheEntry
}

func NewRoomCache() *RoomCache {
	return &RoomCache{
		entries: make(map[string]*CacheEntry),
	}
}

func (rc *RoomCache) Set(key string, data json.RawMessage, ttl time.Duration) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.entries[key] = &CacheEntry{
		Data:      data,
		UpdatedAt: time.Now(),
		TTL:       ttl,
	}
}

func (rc *RoomCache) Get(key string) (json.RawMessage, bool) {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	entry, exists := rc.entries[key]
	if !exists {
		return nil, false
	}
	
	// Check if expired
	if time.Since(entry.UpdatedAt) > entry.TTL {
		delete(rc.entries, key)
		return nil, false
	}
	
	return entry.Data, true
}
