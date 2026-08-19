package installplan

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/qinqingxu/acsync/internal/resource"
)

type Adapter interface {
	Discover(ctx context.Context, spec resource.Spec) ([]Declaration, error)
	Operations(desired, current []Declaration) ([]Operation, error)
}

type InventoryRegistry struct {
	adapters map[string]Adapter
}

func NewInventoryRegistry() *InventoryRegistry {
	return &InventoryRegistry{adapters: map[string]Adapter{}}
}

func NewBuiltinInventory(runner Runner) *InventoryRegistry {
	registry := NewInventoryRegistry()
	registry.Register("claude-plugin", NewClaudeAdapter(runner))
	registry.Register("copilot-plugin", NewCopilotAdapter(runner))
	return registry
}

func (r *InventoryRegistry) Register(name string, adapter Adapter) {
	r.adapters[name] = adapter
}

func (r *InventoryRegistry) Adapter(name string) (Adapter, bool) {
	adapter, ok := r.adapters[name]
	return adapter, ok
}

func (r *InventoryRegistry) Inventory(spec resource.Spec) (map[string][]byte, error) {
	return r.InventoryContext(context.Background(), spec)
}

func (r *InventoryRegistry) InventoryContext(
	ctx context.Context,
	spec resource.Spec,
) (map[string][]byte, error) {
	adapter, ok := r.adapters[spec.Installer]
	if !ok {
		return nil, fmt.Errorf("installer adapter %q is unavailable", spec.Installer)
	}
	declarations, err := adapter.Discover(ctx, spec)
	if err != nil {
		return nil, err
	}
	if len(declarations) == 0 {
		return map[string][]byte{}, nil
	}
	data, err := json.MarshalIndent(declarations, "", "  ")
	if err != nil {
		return nil, err
	}
	return map[string][]byte{"manifest.json": append(data, '\n')}, nil
}
