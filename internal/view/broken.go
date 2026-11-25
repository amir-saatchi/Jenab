package view

import (
	"context"
	"fmt"
	"sync"

	"github.com/amir-saatchi/jenab/internal/store"
)

// Broken views (SPEC 5.7, 7.5, P3-04): a saved view that no longer
// passes validation against the project, as one a migration left as it
// was because it failed before it too, or one an undo or a change of the
// app broke. The project card marks it, and it gives an error on its next
// load in place of its data.

// InvalidError is a saved view that fails validation now, with the
// issues as validation words them.
type InvalidError struct {
	ID, Type, Title string
	Issues          []string
}

func (e *InvalidError) Error() string {
	s := fmt.Sprintf("The view %s no longer works with the project", e.ID)
	if len(e.Issues) > 0 {
		s += ": " + e.Issues[0]
		if n := len(e.Issues) - 1; n > 0 {
			s += fmt.Sprintf(" (and %d more)", n)
		}
	}
	return s + ". Ask the agent to fix it."
}

// brokenCache holds each project's broken views as of its schema version
// and config stamp, which a migration and every config save raise, and
// its objects stamp, as a view may name a file in the bucket; a change to
// any of them validates the views again. It forgets every project when it
// holds too many, as closed projects stay in it.
var brokenCache struct {
	mu sync.Mutex
	m  map[*store.ProjectDB]brokenEntry
}

type brokenEntry struct {
	schema  int
	stamp   int64
	objects string
	issues  map[string][]string
}

const brokenProjects = 16

// BrokenViews validates db's saved views against the project as it is now
// and gives the issues of each that fails, by view ID; none when every
// view passes. The map is shared: callers don't change it.
func BrokenViews(ctx context.Context, db *store.ProjectDB) (map[string][]string, error) {
	schema, err := db.SchemaVersion(ctx)
	if err != nil {
		return nil, err
	}
	stamp, err := db.ConfigStamp(ctx)
	if err != nil {
		return nil, err
	}
	objects, err := db.ObjectStamp(ctx)
	if err != nil {
		return nil, err
	}
	brokenCache.mu.Lock()
	e, ok := brokenCache.m[db]
	brokenCache.mu.Unlock()
	if ok && e.schema == schema && e.stamp == stamp && e.objects == objects {
		return e.issues, nil
	}
	vs, err := db.Configs(ctx, store.KindView)
	if err != nil {
		return nil, err
	}
	v := &Validator{Env: ProjectEnv{DB: db}}
	out := map[string][]string{}
	for _, c := range vs {
		issues, err := configIssues(ctx, v, c)
		if err != nil {
			return nil, err
		}
		if len(issues) > 0 {
			out[c.ID] = issues
		}
	}
	brokenCache.mu.Lock()
	if brokenCache.m == nil || len(brokenCache.m) >= brokenProjects {
		brokenCache.m = map[*store.ProjectDB]brokenEntry{}
	}
	brokenCache.m[db] = brokenEntry{schema: schema, stamp: stamp, objects: objects, issues: out}
	brokenCache.mu.Unlock()
	return out, nil
}

// Invalid is an *InvalidError for the saved view c when it fails
// validation now, and nil when it passes.
func Invalid(ctx context.Context, db *store.ProjectDB, c store.Config) error {
	broken, err := BrokenViews(ctx, db)
	if err != nil {
		return err
	}
	if issues := broken[c.ID]; len(issues) > 0 {
		return &InvalidError{ID: c.ID, Type: c.Type, Title: c.Title, Issues: issues}
	}
	return nil
}
