package settings

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/qinqingxu/acsync/internal/cli"
	"github.com/qinqingxu/acsync/internal/config"
	"github.com/qinqingxu/acsync/internal/resource"
)

func TestLegacySettingsPreservesAndRendersResourceConfiguration(t *testing.T) {
	home := setupHome(t)
	cfg, err := config.Load(cli.ConfigPath(home))
	if err != nil {
		t.Fatal(err)
	}
	cfg.Categories = map[string]map[string]bool{
		"demo": {"skills": false, "sessions": true},
	}
	cfg.CustomResources = []config.CustomResource{{
		ID: "notes", Category: resource.CategoryInstructions,
		Paths:   map[string]string{"linux": "~/.notes"},
		Targets: map[string]string{"linux": "~/.notes"},
		Include: []string{"**/*.md"}, Strategy: resource.StrategyTextTree,
	}}
	if err := config.Save(cli.ConfigPath(home), cfg); err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	Handler(home).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET status = %d", recorder.Code)
	}
	for _, want := range []string{"skills", "sessions", "notes"} {
		if !strings.Contains(recorder.Body.String(), want) {
			t.Fatalf("settings page missing %q: %s", want, recorder.Body.String())
		}
	}

	form := url.Values{
		"repo_url":               {cfg.RepoURL},
		"sync_interval_minutes":  {"10"},
		"trash_grace_days":       {"30"},
		"agent_demo":             {"on"},
		"category_demo_sessions": {"on"},
	}
	request := httptest.NewRequest(
		http.MethodPost,
		"/save",
		strings.NewReader(form.Encode()),
	)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder = httptest.NewRecorder()
	Handler(home).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("POST status = %d: %s", recorder.Code, recorder.Body.String())
	}
	saved, err := config.Load(cli.ConfigPath(home))
	if err != nil {
		t.Fatal(err)
	}
	if saved.CategoryEnabled("demo", resource.CategorySkills) {
		t.Fatal("unchecked skills category was not disabled")
	}
	if len(saved.CustomResources) != 1 || saved.CustomResources[0].ID != "notes" {
		t.Fatalf("legacy save erased custom resources: %#v", saved.CustomResources)
	}
}
