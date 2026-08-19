// Package settings serves a local web page for editing synchub configuration.
package settings

import (
	"html/template"
	"net/http"
	"sort"
	"strconv"

	"github.com/qinqingxu/synchub-for-agents/internal/cli"
	"github.com/qinqingxu/synchub-for-agents/internal/config"
	"github.com/qinqingxu/synchub-for-agents/internal/resource"
)

// AgentView is one agent row in the settings page.
type AgentView struct {
	Name       string
	Enabled    bool
	Exclude    []string
	Categories []CategoryView
}

type CategoryView struct {
	Name    string
	Enabled bool
}

type CustomResourceView struct {
	ID       string
	Category string
	Strategy string
}

// ViewModel is the data rendered by the settings page.
type ViewModel struct {
	RepoURL             string
	SyncIntervalMinutes int
	TrashGraceDays      int
	Agents              []AgentView
	CustomResources     []CustomResourceView
}

// BuildViewModel merges the saved config with the known providers.
func BuildViewModel(home string) (ViewModel, error) {
	cfg, err := config.Load(cli.ConfigPath(home))
	if err != nil {
		return ViewModel{}, err
	}
	providers, err := cli.LoadProviders(home)
	if err != nil {
		return ViewModel{}, err
	}
	vm := ViewModel{
		RepoURL:             cfg.RepoURL,
		SyncIntervalMinutes: cfg.SyncIntervalMinutes,
		TrashGraceDays:      cfg.TrashGraceDays,
	}
	for _, p := range providers {
		agent := AgentView{
			Name:    p.Name,
			Enabled: cfg.Agents[p.Name],
		}
		declarations, err := p.Declarations()
		if err != nil {
			return ViewModel{}, err
		}
		categories := map[string]struct{}{}
		excludes := map[string]struct{}{}
		for _, declaration := range declarations {
			categories[string(declaration.Category)] = struct{}{}
			for _, exclude := range declaration.Exclude {
				excludes[exclude] = struct{}{}
			}
		}
		for category := range categories {
			agent.Categories = append(agent.Categories, CategoryView{
				Name:    category,
				Enabled: cfg.CategoryEnabled(p.Name, resource.Category(category)),
			})
		}
		sort.Slice(agent.Categories, func(i, j int) bool {
			return agent.Categories[i].Name < agent.Categories[j].Name
		})
		for exclude := range excludes {
			agent.Exclude = append(agent.Exclude, exclude)
		}
		sort.Strings(agent.Exclude)
		vm.Agents = append(vm.Agents, agent)
	}
	for _, custom := range cfg.CustomResources {
		vm.CustomResources = append(vm.CustomResources, CustomResourceView{
			ID: custom.ID, Category: string(custom.Category), Strategy: string(custom.Strategy),
		})
	}
	sort.Slice(vm.Agents, func(i, j int) bool { return vm.Agents[i].Name < vm.Agents[j].Name })
	return vm, nil
}

// Handler returns the settings HTTP handler for the given synchub home.
func Handler(home string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		vm, err := BuildViewModel(home)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := pageTemplate.Execute(w, vm); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})
	mux.HandleFunc("/save", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if err := saveForm(home, r); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		vm, err := BuildViewModel(home)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := pageTemplate.Execute(w, vm); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})
	return mux
}

func saveForm(home string, r *http.Request) error {
	if err := r.ParseForm(); err != nil {
		return err
	}
	cfg, err := config.Load(cli.ConfigPath(home))
	if err != nil {
		return err
	}
	cfg.RepoURL = r.FormValue("repo_url")
	if v, err := strconv.Atoi(r.FormValue("sync_interval_minutes")); err == nil {
		cfg.SyncIntervalMinutes = v
	}
	if v, err := strconv.Atoi(r.FormValue("trash_grace_days")); err == nil {
		cfg.TrashGraceDays = v
	}
	providers, err := cli.LoadProviders(home)
	if err != nil {
		return err
	}
	if cfg.Agents == nil {
		cfg.Agents = map[string]bool{}
	}
	for _, p := range providers {
		cfg.Agents[p.Name] = r.FormValue("agent_"+p.Name) == "on"
		declarations, err := p.Declarations()
		if err != nil {
			return err
		}
		if cfg.Categories == nil {
			cfg.Categories = map[string]map[string]bool{}
		}
		if cfg.Categories[p.Name] == nil {
			cfg.Categories[p.Name] = map[string]bool{}
		}
		for _, declaration := range declarations {
			category := string(declaration.Category)
			cfg.Categories[p.Name][category] =
				r.FormValue("category_"+p.Name+"_"+category) == "on"
		}
	}
	return config.Save(cli.ConfigPath(home), cfg)
}

var pageTemplate = template.Must(template.New("page").Parse(`<!doctype html>
<html>
<head><meta charset="utf-8"><title>synchub settings</title></head>
<body>
<h1>SyncHub Settings</h1>
<form method="POST" action="/save">
<p><label>Repo URL: <input name="repo_url" value="{{.RepoURL}}" size="60"></label></p>
<p><label>Sync interval (minutes): <input name="sync_interval_minutes" type="number" min="1" value="{{.SyncIntervalMinutes}}"></label></p>
<p><label>Trash grace (days): <input name="trash_grace_days" type="number" min="0" value="{{.TrashGraceDays}}"></label></p>
<h2>Agents</h2>
{{range .Agents}}
<fieldset>
{{$agent := .}}
<label><input type="checkbox" name="agent_{{.Name}}" {{if .Enabled}}checked{{end}}> {{.Name}}</label>
{{range .Categories}}<label><input type="checkbox" name="category_{{$agent.Name}}_{{.Name}}" {{if .Enabled}}checked{{end}}> {{.Name}}</label>{{end}}
{{if .Exclude}}<div>Excluded: {{range .Exclude}}<code>{{.}}</code> {{end}}</div>{{end}}
</fieldset>
{{end}}
{{if .CustomResources}}
<h2>Custom Resources</h2>
{{range .CustomResources}}<div><strong>{{.ID}}</strong> ({{.Category}}, {{.Strategy}})</div>{{end}}
{{end}}
<p><button type="submit">Save</button></p>
</form>
</body>
</html>
`))
