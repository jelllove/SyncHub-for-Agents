package repository

import "testing"

func TestParseGitHubURL(t *testing.T) {
	tests := []struct {
		raw        string
		protocol   Protocol
		owner      string
		repository string
		ok         bool
	}{
		{"https://github.com/acme/sync.git", HTTPS, "acme", "sync", true},
		{"git@github.com:acme/sync.git", SSH, "acme", "sync", true},
		{"ssh://git@github.com/acme/sync.git", SSH, "acme", "sync", true},
		{"https://token@github.com/acme/sync.git", "", "", "", false},
		{"https://evil.example/acme/sync.git", "", "", "", false},
		{"https://github.com/acme/sync.git?x=1", "", "", "", false},
		{"git@github.com:acme/../sync.git", "", "", "", false},
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
			if got.Protocol != test.protocol || got.Owner != test.owner || got.Repository != test.repository {
				t.Fatalf("ParseGitHubURL(%q) = %#v", test.raw, got)
			}
		})
	}
}
