package app

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/config"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/provider"
	"github.com/amir-saatchi/jenab/internal/store"
)

// UsageReport is *Settings → Usage* (SPEC 3.9): tokens per day, per chat
// and per model, with the cost from the catalog prices.
type UsageReport struct {
	Days   []UsageLine `json:"days"`   // every day of the range, oldest first
	Chats  []UsageLine `json:"chats"`  // most tokens first
	Models []UsageLine `json:"models"` // most tokens first
	Total  UsageLine   `json:"total"`
	// Skipped are the projects whose chats couldn't be read, by name.
	Skipped []string `json:"skipped"`
}

// UsageLine is the tokens of one day, chat or model.
type UsageLine struct {
	Day        string  `json:"day,omitempty"`      // YYYY-MM-DD, for a day
	Project    string  `json:"project,omitempty"`  // the project's name, for a chat
	Chat       string  `json:"chat,omitempty"`     // the chat's title, for a chat
	Model      string  `json:"model,omitempty"`    // the catalog's name, else the model ID
	Provider   string  `json:"provider,omitempty"` // the provider's name, for a model
	Input      int     `json:"input"`              // not read from the cache
	Output     int     `json:"output"`
	CacheRead  int     `json:"cache_read"`
	CacheWrite int     `json:"cache_write"`
	Cost       float64 `json:"cost"`     // US dollars, for the tokens with a price
	Unpriced   int     `json:"unpriced"` // tokens of models the catalog has no price for
}

func (l *UsageLine) add(u chat.Usage, p *provider.Price) {
	l.Input += u.Input
	l.Output += u.Output
	l.CacheRead += u.CacheRead
	l.CacheWrite += u.CacheWrite
	if p == nil {
		l.Unpriced += tokens(u)
		return
	}
	// A cache price the catalog doesn't give is taken as the input price.
	read, write := cmp.Or(p.CacheRead, p.Input), cmp.Or(p.CacheWrite, p.Input)
	l.Cost += (float64(u.Input)*p.Input + float64(u.Output)*p.Output +
		float64(u.CacheRead)*read + float64(u.CacheWrite)*write) / 1e6
}

func (l UsageLine) tokens() int { return l.Input + l.Output + l.CacheRead + l.CacheWrite }

func tokens(u chat.Usage) int { return u.Input + u.Output + u.CacheRead + u.CacheWrite }

// Usage sums the tokens of the last days days, today included, across
// every project.
func (s *SettingsService) Usage(ctx context.Context, days int) (r UsageReport, err error) {
	defer s.guard("settings.usage", &err)
	days = min(max(days, 1), 366)
	now := s.now()
	_, off := now.Zone()
	y, m, d := now.Date()
	from := time.Date(y, m, d-days+1, 0, 0, 0, 0, now.Location())

	r = UsageReport{Days: []UsageLine{}, Chats: []UsageLine{}, Models: []UsageLine{}, Skipped: []string{}}
	byDay := map[string]int{}
	for i := range days {
		day := from.AddDate(0, 0, i).Format(time.DateOnly)
		byDay[day] = i
		r.Days = append(r.Days, UsageLine{Day: day})
	}
	type chatKey struct {
		p  id.Project
		ch id.Chat
	}
	chats := map[chatKey]*UsageLine{}
	models := map[string]*UsageLine{}
	llm := s.settings.Get().LLM

	ps, err := s.registry.Projects(ctx)
	if err != nil {
		return r, err
	}
	for _, p := range ps {
		rows, titles, err := projectUsage(ctx, p.Folder, from, time.Duration(off)*time.Second)
		if err != nil {
			if ctx.Err() != nil {
				return r, ctx.Err()
			}
			s.log.Warn("app: usage of a project not read", "project", p.ID, "err", s.redact(err.Error()))
			r.Skipped = append(r.Skipped, p.Name)
			continue
		}
		for _, row := range rows {
			price := priceOf(llm, s.models.Catalog(), row.Model)
			if i, ok := byDay[row.Day]; ok {
				r.Days[i].add(row.Usage, price)
			}
			k := chatKey{p.ID, row.Chat}
			if chats[k] == nil {
				chats[k] = &UsageLine{Project: p.Name, Chat: titles[row.Chat]}
			}
			chats[k].add(row.Usage, price)
			if models[row.Model] == nil {
				models[row.Model] = modelLine(llm, s.models.Catalog(), row.Model)
			}
			models[row.Model].add(row.Usage, price)
			r.Total.add(row.Usage, price)
		}
	}
	most := func(a, b UsageLine) int { return cmp.Compare(b.tokens(), a.tokens()) }
	for _, l := range chats {
		r.Chats = append(r.Chats, *l)
	}
	for _, l := range models {
		r.Models = append(r.Models, *l)
	}
	slices.SortStableFunc(r.Chats, func(a, b UsageLine) int {
		return cmp.Or(most(a, b), cmp.Compare(a.Project, b.Project), cmp.Compare(a.Chat, b.Chat))
	})
	slices.SortStableFunc(r.Models, func(a, b UsageLine) int {
		return cmp.Or(most(a, b), cmp.Compare(a.Provider, b.Provider), cmp.Compare(a.Model, b.Model))
	})
	return r, nil
}

// projectUsage reads one project's usage rows and chat titles.
func projectUsage(ctx context.Context, dir string, from time.Time, offset time.Duration) ([]store.UsageRow, map[id.Chat]string, error) {
	db, err := store.OpenChatsReadOnly(ctx, dir)
	if err != nil {
		return nil, nil, err
	}
	rows, err := db.Usage(ctx, from, offset)
	var cs []chat.Chat
	if err == nil {
		cs, err = db.Chats(ctx)
	}
	err = errors.Join(err, db.Close(ctx))
	titles := map[id.Chat]string{}
	for _, c := range cs {
		titles[c.ID] = c.Title
	}
	return rows, titles, err
}

// priceOf is the catalog price of a "provider/model" ref, or nil. A
// provider that was removed has no kind, so no price.
func priceOf(llm config.LLMSettings, cat *provider.Catalog, ref string) *provider.Price {
	name, mid, _ := strings.Cut(ref, "/")
	ps, ok := llm.Providers[name]
	if !ok {
		return nil
	}
	m, ok := cat.Find(provider.Kind(ps.Kind), mid)
	if !ok {
		return nil
	}
	return m.Price
}

func modelLine(llm config.LLMSettings, cat *provider.Catalog, ref string) *UsageLine {
	name, mid, _ := strings.Cut(ref, "/")
	l := &UsageLine{Provider: name, Model: mid}
	if ps, ok := llm.Providers[name]; ok {
		if m, ok := cat.Find(provider.Kind(ps.Kind), mid); ok {
			l.Model = m.Name
		}
	}
	return l
}
