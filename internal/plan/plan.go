// Package plan turns picks into the ordered list of items to install:
// every pick, everything it needs (mise for mise items), in catalog order
// with each item after what it needs; the install items first, then the
// configure items (logins, keys, settings), then the ones that go last.
package plan

import (
	"fmt"

	"github.com/eugenetaranov/yardo/internal/catalog"
)

// MiseID is the hidden item that installs mise itself; MiseBuildID the
// build tools that extra mise tools without prebuilt binaries compile with.
const (
	MiseID      = "mise"
	MiseBuildID = "misebuild"
)

// Resolve returns picks plus everything they need, and the Always items,
// in install order. A
// pick that isn't in the catalog is an error. extraMise says whether
// extra mise tools are to be installed (which needs mise too).
func Resolve(c *catalog.Catalog, picks []string, extraMise bool) ([]*catalog.Item, error) {
	want := map[string]bool{}
	var add func(id string) error
	add = func(id string) error {
		if want[id] {
			return nil
		}
		it := c.Item(id)
		if it == nil {
			return fmt.Errorf("%q is not in the catalog", id)
		}
		want[id] = true
		for _, n := range it.Needs {
			if err := add(n); err != nil {
				return err
			}
		}
		if it.UsesMise() && c.Item(MiseID) != nil {
			return add(MiseID)
		}
		return nil
	}
	for _, p := range picks {
		if err := add(p); err != nil {
			return nil, err
		}
	}
	// The baseline: in every plan that installs anything.
	if len(want) > 0 || extraMise {
		for _, it := range c.Items {
			if it.Always {
				if err := add(it.ID); err != nil {
					return nil, err
				}
			}
		}
	}
	if extraMise {
		for _, id := range []string{MiseID, MiseBuildID} {
			if c.Item(id) != nil {
				if err := add(id); err != nil {
					return nil, err
				}
			}
		}
	}

	// Catalog order, except that an item goes after everything it needs
	// (the catalog lists tabs in picker order, not install order).
	var out []*catalog.Item
	placed := map[string]bool{}
	var place func(it *catalog.Item)
	place = func(it *catalog.Item) {
		if placed[it.ID] {
			return
		}
		placed[it.ID] = true
		for _, n := range it.Needs {
			place(c.Item(n))
		}
		for _, n := range it.After {
			if want[n] {
				place(c.Item(n))
			}
		}
		if it.UsesMise() && it.ID != MiseID && c.Item(MiseID) != nil {
			place(c.Item(MiseID))
		}
		out = append(out, it)
	}
	// Installing first: hidden helpers when wanted (as the bash installer
	// did), then the rest in catalog order. The catalog guarantees an
	// install item needs no configure item, so this places none.
	for _, it := range c.Items {
		if want[it.ID] && it.Hidden() && !it.Configure() {
			place(it)
		}
	}
	for _, it := range c.Items {
		if want[it.ID] && !it.Configure() && !it.Last {
			place(it)
		}
	}
	// Then configuring: logins, keys, settings (and their hidden helpers,
	// through needs).
	for _, it := range c.Items {
		if want[it.ID] && it.Configure() && !it.Last {
			place(it)
		}
	}
	// Then the items that go last (dotfiles: what they write wins), even
	// after catalog.d's.
	for _, it := range c.Items {
		if want[it.ID] {
			place(it)
		}
	}
	return out, nil
}

// Configuring returns the index of the first configure item (or Last
// item) in a resolved plan: where installing ends. len(items) if none.
func Configuring(items []*catalog.Item) int {
	for i, it := range items {
		if it.Configure() || it.Last {
			return i
		}
	}
	return len(items)
}

// IDs returns the items' ids.
func IDs(items []*catalog.Item) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.ID
	}
	return out
}
