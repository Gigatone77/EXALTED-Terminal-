// Package plugins defines a small plug-in registry for installable, optional
// capabilities (such as the church locator). Plugins are registered at build
// time and their on/off state is persisted in the app settings so they behave
// like installable components.
package plugins

import (
	"context"
	"fmt"

	"github.com/gigatone/biblelearn/internal/store"
)

// Plugin describes an installable optional capability.
type Plugin struct {
	// Name is a short identifier, e.g. "church-locator".
	Name string
	// DisplayName is a human title, e.g. "Church Locator".
	DisplayName string
	// Description explains what the plugin does.
	Description string
	// Version is the plugin's semantic version.
	Version string
	// RequiresNetwork reports whether the plugin needs internet access.
	RequiresNetwork bool
	// Install performs any setup/validation. It may be nil.
	Install func(ctx context.Context, s *store.Store) error
	// Uninstall performs teardown. It may be nil.
	Uninstall func(ctx context.Context, s *store.Store) error
}

// PluginState is a persisted toggle inside the store so plugins feel
// installable/uninstallable without recompiling.
const (
	settingPrefix = "plugin.installed."
)

// Registry holds all known plugins.
type Registry struct {
	byName map[string]*Plugin
	order  []string
}

// New returns an empty registry.
func New() *Registry {
	return &Registry{byName: map[string]*Plugin{}}
}

// Register adds a plugin to the registry.
func (r *Registry) Register(p *Plugin) {
	if _, ok := r.byName[p.Name]; ok {
		return
	}
	r.byName[p.Name] = p
	r.order = append(r.order, p.Name)
}

// Get returns a plugin by name.
func (r *Registry) Get(name string) (*Plugin, bool) {
	p, ok := r.byName[name]
	return p, ok
}

// List returns all registered plugins in registration order.
func (r *Registry) List() []*Plugin {
	out := make([]*Plugin, 0, len(r.order))
	for _, n := range r.order {
		out = append(out, r.byName[n])
	}
	return out
}

// IsInstalled reports whether a plugin's state is currently enabled.
func (r *Registry) IsInstalled(s *store.Store, name string) (bool, error) {
	v, err := s.GetSetting(settingPrefix + name)
	if err != nil {
		return false, err
	}
	return v == "1" || v == "true", nil
}

// Install enables a plugin (idempotent), running its Install hook if present.
func (r *Registry) Install(ctx context.Context, s *store.Store, name string) error {
	p, ok := r.Get(name)
	if !ok {
		return fmt.Errorf("unknown plugin %q", name)
	}
	if installed, _ := r.IsInstalled(s, name); installed {
		return nil
	}
	if p.Install != nil {
		if err := p.Install(ctx, s); err != nil {
			return err
		}
	}
	return s.SetSetting(settingPrefix+name, "1")
}

// Uninstall disables a plugin, running its Uninstall hook if present.
func (r *Registry) Uninstall(ctx context.Context, s *store.Store, name string) error {
	if _, ok := r.Get(name); !ok {
		return fmt.Errorf("unknown plugin %q", name)
	}
	p, _ := r.Get(name)
	if p.Uninstall != nil {
		if err := p.Uninstall(ctx, s); err != nil {
			return err
		}
	}
	return s.SetSetting(settingPrefix+name, "0")
}
