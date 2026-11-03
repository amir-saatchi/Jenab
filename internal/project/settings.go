package project

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/amir-saatchi/jenab/internal/web"
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
// the project's exceptions (SPEC 6.7), sorted. A request reads them
// when it starts, so a change applies from the next one.
func (p *Project) PrivateHosts(ctx context.Context) ([]string, error) {
	meta, err := p.DB.Meta(ctx)
	if err != nil {
		return nil, err
	}
	hosts := []string{}
	if v := meta[metaPrivateHosts]; v != "" {
		if err := json.Unmarshal([]byte(v), &hosts); err != nil {
			return nil, fmt.Errorf("project: %s: %w", metaPrivateHosts, err)
		}
	}
	return hosts, nil
}

// HostError is a host that can't be an exception: not a name or an
// address alone (web.ParseHost).
type HostError struct {
	Host string
	Err  error
}

func (e *HostError) Error() string {
	return "project: " + strings.TrimPrefix(e.Err.Error(), "web: ")
}

func (e *HostError) Unwrap() error { return e.Err }

// SetPrivateHosts replaces the exceptions and returns them as stored.
// Hosts are kept as requests name them (web.ParseHost): lower case, ASCII
// (punycode), addresses in their standard form. Empty ones are skipped;
// any other that isn't a host is a *HostError, and nothing changes.
func (p *Project) SetPrivateHosts(ctx context.Context, hosts []string) ([]string, error) {
	out := []string{}
	for _, h := range hosts {
		if strings.TrimSpace(h) == "" {
			continue
		}
		c, err := web.ParseHost(h)
		if err != nil {
			return nil, &HostError{Host: h, Err: err}
		}
		out = append(out, c)
	}
	slices.Sort(out)
	out = slices.Compact(out)
	p.hostsMu.Lock()
	defer p.hostsMu.Unlock()
	return out, p.storeHosts(ctx, out)
}

// AddPrivateHost adds one host to the exceptions, as an approval card's
// "Allow in this project" does (8.8).
func (p *Project) AddPrivateHost(ctx context.Context, host string) error {
	c, err := web.ParseHost(host)
	if err != nil {
		return &HostError{Host: host, Err: err}
	}
	p.hostsMu.Lock()
	defer p.hostsMu.Unlock()
	hs, err := p.PrivateHosts(ctx)
	if err != nil || slices.Contains(hs, c) {
		return err
	}
	hs = append(hs, c)
	slices.Sort(hs)
	return p.storeHosts(ctx, hs)
}

func (p *Project) storeHosts(ctx context.Context, hosts []string) error {
	b, err := json.Marshal(hosts)
	if err != nil {
		return err
	}
	return p.DB.SetMeta(ctx, map[string]string{metaPrivateHosts: string(b)})
}
