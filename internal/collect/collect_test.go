package collect

import "testing"

func TestEveryPanelHasItsOwnIcon(t *testing.T) {
	seen := map[string]string{}
	for _, c := range All() {
		if c.Icon == "" {
			t.Errorf("%s has no icon", c.Name)
			continue
		}
		if other, ok := seen[c.Icon]; ok {
			t.Errorf("%s and %s share an icon", c.Name, other)
		}
		seen[c.Icon] = c.Name
	}
}
