package classifier

import (
	"fmt"
	"hash/fnv"
	"strings"
)

// GenerateStablePoemID generates a stable numeric ID based on poem content
// This ensures the same poem always gets the same ID, even across re-imports
func GenerateStablePoemID(title, author string, paragraphs []string) int64 {
	// Combine key information to create a unique identifier
	key := fmt.Sprintf("%s|%s|%s",
		strings.TrimSpace(title),
		strings.TrimSpace(author),
		strings.Join(paragraphs, "|"),
	)

	// Use FNV-1a hash for fast, stable hashing
	h := fnv.New64a()
	h.Write([]byte(key))

	// Return positive int64 (clear the sign bit)
	return int64(h.Sum64() & 0x7FFFFFFFFFFFFFFF)
}

// GenerateStableAuthorID generates a stable 6-digit ID based on author name
// This ensures the same author always gets the same ID, even across re-imports
// Returns a 6-digit number (100000-999999) for consistency and readability
func GenerateStableAuthorID(name string) int64 {
	// Normalize the name
	key := strings.TrimSpace(name)

	// Use FNV-1a hash
	h := fnv.New64a()
	h.Write([]byte(key))

	// Get hash value
	hash := h.Sum64()

	// Map to 6-digit range (100000-999999)
	// This gives us 900,000 possible IDs
	id := int64((hash % 900000) + 100000)

	return id
}
