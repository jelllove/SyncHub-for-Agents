package resource

import (
	"strings"
	"testing"
)

func TestDeclarationValidateAcceptsSupportedValues(t *testing.T) {
	t.Parallel()

	tests := []Declaration{
		{ID: "sessions", Category: CategorySessions, Strategy: StrategyFileTree},
		{ID: "config", Category: CategoryConfig, Strategy: StrategyStructuredMerge, Transformer: "generic-safe"},
		{ID: "instructions", Category: CategoryInstructions, Strategy: StrategyTextTree},
		{ID: "skills", Category: CategorySkills, Strategy: StrategySourceTree},
		{ID: "plugins", Category: CategoryPlugins, Strategy: StrategyInstallManifest, Installer: "claude-plugin"},
	}

	for _, declaration := range tests {
		declaration := declaration
		t.Run(declaration.ID, func(t *testing.T) {
			t.Parallel()

			got := declaration.Normalized()
			if got.Layout != LayoutPortable {
				t.Fatalf("layout = %q, want %q", got.Layout, LayoutPortable)
			}
			if err := got.Validate("demo"); err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
		})
	}
}

func TestDeclarationValidateRejectsUnsupportedValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		declaration Declaration
		want        string
	}{
		{
			name:        "missing-id",
			declaration: Declaration{Category: CategoryConfig, Strategy: StrategyFileTree},
			want:        "missing id",
		},
		{
			name:        "id-with-slash",
			declaration: Declaration{ID: "config/settings", Category: CategoryConfig, Strategy: StrategyFileTree},
			want:        "invalid resource id",
		},
		{
			name:        "id-with-backslash",
			declaration: Declaration{ID: `config\settings`, Category: CategoryConfig, Strategy: StrategyFileTree},
			want:        "invalid resource id",
		},
		{
			name:        "unsafe-shared-as",
			declaration: Declaration{ID: "skills", Category: CategorySkills, Strategy: StrategySourceTree, SharedAs: "../common"},
			want:        "invalid shared_as",
		},
		{
			name:        "bad-category",
			declaration: Declaration{ID: "bad-category", Category: Category("unknown"), Strategy: StrategyFileTree},
			want:        "unsupported category",
		},
		{
			name:        "bad-strategy",
			declaration: Declaration{ID: "bad-strategy", Category: CategoryConfig, Strategy: Strategy("unknown")},
			want:        "unsupported strategy",
		},
		{
			name:        "bad-layout",
			declaration: Declaration{ID: "bad-layout", Category: CategoryConfig, Strategy: StrategyFileTree, Layout: Layout("sideways")},
			want:        "unsupported layout",
		},
		{
			name:        "bad-transformer",
			declaration: Declaration{ID: "bad-transformer", Category: CategoryConfig, Strategy: StrategyStructuredMerge, Transformer: "unknown"},
			want:        "unknown transformer",
		},
		{
			name:        "missing-transformer",
			declaration: Declaration{ID: "missing-transformer", Category: CategoryConfig, Strategy: StrategyStructuredMerge},
			want:        "missing transformer",
		},
		{
			name:        "bad-installer",
			declaration: Declaration{ID: "bad-installer", Category: CategoryPlugins, Strategy: StrategyInstallManifest, Installer: "unknown"},
			want:        "unknown installer",
		},
		{
			name:        "missing-installer",
			declaration: Declaration{ID: "missing-installer", Category: CategoryPlugins, Strategy: StrategyInstallManifest},
			want:        "missing installer",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.declaration.Normalized().Validate("demo")
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate() error = %v, want substring %q", err, tt.want)
			}
		})
	}
}
