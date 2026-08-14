package desktop

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/qinqingxu/acsync/internal/cli"
	"github.com/qinqingxu/acsync/internal/config"
	"github.com/qinqingxu/acsync/internal/conflict"
	"github.com/qinqingxu/acsync/internal/installplan"
	"github.com/qinqingxu/acsync/internal/portableconfig"
	"github.com/qinqingxu/acsync/internal/provider"
	"github.com/qinqingxu/acsync/internal/resource"
	"github.com/qinqingxu/acsync/internal/resourcecollect"
	"github.com/qinqingxu/acsync/internal/syncengine"
)

func (s *Service) ResourcePreview() (ResourcePreview, error) {
	cfg, err := config.Load(cli.ConfigPath(s.home))
	if err != nil {
		return ResourcePreview{}, err
	}
	providers, err := cli.LoadProviders(s.home)
	if err != nil {
		return ResourcePreview{}, err
	}
	return s.preview(cfg, providers)
}

func (s *Service) PreviewCustomResource(input CustomResourceInput) (ResourcePreview, error) {
	custom := input.toConfig()
	if err := config.ValidateCustomResources([]config.CustomResource{custom}); err != nil {
		return ResourcePreview{}, err
	}
	return s.preview(config.Config{
		CustomResources: []config.CustomResource{custom},
		Agents:          map[string]bool{},
	}, nil)
}

func (s *Service) preview(cfg config.Config, providers []provider.Provider) (result ResourcePreview, retErr error) {
	userHome, err := os.UserHomeDir()
	if err != nil {
		return ResourcePreview{}, err
	}
	specs, err := cli.BuildResourceSpecs(cfg, providers, s.goos, userHome)
	if err != nil {
		return ResourcePreview{}, err
	}
	stageParent, err := os.MkdirTemp("", "acsync-preview-*")
	if err != nil {
		return ResourcePreview{}, err
	}
	defer func() {
		retErr = errors.Join(retErr, os.RemoveAll(stageParent))
	}()
	collector := resourcecollect.New(resourcecollect.Options{
		StageParent: stageParent,
		GOOS:        s.goos,
		UserHome:    userHome,
		Projector:   portableconfig.BuiltinRegistry(),
		Inventory: installplan.NewBuiltinInventory(
			installplan.CommandRunner{},
		),
	})
	collected, err := collector.Collect(sortedSpecs(specs))
	if err != nil {
		return ResourcePreview{}, err
	}
	defer func() {
		retErr = errors.Join(retErr, collected.Close())
	}()

	byKey := make(map[string][]int, len(specs))
	for _, key := range sortedSpecKeys(specs) {
		spec := specs[key]
		target := ""
		if len(spec.Targets) > 0 {
			target = spec.Targets[0]
		}
		result.Resources = append(result.Resources, ResourceCategory{
			Provider:  spec.Provider,
			ID:        spec.ID,
			Category:  string(spec.Category),
			Enabled:   true,
			Supported: true,
			Source:    spec.Root,
			Target:    target,
			Status:    "ready",
		})
		collectionKey := key
		if spec.SharedAs != "" {
			collectionKey = "common/" + spec.SharedAs
		}
		byKey[collectionKey] = append(byKey[collectionKey], len(result.Resources)-1)
	}
	for _, artifact := range collected.Artifacts {
		indexes := byKey[artifact.ResourceKey]
		if len(indexes) == 0 {
			continue
		}
		for _, index := range indexes {
			result.Resources[index].FileCount++
			result.Resources[index].Bytes += artifact.Size
		}
		result.Files++
		result.Bytes += artifact.Size
	}
	issues := append(append([]resource.Issue{}, collected.Blocked...), collected.Skipped...)
	for _, issue := range issues {
		result.Issues = append(result.Issues, ResourceIssue{
			ResourceKey: issue.ResourceKey,
			Path:        issue.Path,
			Code:        issue.Code,
			Message:     issue.Message,
			Bytes:       issue.Bytes,
		})
		for _, index := range byKey[issue.ResourceKey] {
			result.Resources[index].ExcludedFiles++
			result.Resources[index].ExcludedBytes += issue.Bytes
			result.Resources[index].Status = "attention"
			result.Resources[index].Reason = issue.Message
		}
		result.ExcludedFiles++
		result.ExcludedBytes += issue.Bytes
	}
	return result, nil
}

func (input CustomResourceInput) toConfig() config.CustomResource {
	return config.CustomResource{
		ID:       input.ID,
		Category: resource.Category(input.Category),
		Paths:    cloneStringMap(input.Paths),
		Targets:  cloneStringMap(input.Targets),
		Include:  append([]string(nil), input.Include...),
		Exclude:  append([]string(nil), input.Exclude...),
		Strategy: resource.Strategy(input.Strategy),
	}
}

func sortedSpecs(specs map[string]resource.Spec) []resource.Spec {
	keys := sortedSpecKeys(specs)
	result := make([]resource.Spec, 0, len(keys))
	for _, key := range keys {
		result = append(result, specs[key])
	}
	return result
}

func sortedSpecKeys(specs map[string]resource.Spec) []string {
	keys := make([]string, 0, len(specs))
	for key := range specs {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func cloneStringMap(source map[string]string) map[string]string {
	if source == nil {
		return nil
	}
	result := make(map[string]string, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func previewByIdentity(preview ResourcePreview) map[string]ResourceCategory {
	result := make(map[string]ResourceCategory, len(preview.Resources))
	for _, item := range preview.Resources {
		result[resourceIdentity(item.Provider, item.ID, item.Category)] = item
	}
	return result
}

func unsupportedResource(
	provider, id string,
	category resource.Category,
	source string,
	enabled bool,
) ResourceCategory {
	return ResourceCategory{
		Provider: provider, ID: id,
		Category: string(category), Enabled: enabled,
		Supported: false, Source: source, Status: "unsupported",
		Reason: fmt.Sprintf("resource has no path for this platform"),
	}
}

func resourceIdentity(provider, id, category string) string {
	return provider + "\x00" + id + "\x00" + category
}

func (s *Service) ApproveInstallPlan(id string) error {
	store := installplan.NewStore(filepath.Join(s.home, "install"))
	pending, err := store.Pending()
	if err != nil {
		return err
	}
	if pending == nil {
		return fmt.Errorf("no install plan is pending")
	}
	if pending.ID != id {
		return fmt.Errorf("pending install plan is %q, not %q", pending.ID, id)
	}
	for _, operation := range pending.Operations {
		if err := store.Approve(operation); err != nil {
			return err
		}
	}
	return s.Trigger()
}

func (s *Service) ResolveConflict(input ConflictResolution) error {
	if input.Choice != "local" &&
		input.Choice != "remote" &&
		input.Choice != "merged" {
		return fmt.Errorf("conflict choice %q is not supported", input.Choice)
	}
	cfg, err := config.Load(cli.ConfigPath(s.home))
	if err != nil {
		return err
	}
	providers, err := cli.LoadProviders(s.home)
	if err != nil {
		return err
	}
	userHome, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	specs, err := cli.BuildResourceSpecs(cfg, providers, s.goos, userHome)
	if err != nil {
		return err
	}
	store := conflictStore(s.home, syncengine.ConflictScanner(specs))
	if input.Choice == "merged" {
		err = store.Resolve(input.ID, []byte(input.Content))
	} else {
		err = store.ResolveVariant(input.ID, input.Choice)
	}
	if err != nil {
		return err
	}
	return s.Trigger()
}

func conflictStore(home string, scanner conflict.Scanner) *conflict.Store {
	return conflict.NewStore(
		filepath.Join(home, "conflicts"),
		filepath.Join(home, "repo"),
		scanner,
	)
}

func desktopInstallPlan(plan *installplan.Plan) *InstallPlan {
	if plan == nil {
		return nil
	}
	result := &InstallPlan{ID: plan.ID, Approved: plan.Approved}
	for _, operation := range plan.Operations {
		result.Operations = append(result.Operations, InstallOperation{
			ID:         operation.ID,
			Adapter:    operation.Adapter,
			Source:     operation.Source,
			Kind:       operation.Kind,
			Executable: operation.Executable,
			Args:       append([]string(nil), operation.Args...),
			WorkingDir: operation.WorkingDir,
			Error:      plan.Errors[operation.ID],
		})
	}
	return result
}

func desktopConflicts(records []conflict.Record) []ConflictSummary {
	result := make([]ConflictSummary, 0, len(records))
	for _, record := range records {
		result = append(result, ConflictSummary{
			ID:          record.ID,
			ResourceKey: record.ResourceKey,
			Path:        record.RepoRel,
			CreatedAt:   record.CreatedAt,
		})
	}
	return result
}
