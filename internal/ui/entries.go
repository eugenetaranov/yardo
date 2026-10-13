package ui

import "github.com/eugenetaranov/yardo/internal/catalog"

// CatalogEntries are the picker rows of the catalog's visible items
// (Always items are in every plan: nothing to pick).
func CatalogEntries(c *catalog.Catalog) []*Entry {
	var out []*Entry
	for _, it := range c.Items {
		if it.Hidden() || it.Always {
			continue
		}
		out = append(out, &Entry{Tab: it.Tab, ID: it.ID, Label: it.Label, Source: it.Describe()})
	}
	return out
}
