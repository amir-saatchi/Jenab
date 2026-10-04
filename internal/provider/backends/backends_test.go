package backends

import (
	"testing"

	"github.com/amir-saatchi/jenab/internal/provider"
)

func TestEveryKind(t *testing.T) {
	all := All()
	for _, k := range provider.Kinds {
		f := all[k]
		if f == nil {
			t.Errorf("no backend for %s", k)
			continue
		}
		base := provider.DefaultBaseURL(k)
		if base == "" {
			base = "https://api.z.ai/api/paas/v4/"
		}
		if _, err := f(provider.Connection{Name: "x", Kind: k, BaseURL: base}); err != nil {
			t.Errorf("%s: %v", k, err)
		}
	}
}
