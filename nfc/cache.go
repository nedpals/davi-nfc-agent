package nfc

import (
	"sync"
	"time"
)

// TagCache provides thread-safe caching of NFC tag data and tracks the last successful scan.
type TagCache struct {
	lastSeen     map[string]string // map[UID]Text
	lastUID      string            // Most recently scanned valid UID
	lastText     string            // Most recently scanned valid text
	mu           sync.RWMutex
	lastSeenTime time.Time
}

// NewTagCache creates and initializes a new TagCache instance.
func NewTagCache() *TagCache {
	return &TagCache{
		lastSeen:     make(map[string]string),
		lastSeenTime: time.Time{},
	}
}

// GetLastScanned returns the UID and text of the last successfully scanned tag.
func (c *TagCache) GetLastScanned() (string, string) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastUID, c.lastText
}

// HasChanged checks if the given UID and text combination differs from the cached version
// and updates the cache if it has changed. It returns true if the data has changed.
func (c *TagCache) HasChanged(uid, text string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Always update lastSeenTime for any valid card detection
	c.lastSeenTime = time.Now()

	// For factory mode cards, cache UID but no text
	if text == "" {
		c.lastUID = uid
		// Only report change if it's a different card
		_, exists := c.lastSeen[uid]
		return !exists
	}

	lastText, exists := c.lastSeen[uid]
	if !exists || lastText != text {
		c.lastSeen[uid] = text
		c.lastUID = uid
		c.lastText = text
		return true
	}
	return false
}

// Clear removes all entries from the cache and resets the last scanned data.
func (c *TagCache) Clear() {
	c.mu.Lock()
	c.lastSeen = make(map[string]string)
	c.lastUID = ""
	c.lastText = ""
	c.lastSeenTime = time.Time{}
	c.mu.Unlock()
}

// IsCardPresent checks if a card is still present based on the last seen time.
func (c *TagCache) IsCardPresent() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return !c.lastSeenTime.IsZero() && time.Since(c.lastSeenTime) < time.Second
}

// UpdateLastSeenTime updates the global last seen time in the cache,
// indicating recent card activity. The uid parameter is currently
// not used for specific per-tag timestamping with the current cache structure
// but is retained from the original intended signature of ForceSeen.
func (tc *TagCache) UpdateLastSeenTime(uid string) {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	tc.lastSeenTime = time.Now()
}
