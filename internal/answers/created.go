package answers

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
)

// Created is one thing yardo created, for `yardo forget`
// (created.json, shared with picker.py). Kinds: gh_key, gh_login,
// registry, file, block, tailscale. Never a secret.
type Created struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

// ReadCreated returns state's created.json, in creation order.
func ReadCreated(state string) []Created {
	b, err := os.ReadFile(filepath.Join(state, "created.json"))
	if err != nil {
		return nil
	}
	var out []Created
	if json.Unmarshal(b, &out) != nil {
		return nil
	}
	return out
}

// AddCreated records c, once.
func AddCreated(state string, c Created) error {
	list := ReadCreated(state)
	if slices.Contains(list, c) {
		return nil
	}
	if err := os.MkdirAll(state, 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(append(list, c), "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(filepath.Join(state, "created.json"), append(b, '\n'), 0o600)
}

// DropCreated takes c off the record (a join undone by hand: yardo
// forget has nothing left to undo).
func DropCreated(state string, c Created) error {
	list := ReadCreated(state)
	if !slices.Contains(list, c) {
		return nil
	}
	b, err := json.MarshalIndent(slices.DeleteFunc(list, func(x Created) bool { return x == c }), "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(filepath.Join(state, "created.json"), append(b, '\n'), 0o600)
}
