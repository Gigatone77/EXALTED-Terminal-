package engine

import (
	"context"

	"github.com/gigatone/biblelearn/internal/plugins"
	"github.com/gigatone/biblelearn/internal/plugins/churchlocator"
)

// Plugins returns the app's configured plugin registry.
func (e *Engine) Plugins() *plugins.Registry {
	r := plugins.New()
	r.Register(churchlocator.Plugin())
	return r
}

// InstallPlugin enables a plugin by name (idempotent).
func (e *Engine) InstallPlugin(ctx context.Context, name string) error {
	return e.Plugins().Install(ctx, e.Store, name)
}

// UninstallPlugin disables a plugin by name.
func (e *Engine) UninstallPlugin(ctx context.Context, name string) error {
	return e.Plugins().Uninstall(ctx, e.Store, name)
}

// PluginInstalled reports whether a plugin is currently enabled.
func (e *Engine) PluginInstalled(name string) (bool, error) {
	return e.Plugins().IsInstalled(e.Store, name)
}

// LocateChurches resolves a place and returns nearby churches (church locator
// plugin). It works online against open-source OSM services and uses the local
// cache offline.
func (e *Engine) LocateChurches(ctx context.Context, query string, radiusKm float64) (string, []churchlocator.Church, error) {
	return churchlocator.Locate(ctx, e.Store, query, radiusKm)
}
