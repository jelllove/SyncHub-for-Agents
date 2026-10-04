package repository

import "testing"

func TestParseGitHubURL(t *testing.T) {
	tests := []struct {
		raw        string
		protocol   Protocol
		sshHost    string
		owner      string
		repository string
		cloneURL   string
		ok         bool
	}{
		{
			"https://github.com/acme/sync.git",
			HTTPS, "", "acme", "sync",
			"https://github.com/acme/sync.git", true,
		},
		{
			"git@github.com:acme/sync.git",
			SSH, "github.com", "acme", "sync",
			"git@github.com:acme/sync.git", true,
		},
		{
			"ssh://git@github.com/acme/sync.git",
			SSH, "github.com", "acme", "sync",
			"ssh://git@github.com/acme/sync.git", true,
		},
		{
			"git@github-jelllove:jelllove/agents-backup.git",
			SSH, "github-jelllove", "jelllove", "agents-backup",
			"git@github-jelllove:jelllove/agents-backup.git", true,
		},
		{
			"ssh://git@github-jelllove/jelllove/agents-backup.git",
			SSH, "github-jelllove", "jelllove", "agents-backup",
			"ssh://git@github-jelllove/jelllove/agents-backup.git", true,
		},
		{"https://token@github.com/acme/sync.git", "", "", "", "", "", false},
		{"https://evil.example/acme/sync.git", "", "", "", "", "", false},
		{"https://github.com/acme/sync.git?x=1", "", "", "", "", "", false},
		{"git@github.com:acme/../sync.git", "", "", "", "", "", false},
		{"git@-bad:acme/sync.git", "", "", "", "", "", false},
		{"git@bad*:acme/sync.git", "", "", "", "", "", false},
	}
	for _, test := range tests {
		t.Run(test.raw, func(t *testing.T) {
			got, err := ParseGitHubURL(test.raw)
			if test.ok && err != nil {
				t.Fatal(err)
			}
			if !test.ok {
				if err == nil {
					t.Fatalf("ParseGitHubURL(%q) unexpectedly succeeded", test.raw)
				}
				return
			}
			if got.Protocol != test.protocol ||
				got.SSHHost != test.sshHost ||
				got.Owner != test.owner ||
				got.Repository != test.repository ||
				got.CloneURL != test.cloneURL {
				t.Fatalf("ParseGitHubURL(%q) = %#v", test.raw, got)
			}
		})
	}
}
