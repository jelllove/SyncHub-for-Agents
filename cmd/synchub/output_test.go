package main

import "testing"

func TestRequestedJSONSurvivesFlagErrorsWithoutTreatingValuesAsFlags(t *testing.T) {
	for _, test := range []struct {
		args []string
		want bool
	}{
		{[]string{"status", "--bogus", "--json"}, true},
		{[]string{"status", "--json", "--bogus"}, true},
		{[]string{"status", "--json", "--bogus", "--json=false"}, false},
		{[]string{"--home", "--json", "status"}, false},
		{[]string{"--home=--json", "status"}, false},
		{[]string{"status", "--", "--json"}, false},
		{[]string{"status", "--json=true"}, true},
	} {
		if got := requestedJSON(test.args); got != test.want {
			t.Errorf("requestedJSON(%v) = %t, want %t", test.args, got, test.want)
		}
	}
}
