// Package engine is the high-level orchestrator: it wraps the local store,
// online fetching, import, search, memory, and quiz subsystems behind a single
// API used by both the terminal UI and the web/app UI.
package engine

import (
	"context"
	"fmt"

	"github.com/gigatone/biblelearn/internal/model"
	"github.com/gigatone/biblelearn/internal/store"
)

// Engine is the top-level application object.
type Engine struct {
	Store *store.Store
}

// New opens the store (at the default or explicit data dir) and returns an
// engine ready for use.
func New(dir string) (*Engine, error) {
	st, err := store.Open(dir)
	if err != nil {
		return nil, fmt.Errorf("open store: %w", err)
	}
	return &Engine{Store: st}, nil
}

// Close releases resources.
func (e *Engine) Close() error {
	return e.Store.Close()
}

// EnsureDefaultVersion provisions the stock standard versions on first run:
// KJV (the public-domain bundled standard) is always seeded from the embedded
// data, and NKJV (the dominant default) is pulled from the reliable GetBible
// API when no version exists yet. It is idempotent and runs without a network.
func (e *Engine) EnsureDefaultVersion(ctx context.Context) error {
	// KJV: bundled in the binary, always available offline.
	if _, err := e.SeedEmbeddedKJV(); err != nil {
		return err
	}
	active, err := e.ActiveVersionID()
	if err != nil {
		return err
	}
	if active == "" {
		// No versions with text: seed the dominant stock standard.
		if _, ok, _ := e.Store.GetVersion("NKJV"); !ok {
			v := model.Version{ID: "NKJV", Name: "New King James Version", Lang: "en",
				Source: "GetBible", SourceURL: "https://bolls.life", Enabled: true}
			_, _ = e.InstallNKJV(ctx, v, nil)
		}
	}
	return nil
}
