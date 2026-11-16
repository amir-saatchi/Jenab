package scenario

import (
	"context"
	"iter"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/amir-saatchi/jenab/internal/config"
	"github.com/amir-saatchi/jenab/internal/provider"
	"github.com/amir-saatchi/jenab/internal/provider/fake"
)

// byModel answers each model on its own: the request numbered quotaAt
// (from 1) to the model quotaOn comes back as a used-up quota, every
// other request with a short text.
type byModel struct {
	mu      sync.Mutex
	n       map[string]int
	quotaOn string
	quotaAt int
}

func (b *byModel) Stream(ctx context.Context, req provider.Request) iter.Seq2[provider.Event, error] {
	b.mu.Lock()
	b.n[req.Model]++
	n := b.n[req.Model]
	b.mu.Unlock()
	if req.Model == b.quotaOn && n == b.quotaAt {
		return fake.New(fake.Fail(&provider.Error{Kind: provider.Quota, Provider: "p", Status: 429, Message: "the daily quota is used up"})).Stream(ctx, req)
	}
	return fake.New(reply("Done.", nil)).Stream(ctx, req)
}

func (b *byModel) Models(context.Context) ([]provider.ModelInfo, error) { return nil, nil }

func (b *byModel) requests(model string) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.n[model]
}

// A model whose provider says the quota is used up gets no more runs, in
// this set and the next; the run that got it ends as it does, and the
// other model's runs all go ahead (TASK-048).
func TestQuotaStopsTheModel(t *testing.T) {
	files := map[string]string{
		"_set.yaml": "title: Quota\n",
		"a.yaml":    "id: a\ncontext: main\nmessages:\n  - text: Hi.\nasserts:\n  - {type: max_turns, n: 1}\n",
	}
	first, err := Load(writeSet(t, files))
	if err != nil {
		t.Fatal(err)
	}
	files["_set.yaml"] = "title: Later\n"
	second, err := Load(writeSet(t, files))
	if err != nil {
		t.Fatal(err)
	}
	b := &byModel{n: map[string]int{}, quotaOn: "m1", quotaAt: 3}
	models := Static([]ProviderSpec{{Name: "p", Kind: "openai_compatible", BaseURL: "https://example.test/v1/",
		Models: []config.ModelSettings{{ID: "m1", Context: 32000}, {ID: "m2", Context: 32000}}}},
		map[string]string{"p": "test-key-0123456789"}, "p/m1", "p/m2")
	quotas := &Quotas{}
	var stops []string
	opt := Options{
		Models: models, Backends: map[provider.Kind]provider.Factory{provider.KindCompatible: func(provider.Connection) (provider.Provider, error) { return b, nil }},
		Reps: 5, Scale: 0.01, Timeout: time.Minute, Temp: t.TempDir(), Quotas: quotas,
		QuotaStop: func(model string, at Run) { stops = append(stops, model+" "+at.at()) },
	}

	rep, err := RunSet(context.Background(), first, opt)
	if err != nil {
		t.Fatal(err)
	}
	if got := b.requests("m1"); got != 3 {
		t.Errorf("m1 got %d requests, want 3", got)
	}
	for _, r := range rep.Runs {
		switch {
		case r.Model == "p/m2" && (r.Skipped != "" || r.Error != "" || r.Quota):
			t.Errorf("m2 r%d: %+v", r.Rep, r)
		case r.Model == "p/m1" && r.Rep < 3 && (!r.Passed() || r.Quota):
			t.Errorf("m1 r%d before the quota: %+v", r.Rep, r)
		case r.Model == "p/m1" && r.Rep == 3 && (!r.Quota || r.Error == "" || r.Skipped != "" || r.Requests != 1):
			t.Errorf("m1 r3, the run that got the quota, should end broke: %+v", r)
		case r.Model == "p/m1" && r.Rep > 3 && (r.Skipped != "quota: p/m1" || r.Quota || r.Requests != 0):
			t.Errorf("m1 r%d after the quota should be skipped: %+v", r.Rep, r)
		}
	}
	m1, _ := rep.summary("p/m1")
	m2, _ := rep.summary("p/m2")
	if m1.Runs != 3 || m1.Passed != 2 || m1.Errors != 1 || m1.Skipped != 2 || m1.QuotaStop != "at a r3" {
		t.Errorf("m1 summary %+v", m1)
	}
	if m2.Runs != 5 || m2.Passed != 5 || m2.Skipped != 0 || m2.QuotaStop != "" {
		t.Errorf("m2 summary %+v", m2)
	}
	md := rep.Markdown(nil)
	for _, want := range []string{"## Stopped on quota", "- p/m1: the provider said the quota or credit is used up at a r3. 2 runs here were skipped."} {
		if !strings.Contains(md, want) {
			t.Errorf("Markdown lacks %q:\n%s", want, md)
		}
	}
	if strings.Contains(md, "p/m2: the provider said") {
		t.Errorf("m2 is listed as stopped:\n%s", md)
	}
	if len(stops) != 1 || stops[0] != "p/m1 a r3" {
		t.Errorf("stops %v", stops)
	}

	// The next set: m1 stays stopped and gets nothing; m2 runs.
	rep, err = RunSet(context.Background(), second, opt)
	if err != nil {
		t.Fatal(err)
	}
	if got := b.requests("m1"); got != 3 {
		t.Errorf("m1 got %d requests after the second set, want 3", got)
	}
	m1, _ = rep.summary("p/m1")
	m2, _ = rep.summary("p/m2")
	if m1.Runs != 0 || m1.Skipped != 5 || m1.QuotaStop != "earlier, at Quota, a r3" {
		t.Errorf("m1 summary in the next set %+v", m1)
	}
	if m2.Runs != 5 || m2.Passed != 5 {
		t.Errorf("m2 summary in the next set %+v", m2)
	}
	md = rep.Markdown(nil)
	for _, want := range []string{"| a | skip (quota: p/m1) | 5/5 |", "used up earlier, at Quota, a r3. 5 runs here were skipped."} {
		if !strings.Contains(md, want) {
			t.Errorf("Markdown lacks %q:\n%s", want, md)
		}
	}
	if len(stops) != 1 {
		t.Errorf("stops %v", stops)
	}
}

// Only the quota kind stops a model; a per-minute limit, which the app
// retries, doesn't.
func TestCountingMarksQuota(t *testing.T) {
	for _, c := range []struct {
		kind provider.ErrorKind
		want bool
	}{{provider.Quota, true}, {provider.RateLimited, false}, {provider.Overloaded, false}, {provider.BadRequest, false}} {
		var n atomic.Int64
		var quota atomic.Bool
		fp := fake.New(fake.Fail(&provider.Error{Kind: c.kind, Provider: "p", Status: 429}))
		p, _ := counted(map[provider.Kind]provider.Factory{"k": fp.Factory()}, &n, &quota)["k"](provider.Connection{})
		for range p.Stream(context.Background(), provider.Request{}) {
		}
		if quota.Load() != c.want || n.Load() != 1 {
			t.Errorf("%s: quota %v, requests %d", c.kind, quota.Load(), n.Load())
		}
	}
}
