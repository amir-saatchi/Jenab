package view

import (
	"context"
	"errors"

	"github.com/amir-saatchi/jenab/internal/store"
)

// Env is what the checks of steps 3 to 5 read from a project: its saved
// configs, its tables, its files, and the prepared form of a query. A nil
// Env skips those steps, so only the config itself is checked.
type Env interface {
	// Config returns the live config id of kind ("" for either); false
	// when there is none.
	Config(ctx context.Context, kind store.ConfigKind, id string) (store.Config, bool, error)
	// IDs lists the live configs of kind, for "did you mean".
	IDs(ctx context.Context, kind store.ConfigKind) ([]string, error)
	// Tables lists the agent tables.
	Tables(ctx context.Context) ([]string, error)
	// Table describes the agent table name, without its row count; false
	// when there is none.
	Table(ctx context.Context, name string) (store.Table, bool, error)
	// QueryColumns runs the SQL check on q and returns its result columns
	// (store.ProjectDB.QueryColumns).
	QueryColumns(ctx context.Context, q string) ([]store.QueryColumn, error)
	// Object reports whether the bucket has a file at key.
	Object(ctx context.Context, key string) (bool, error)
}

// ProjectEnv is the Env of an open project.
type ProjectEnv struct{ DB *store.ProjectDB }

var _ Env = ProjectEnv{}

func (e ProjectEnv) Config(ctx context.Context, kind store.ConfigKind, id string) (store.Config, bool, error) {
	c, err := e.DB.Config(ctx, kind, id)
	if errors.Is(err, store.ErrNotFound) {
		return store.Config{}, false, nil
	}
	return c, err == nil, err
}

func (e ProjectEnv) IDs(ctx context.Context, kind store.ConfigKind) ([]string, error) {
	cs, err := e.DB.Configs(ctx, kind)
	if err != nil {
		return nil, err
	}
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.ID
	}
	return out, nil
}

func (e ProjectEnv) Tables(ctx context.Context) ([]string, error) { return e.DB.Tables(ctx) }

func (e ProjectEnv) Table(ctx context.Context, name string) (store.Table, bool, error) {
	t, err := e.DB.TableShape(ctx, name)
	if errors.Is(err, store.ErrNotFound) {
		return store.Table{}, false, nil
	}
	return t, err == nil, err
}

func (e ProjectEnv) QueryColumns(ctx context.Context, q string) ([]store.QueryColumn, error) {
	return e.DB.QueryColumns(ctx, q)
}

func (e ProjectEnv) Object(ctx context.Context, key string) (bool, error) {
	_, err := e.DB.HeadObject(ctx, key)
	if errors.Is(err, store.ErrNotFound) {
		return false, nil
	}
	return err == nil, err
}
