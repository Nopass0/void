// Package index re-exports storage.BloomFilter under the index package name
// so upper layers can reference index.BloomFilter without importing storage.
// The actual implementation lives in internal/engine/storage/bloom.go.
package index
