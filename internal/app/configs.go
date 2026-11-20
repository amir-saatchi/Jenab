package app

import (
	"context"
	"time"

	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/project"
	"github.com/amir-saatchi/jenab/internal/store"
)

// ConfigService reads a project's views and pages (SPEC 5) for the native
// stores: the Pages tab lists the pages, and a page reads its config and
// those of its views. Changes come as the project:configs event
// (EventConfigs), for saves, deletes and undos alike. The agent saves and
// deletes them with its tools (P3-03); this service only reads.
type ConfigService struct {
	base
	projects *project.Manager
}

// ConfigItem is a view or a page in a list, without its YAML.
type ConfigItem struct {
	Kind      store.ConfigKind `json:"kind"`
	ID        string           `json:"id"`
	Type      string           `json:"type"` // table, chart, stat, form; page for a page
	Title     string           `json:"title"`
	Revision  int              `json:"revision"`
	UpdatedAt time.Time        `json:"updated_at"`
	UpdatedBy id.Source        `json:"updated_by"`
}

func configItem(c store.Config) ConfigItem {
	return ConfigItem{Kind: c.Kind, ID: c.ID, Type: c.Type, Title: c.Title, Revision: c.Revision, UpdatedAt: c.UpdatedAt, UpdatedBy: c.UpdatedBy}
}

// checkKind refuses a kind other than view and page; "" means both when
// allowed.
func checkKind(kind store.ConfigKind, any bool) error {
	if kind == store.KindView || kind == store.KindPage || (any && kind == "") {
		return nil
	}
	return &UIError{Kind: KindInvalid, Message: "Choose views or pages.", Details: "unknown config kind " + string(kind)}
}

// List lists the project's views or pages by ID; kind "" lists both,
// views first.
func (s *ConfigService) List(ctx context.Context, p id.Project, kind store.ConfigKind) (out []ConfigItem, err error) {
	defer s.guard("config.list", &err)
	if err := checkKind(kind, true); err != nil {
		return nil, err
	}
	kinds := store.ConfigKinds
	if kind != "" {
		kinds = []store.ConfigKind{kind}
	}
	out = []ConfigItem{}
	err = open(ctx, s.projects, p, func(proj *project.Project) error {
		for _, k := range kinds {
			cs, err := proj.DB.Configs(ctx, k)
			if err != nil {
				return err
			}
			for _, c := range cs {
				out = append(out, configItem(c))
			}
		}
		return nil
	})
	return out, err
}

// Read reads one view or page with its YAML; kind "" finds either. One
// that isn't there, or was deleted, is a not-found error.
func (s *ConfigService) Read(ctx context.Context, p id.Project, kind store.ConfigKind, cid string) (c store.Config, err error) {
	defer s.guard("config.read", &err)
	if err := checkKind(kind, true); err != nil {
		return c, err
	}
	err = open(ctx, s.projects, p, func(proj *project.Project) error {
		c, err = proj.DB.Config(ctx, kind, cid)
		return err
	})
	return c, err
}
