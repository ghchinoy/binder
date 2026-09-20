package okfrules

import "github.com/ghchinoy/binder/pkg/okf"

// NewOrderedMap returns an empty okf.OrderedMap. The constructor is kept internal
// (OQ2 owner ruling): okf.OrderedMap is public, but its constructor is not part
// of the published okf vocabulary. The zero value is already usable (Set lazily
// initializes), so this is a convenience/consistency helper for binder's own
// packages; external callers use &okf.OrderedMap{}.
func NewOrderedMap() *okf.OrderedMap {
	return &okf.OrderedMap{}
}
