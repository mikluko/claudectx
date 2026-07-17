package main

import "testing"

func TestParseOPRef(t *testing.T) {
	cases := []struct {
		in   string
		want opRef
		err  bool
	}{
		{"apikey", opRef{Item: "apikey"}, false},
		{"Private/apikey", opRef{Vault: "Private", Item: "apikey"}, false},
		{"work/Private/apikey", opRef{Account: "work", Vault: "Private", Item: "apikey"}, false},
		{"a/b/c/d", opRef{}, true},
		{"", opRef{}, true},
		{"/apikey", opRef{}, true},
		{"vault//item", opRef{}, true},
	}
	for _, tc := range cases {
		got, err := parseOPRef(tc.in)
		if tc.err {
			if err == nil {
				t.Errorf("parseOPRef(%q): expected error", tc.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseOPRef(%q): %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("parseOPRef(%q) = %+v, want %+v", tc.in, got, tc.want)
		}
	}
}

func TestPickOPField(t *testing.T) {
	cases := []struct {
		name   string
		fields []opField
		want   string
		ok     bool
	}{
		{"credential preferred over password",
			[]opField{
				{ID: "password", Purpose: "PASSWORD", Value: "pw"},
				{ID: "credential", Value: "cred"},
			}, "cred", true},
		{"password fallback",
			[]opField{
				{ID: "username", Purpose: "USERNAME", Value: "u"},
				{ID: "password", Purpose: "PASSWORD", Value: "pw"},
			}, "pw", true},
		{"credential by label",
			[]opField{{ID: "x1", Label: "credential", Value: "cred"}}, "cred", true},
		{"empty values skipped",
			[]opField{{ID: "credential", Value: ""}}, "", false},
		{"nothing usable",
			[]opField{{ID: "notes", Value: "hello"}}, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := pickOPField(tc.fields)
			if ok != tc.ok || got != tc.want {
				t.Errorf("pickOPField = (%q, %v), want (%q, %v)", got, ok, tc.want, tc.ok)
			}
		})
	}
}
