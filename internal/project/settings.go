package project

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"golang.org/x/net/idna"
)

// Meta keys of the project's settings (SPEC 6.7, 8.8).
const (
	metaLevel        = "approval_level"
	metaPrivateHosts = "private_hosts"
)

// Level is how much the agent asks before it acts (SPEC 8.8).
type Level string

const (
	Strict   Level = "strict"
	Standard Level = "standard"
	Auto     Level = "auto"
)

// ParseLevel checks s, as approvals.default_level and the chip give it.
func ParseLevel(s string) (Level, error) {
	switch l := Level(s); l {
	case Strict, Standard, Auto:
		return l, nil
	}
	return "", fmt.Errorf("project: unknown approval level %q", s)
}

// Level is the project's approval level; Standard if none is stored.
func (p *Project) Level(ctx context.Context) (Level, error) {
	meta, err := p.DB.Meta(ctx)
	if err != nil {
		return "", err
	}
	if l, err := ParseLevel(meta[metaLevel]); err == nil {
		return l, nil
	}
	return Standard, nil
}

// SetLevel stores the project's approval level.
func (p *Project) SetLevel(ctx context.Context, l Level) error {
	if _, err := ParseLevel(string(l)); err != nil {
		return err
	}
	return p.DB.SetMeta(ctx, map[string]string{metaLevel: string(l)})
}

// PrivateHosts are the hosts that may reach private network addresses:
// the project's exceptions (SPEC 6.7).
func (p *Project) PrivateHosts(ctx context.Context) ([]string, error) {
	meta, err := p.DB.Meta(ctx)
	if err != nil {
		return nil, err
	}
	var hosts []string
	if v := meta[metaPrivateHosts]; v != "" {
		if err := json.Unmarshal([]byte(v), &hosts); err != nil {
			return nil, fmt.Errorf("project: %s: %w", metaPrivateHosts, err)
		}
	}
	return hosts, nil
}

// SetPrivateHosts replaces the exceptions. Hosts are kept in lower case
// and ASCII (punycode, which also lowers the case), as requests name them.
func (p *Project) SetPrivateHosts(ctx context.Context, hosts []string) error {
	var out []string
	for _, h := range hosts {
		h = strings.TrimSpace(h)
		if h == "" {
			continue
		}
		a, err := idna.Lookup.ToASCII(h)
		if err != nil {
			return fmt.Errorf("project: host %q: %w", h, err)
		}
		out = append(out, a)
	}
	slices.Sort(out)
	b, err := json.Marshal(slices.Compact(out))
	if err != nil {
		return err
	}
	return p.DB.SetMeta(ctx, map[string]string{metaPrivateHosts: string(b)})
}
