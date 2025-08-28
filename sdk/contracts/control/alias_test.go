package ctrl

import "testing"

func TestAliasKey(t *testing.T) {
	cases := []struct {
		id   string
		want string
		ok   bool
	}{
		{"llama2:7b-q4_0", "llama2:7b", true},
		{"llama2:7b", "llama2:7b", true},
		{"mistral:7b-q5_k_m", "mistral:7b", true},
		{"llama2-7b-q4_0", "", false},
	}
	for _, c := range cases {
		got, ok := AliasKey(c.id)
		if ok != c.ok || got != c.want {
			t.Errorf("AliasKey(%q)=%q,%v want %q,%v", c.id, got, ok, c.want, c.ok)
		}
	}
}
