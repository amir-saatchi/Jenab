package store

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"sort"
)

// StoredObject is a file in objects/ that a copy of project.db points to.
type StoredObject struct {
	Hash string
	Size int64
}

// CopyEdit is what an export changes in its copy of project.db.
type CopyEdit struct {
	Meta          map[string]string // keys to set in _jenab_meta, e.g. a new project ID
	DropMeta      []string          // keys to remove from _jenab_meta
	DropApprovals bool              // empty _jenab_approvals
}

// ExportCopy copies project.db to target with the safe-copy rule, as of
// one moment, for an export (SPEC 2.1, 7.5). Before the copy gets its final
// name, e is applied to it, in the same transaction. It returns the files
// in objects/ the copy points to, by hash: the same set the sweep keeps,
// so the export carries every byte its index needs.
func (p *ProjectDB) ExportCopy(ctx context.Context, target string, e CopyEdit) ([]StoredObject, error) {
	var objs []StoredObject
	err := vacuumInto(ctx, p.path, target, func(tx *sql.Tx) error {
		keys := make([]string, 0, len(e.Meta))
		for k := range e.Meta {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if _, err := tx.Exec(`INSERT INTO _jenab_meta (key, value) VALUES (?, ?)
				ON CONFLICT (key) DO UPDATE SET value = excluded.value`, k, e.Meta[k]); err != nil {
				return err
			}
		}
		for _, k := range e.DropMeta {
			if _, err := tx.Exec(`DELETE FROM _jenab_meta WHERE key = ?`, k); err != nil {
				return err
			}
		}
		if e.DropApprovals {
			if _, err := tx.Exec(`DELETE FROM _jenab_approvals`); err != nil {
				return err
			}
		}
		// As usedHashes, with sizes. A hash has one size, so max() only
		// picks a row.
		rows, err := tx.Query(`SELECT sha256, max(size) FROM (
				SELECT sha256, size FROM _jenab_objects
				UNION ALL SELECT sha256, size FROM _jenab_object_versions)
			GROUP BY sha256 ORDER BY sha256`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var o StoredObject
			if err := rows.Scan(&o.Hash, &o.Size); err != nil {
				return err
			}
			objs = append(objs, o)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return objs, nil
}

// ExportCopy copies chats.db to target with the safe-copy rule, as of one
// moment, for an export (SPEC 2.1, 7.5).
func (c *ChatsDB) ExportCopy(ctx context.Context, target string) error {
	return vacuumInto(ctx, c.path, target, func(*sql.Tx) error { return nil })
}

// OpenStored reads the file in objects/ with hash, for an export.
func (p *ProjectDB) OpenStored(ctx context.Context, hash string) (io.ReadSeekCloser, error) {
	return p.objects.Open(ctx, hash)
}

// editCopy runs edit in one transaction on the copy at path, on its own
// connection, and closes it again, so no -wal or -shm file stays.
func editCopy(ctx context.Context, path string, edit func(*sql.Tx) error) error {
	db, err := sql.Open("sqlite", dsn(path, false, common...))
	if err != nil {
		return err
	}
	db.SetMaxOpenConns(1)
	c, err := db.Conn(ctx)
	if err != nil {
		db.Close()
		return err
	}
	// A copy of a WAL file is marked WAL too. As a rollback-journal file
	// it is one file, which opens even where no -shm can be made; the
	// writer turns WAL on again when the copy is opened as a project.
	_, err = c.ExecContext(ctx, "PRAGMA journal_mode = DELETE")
	if err == nil {
		_, err = inTx(ctx, c, func(tx *sql.Tx) (struct{}, error) { return struct{}{}, edit(tx) })
	}
	if cerr := c.Close(); err == nil {
		err = cerr
	}
	if cerr := db.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return fmt.Errorf("editing the copy: %w", err)
	}
	return nil
}
