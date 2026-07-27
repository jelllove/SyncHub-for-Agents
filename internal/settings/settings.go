// Package settings serves a local web page for editing acsync configuration.
package settings

import (
	"html/template"
	"net/http"
	"sort"
	"strconv"

	"github.com/qinqingxu/acsync/internal/cli"
	"github.com/qinqingxu/acsync/internal/config"
)

// AgentView is one agent row in the settings page.
type AgentView struct {
	Name    string
	Enabled bool
	Exclude []string
}

// ViewModel is the data rendered by the settings page.
type ViewModel struct {
	RepoURL             string
	SyncIntervalMinutes int
	TrashGraceDays      int
	Agents              []AgentView
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
		vm.Agents = append(vm.Agents, AgentView{
			Name:    p.Name,
			Enabled: cfg.Agents[p.Name],
			Exclude: p.Config.Exclude,
		})
	}
	sort.Slice(vm.Agents, func(i, j int) bool { return vm.Agents[i].Name < vm.Agents[j].Name })
	return vm, nil
}

// Handler returns the settings HTTP handler for the given acsync home.
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
	}
	return config.Save(cli.ConfigPath(home), cfg)
}

var pageTemplate = template.Must(template.New("page").Parse(`<!doctype html>
<html>
<head><meta charset="utf-8"><title>acsync settings</title></head>
<body>
<h1>AgentConfigSync Settings</h1>
<form method="POST" action="/save">
<p><label>Repo URL: <input name="repo_url" value="{{.RepoURL}}" size="60"></label></p>
<p><label>Sync interval (minutes): <input name="sync_interval_minutes" type="number" min="1" value="{{.SyncIntervalMinutes}}"></label></p>
<p><label>Trash grace (days): <input name="trash_grace_days" type="number" min="0" value="{{.TrashGraceDays}}"></label></p>
<h2>Agents</h2>
{{range .Agents}}
<fieldset>
<label><input type="checkbox" name="agent_{{.Name}}" {{if .Enabled}}checked{{end}}> {{.Name}}</label>
{{if .Exclude}}<div>Excluded: {{range .Exclude}}<code>{{.}}</code> {{end}}</div>{{end}}
</fieldset>
{{end}}
<p><button type="submit">Save</button></p>
</form>
</body>
</html>
`))
