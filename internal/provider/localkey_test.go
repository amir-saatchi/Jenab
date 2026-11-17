package provider_test

import (
	"context"
	"testing"

	"github.com/amir-saatchi/jenab/internal/provider"
	"github.com/amir-saatchi/jenab/internal/provider/fake"
)

// A provider on this computer may have a token: connected again with a new
// one, the new one is checked and stored; with none, it is checked without
// one and the stored one goes. A check that fails keeps it (TASK-074).
func TestALocalKeyIsReplacedAndRemoved(t *testing.T) {
	ctx := context.Background()
	const base = "http://localhost:1234/v1/"
	fp := fake.New()
	s := newConnectSetup(fp)
	stored := func() (string, bool) {
		s.kr.mu.Lock()
		defer s.kr.mu.Unlock()
		v, ok := s.kr.m[provider.KeyName("studio")]
		return v, ok
	}
	sent := func() string {
		b := s.rec.built()
		return b[len(b)-1].Key.Reveal()
	}

	for _, key := range []string{"tok-first-0123", "tok-second-0123"} {
		if _, err := s.reg.Connect(ctx, "studio", provider.KindCompatible, base, key, nil); err != nil {
			t.Fatal(err)
		}
		if v, ok := stored(); !ok || v != key || sent() != key {
			t.Errorf("after connecting with a token: stored %v, the check used the new token %v", ok, sent() == key)
		}
	}

	fp.ListErr = &provider.Error{Kind: provider.BadRequest, Status: 401, Message: "a token is needed"}
	if _, err := s.reg.Connect(ctx, "studio", provider.KindCompatible, base, "", nil); err == nil {
		t.Fatal("Connect worked with a 401 model list")
	}
	if v, ok := stored(); !ok || v != "tok-second-0123" {
		t.Error("a failed check without a token removed the stored one")
	}

	fp.ListErr = nil
	if _, err := s.reg.Connect(ctx, "studio", provider.KindCompatible, base, "", nil); err != nil {
		t.Fatal(err)
	}
	if _, ok := stored(); ok || sent() != "" {
		t.Errorf("after connecting without a token: still stored %v, the check sent one %v", ok, sent() != "")
	}
	// Again without one: nothing to remove, and no error.
	if _, err := s.reg.Connect(ctx, "studio", provider.KindCompatible, base, "", nil); err != nil {
		t.Fatal(err)
	}
}
