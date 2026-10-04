package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/amir-saatchi/jenab/internal/bucket"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/limit"
)

// objectTables is project.db format step 2: the bucket index (SPEC 4.2)
// and the default lifecycle rules (SPEC 4.4). Every put adds a version
// row, so _jenab_object_versions holds the current version too.
var objectTables = execAll(
	`CREATE TABLE _jenab_objects (
		key        TEXT PRIMARY KEY,
		sha256     TEXT NOT NULL,
		mime       TEXT NOT NULL,
		size       INTEGER NOT NULL,
		metadata   TEXT NOT NULL DEFAULT '{}',
		source     TEXT NOT NULL,
		source_url TEXT NOT NULL DEFAULT '',
		created_by TEXT NOT NULL,
		updated_at TEXT NOT NULL
	)`,
	`CREATE TABLE _jenab_object_versions (
		key        TEXT NOT NULL,
		version    INTEGER NOT NULL,
		sha256     TEXT NOT NULL,
		size       INTEGER NOT NULL,
		created_by TEXT NOT NULL,
		created_at TEXT NOT NULL,
		PRIMARY KEY (key, version)
	) WITHOUT ROWID`,
	`INSERT INTO _jenab_meta (key, value) VALUES ('lifecycle', '`+defaultLifecycle+`')`,
)

// Lifecycle is the bucket's rules (SPEC 4.4), stored in _jenab_meta. Phase
// 1 reads only the expiry; keeping versions, the cache size cap and their
// cleanup come in Phase 3 (R-71).
type Lifecycle struct {
	Rules        []LifecycleRule `json:"rules"`
	CacheMaxSize int64           `json:"cache_max_size"` // bytes
}

type LifecycleRule struct {
	Prefix          string `json:"prefix"`
	ExpireAfterDays int    `json:"expire_after_days,omitempty"`
	KeepVersions    int    `json:"keep_versions,omitempty"`
}

const defaultLifecycle = `{"rules":[{"prefix":"cache/","expire_after_days":30},{"prefix":"","keep_versions":5}],"cache_max_size":524288000}`

// expires is when an object under key, last written at t, expires: the
// longest matching prefix with an expiry decides. Zero means never.
func (l Lifecycle) expires(key string, t time.Time) time.Time {
	best := -1
	var out time.Time
	for _, r := range l.Rules {
		if r.ExpireAfterDays > 0 && strings.HasPrefix(key, r.Prefix) && len(r.Prefix) > best {
			best, out = len(r.Prefix), t.AddDate(0, 0, r.ExpireAfterDays)
		}
	}
	return out
}

// MaxUpload is the default size limit of a put (SPEC 4.7).
const MaxUpload = 100 << 20

// maxMetadata limits an object's metadata, which is for small facts such
// as the original file name.
const maxMetadata = 16 << 10

// Object is a key's current version.
type Object struct {
	Key       string
	SHA256    string
	MIME      string
	Size      int64
	Version   int
	Metadata  json.RawMessage // a JSON object; never used as a path
	Source    string          // upload, http, pipeline, agent or tool
	SourceURL string
	CreatedBy id.Source
	UpdatedAt time.Time
	Expires   time.Time // zero if it never expires (SPEC 4.4)
}

// ObjectVersion is one stored version of a key.
type ObjectVersion struct {
	Version   int
	SHA256    string
	Size      int64
	CreatedBy id.Source
	CreatedAt time.Time
}

// PutOptions describe where content came from.
type PutOptions struct {
	Source    string // upload, http, pipeline, agent or tool (SPEC 4.2)
	SourceURL string
	Metadata  json.RawMessage
	MaxSize   int64 // 0 means MaxUpload
}

var objectSources = map[string]bool{"upload": true, "http": true, "pipeline": true, "agent": true, "tool": true}

// ErrObjectExists is returned by MoveObject when the new key is taken.
var ErrObjectExists = errors.New("store: an object already has that key")

// noObject is a missing key. It matches both ErrNotFound and
// bucket.ErrNotFound.
type noObject struct{ key string }

func (e *noObject) Error() string     { return "store: no object at " + e.key }
func (e *noObject) Is(err error) bool { return err == ErrNotFound || err == bucket.ErrNotFound }

// PutObject stores r under key in the write order of SPEC 4.3: the bytes
// first, then the index and version rows in one transaction. A crash in
// between leaves only bytes no row points to, which the sweep removes.
// Content equal to the current version adds no version; the other fields
// are updated.
func (p *ProjectDB) PutObject(ctx context.Context, pr limit.Priority, src id.Source, key string, r io.Reader, o PutOptions) (Object, error) {
	if err := bucket.CheckKey(key); err != nil {
		return Object{}, err
	}
	if !objectSources[o.Source] {
		return Object{}, fmt.Errorf("store: %q is not an object source", o.Source)
	}
	meta, err := checkMetadata(o.Metadata)
	if err != nil {
		return Object{}, err
	}
	if p.w == nil {
		return Object{}, ErrReadOnly // before any bytes are written
	}
	max := o.MaxSize
	if max <= 0 {
		max = MaxUpload
	}
	life := p.lifecycle(ctx)

	p.objMu.RLock()
	defer p.objMu.RUnlock()
	b, err := p.objects.Put(ctx, r, max)
	if err != nil {
		return Object{}, err
	}
	obj := Object{Key: key, SHA256: b.Hash, MIME: bucket.DetectMIME(b.Head, key), Size: b.Size, Metadata: meta,
		Source: o.Source, SourceURL: o.SourceURL, CreatedBy: src, UpdatedAt: time.Now().UTC().Truncate(time.Millisecond)}
	obj.Expires = life.expires(key, obj.UpdatedAt)
	return Do(ctx, p.DB, pr, func(tx *sql.Tx) (Object, error) {
		var cur sql.NullString
		var version int
		if err := tx.QueryRow(`SELECT (SELECT sha256 FROM _jenab_objects WHERE key = ?),
			(SELECT COALESCE(MAX(version), 0) FROM _jenab_object_versions WHERE key = ?)`, key, key).Scan(&cur, &version); err != nil {
			return Object{}, err
		}
		if cur.String != obj.SHA256 || !cur.Valid {
			version++
			if _, err := tx.Exec(`INSERT INTO _jenab_object_versions (key, version, sha256, size, created_by, created_at)
				VALUES (?, ?, ?, ?, ?, ?)`, key, version, obj.SHA256, obj.Size, string(src), formatTime(obj.UpdatedAt)); err != nil {
				return Object{}, err
			}
		}
		obj.Version = version
		_, err := tx.Exec(`INSERT INTO _jenab_objects (key, sha256, mime, size, metadata, source, source_url, created_by, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (key) DO UPDATE SET sha256 = excluded.sha256, mime = excluded.mime, size = excluded.size,
				metadata = excluded.metadata, source = excluded.source, source_url = excluded.source_url,
				created_by = excluded.created_by, updated_at = excluded.updated_at`,
			key, obj.SHA256, obj.MIME, obj.Size, string(meta), obj.Source, obj.SourceURL, string(src), formatTime(obj.UpdatedAt))
		return obj, err
	})
}

func checkMetadata(m json.RawMessage) (json.RawMessage, error) {
	if len(m) == 0 {
		return json.RawMessage("{}"), nil
	}
	if len(m) > maxMetadata {
		return nil, fmt.Errorf("store: object metadata is over %d bytes", maxMetadata)
	}
	var v map[string]any
	if err := json.Unmarshal(m, &v); err != nil || v == nil {
		return nil, errors.New("store: object metadata must be a JSON object")
	}
	return m, nil
}

const objectCols = `o.key, o.sha256, o.mime, o.size, o.metadata, o.source, o.source_url, o.created_by, o.updated_at,
	(SELECT MAX(version) FROM _jenab_object_versions v WHERE v.key = o.key)`

func scanObject(r interface{ Scan(...any) error }) (Object, error) {
	var o Object
	var meta, by, at string
	var version sql.NullInt64
	if err := r.Scan(&o.Key, &o.SHA256, &o.MIME, &o.Size, &meta, &o.Source, &o.SourceURL, &by, &at, &version); err != nil {
		return Object{}, err
	}
	o.Metadata, o.CreatedBy, o.Version = json.RawMessage(meta), id.Source(by), int(version.Int64)
	var err error
	o.UpdatedAt, err = parseTime(at)
	return o, err
}

// HeadObject returns key's current version.
func (p *ProjectDB) HeadObject(ctx context.Context, key string) (Object, error) {
	if err := bucket.CheckKey(key); err != nil {
		return Object{}, err
	}
	rows, err := Query(ctx, p.DB, "SELECT "+objectCols+" FROM _jenab_objects o WHERE o.key = ?", []any{key},
		func(r *sql.Rows) (Object, error) { return scanObject(r) })
	if err != nil {
		return Object{}, err
	}
	if len(rows) == 0 {
		return Object{}, &noObject{key}
	}
	o := rows[0]
	o.Expires = p.lifecycle(ctx).expires(o.Key, o.UpdatedAt)
	return o, nil
}

// OpenObject returns key's current version and its content. The caller
// closes the reader.
func (p *ProjectDB) OpenObject(ctx context.Context, key string) (Object, io.ReadSeekCloser, error) {
	p.objMu.RLock() // the row and the bytes it points to are read together
	defer p.objMu.RUnlock()
	o, err := p.HeadObject(ctx, key)
	if err != nil {
		return Object{}, nil, err
	}
	f, err := p.objects.Open(ctx, o.SHA256)
	if err != nil {
		return Object{}, nil, fmt.Errorf("store: object %s: %w", key, err)
	}
	return o, f, nil
}

// ObjectList is one page of ListObjects.
type ObjectList struct {
	Objects []Object
	Folders []string // common prefixes, ending in "/"
	Next    string   // pass as after for the next page; "" at the end
}

// ListObjects lists the keys under prefix, with "/" as the delimiter: keys
// directly under prefix are objects, deeper ones are grouped into folders.
// Objects and folders come in key order, at most max of them together,
// starting after after (a key or a folder from an earlier page).
func (p *ProjectDB) ListObjects(ctx context.Context, prefix, after string, max int) (ObjectList, error) {
	if err := bucket.CheckPrefix(prefix); err != nil {
		return ObjectList{}, err
	}
	if max <= 0 || max > 1000 {
		max = 1000
	}
	life := p.lifecycle(ctx)
	var out ObjectList
	from := after
	if strings.HasSuffix(from, "/") {
		from += bucket.KeyEnd // past every key in that folder
	}
	n := 0
	for {
		rows, err := Query(ctx, p.DB, "SELECT "+objectCols+` FROM _jenab_objects o
			WHERE o.key > ? AND o.key >= ? AND o.key < ? ORDER BY o.key LIMIT ?`,
			[]any{from, prefix, prefix + bucket.KeyEnd, max + 1}, func(r *sql.Rows) (Object, error) { return scanObject(r) })
		if err != nil {
			return ObjectList{}, err
		}
		if len(rows) == 0 {
			return out, nil
		}
		folder := ""
		for _, o := range rows {
			name := o.Key
			if i := strings.IndexByte(o.Key[len(prefix):], '/'); i >= 0 {
				folder = prefix + o.Key[len(prefix):][:i+1]
				name = folder
			}
			if n == max {
				out.Next = lastName(out)
				return out, nil
			}
			n++
			if folder != "" {
				out.Folders = append(out.Folders, folder)
				break
			}
			o.Expires = life.expires(o.Key, o.UpdatedAt)
			out.Objects = append(out.Objects, o)
			from = name
		}
		if folder != "" {
			from = folder + bucket.KeyEnd
			continue
		}
		if len(rows) <= max {
			return out, nil
		}
	}
}

// lastName is the greater of the last object and the last folder.
func lastName(l ObjectList) string {
	var k, f string
	if len(l.Objects) > 0 {
		k = l.Objects[len(l.Objects)-1].Key
	}
	if len(l.Folders) > 0 {
		f = l.Folders[len(l.Folders)-1]
	}
	if f > k {
		return f
	}
	return k
}

// ObjectVersions lists key's stored versions, newest first, including
// those of a deleted key.
func (p *ProjectDB) ObjectVersions(ctx context.Context, key string) ([]ObjectVersion, error) {
	if err := bucket.CheckKey(key); err != nil {
		return nil, err
	}
	return Query(ctx, p.DB, `SELECT version, sha256, size, created_by, created_at FROM _jenab_object_versions
		WHERE key = ? ORDER BY version DESC`, []any{key}, func(r *sql.Rows) (ObjectVersion, error) {
		var v ObjectVersion
		var by, at string
		if err := r.Scan(&v.Version, &v.SHA256, &v.Size, &by, &at); err != nil {
			return v, err
		}
		v.CreatedBy = id.Source(by)
		var err error
		v.CreatedAt, err = parseTime(at)
		return v, err
	})
}

// DeleteObject removes key. Its versions and bytes stay until the
// lifecycle rules and the sweep remove them (SPEC 4.3, 4.4), so the delete
// can be undone.
func (p *ProjectDB) DeleteObject(ctx context.Context, pr limit.Priority, src id.Source, key string) error {
	if err := bucket.CheckKey(key); err != nil {
		return err
	}
	_, err := Do(ctx, p.DB, pr, func(tx *sql.Tx) (struct{}, error) {
		res, err := tx.Exec(`DELETE FROM _jenab_objects WHERE key = ?`, key)
		if err != nil {
			return struct{}{}, err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return struct{}{}, &noObject{key}
		}
		return struct{}{}, nil
	})
	return err
}

// MoveObject renames from to to, with its versions. It fails with
// ErrObjectExists if to is taken. If to had versions from an earlier
// deleted object, the moved ones are numbered after them.
func (p *ProjectDB) MoveObject(ctx context.Context, pr limit.Priority, src id.Source, from, to string) (Object, error) {
	if err := bucket.CheckKey(from); err != nil {
		return Object{}, err
	}
	if err := bucket.CheckKey(to); err != nil {
		return Object{}, err
	}
	if from == to {
		return Object{}, fmt.Errorf("store: moving %s onto itself", from)
	}
	life := p.lifecycle(ctx)
	now := time.Now().UTC().Truncate(time.Millisecond)
	return Do(ctx, p.DB, pr, func(tx *sql.Tx) (Object, error) {
		var taken int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM _jenab_objects WHERE key = ?`, to).Scan(&taken); err != nil {
			return Object{}, err
		}
		if taken > 0 {
			return Object{}, fmt.Errorf("%w: %s", ErrObjectExists, to)
		}
		var offset int
		if err := tx.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM _jenab_object_versions WHERE key = ?`, to).Scan(&offset); err != nil {
			return Object{}, err
		}
		res, err := tx.Exec(`UPDATE _jenab_objects SET key = ?, created_by = ?, updated_at = ? WHERE key = ?`,
			to, string(src), formatTime(now), from)
		if err != nil {
			return Object{}, err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return Object{}, &noObject{from}
		}
		if _, err := tx.Exec(`UPDATE _jenab_object_versions SET key = ?, version = version + ? WHERE key = ?`, to, offset, from); err != nil {
			return Object{}, err
		}
		o, err := scanObject(tx.QueryRow("SELECT "+objectCols+" FROM _jenab_objects o WHERE o.key = ?", to))
		o.Expires = life.expires(o.Key, o.UpdatedAt)
		return o, err
	})
}

// SweepReport says what a sweep removed.
type SweepReport struct {
	Files int   // bytes no row points to
	Bytes int64 // their size
	Tmp   int   // temporary files older than a day
}

// SweepObjects removes the bytes that no current or kept version points
// to, such as those left by a crash between a put's bytes and its row, and
// temporary files older than a day (SPEC 4.4). Recovery runs it after a
// crash (SPEC 2.7). Phase 2 adds the changes inside the undo window to
// what keeps bytes.
func (p *ProjectDB) SweepObjects(ctx context.Context) (SweepReport, error) {
	if p.w == nil {
		return SweepReport{}, ErrReadOnly
	}
	var rep SweepReport
	// Look without the lock first, so puts aren't held up by the walk.
	used, err := p.usedHashes(ctx)
	if err != nil {
		return rep, err
	}
	var maybe []bucket.Stored
	for s, err := range p.objects.All(ctx) {
		if err != nil {
			return rep, err
		}
		if !used[s.Hash] {
			maybe = append(maybe, s)
		}
	}
	if len(maybe) > 0 {
		p.objMu.Lock()
		// Every put that held the lock has written its row by now.
		used, err = p.usedHashes(ctx)
		if err == nil {
			for _, s := range maybe {
				if used[s.Hash] {
					continue
				}
				if p.objects.Delete(ctx, s.Hash) == nil {
					rep.Files++
					rep.Bytes += s.Size
				}
			}
		}
		p.objMu.Unlock()
		if err != nil {
			return rep, err
		}
	}
	rep.Tmp, err = p.objects.CleanTmp(ctx, time.Now().Add(-24*time.Hour))
	return rep, err
}

func (p *ProjectDB) usedHashes(ctx context.Context) (map[string]bool, error) {
	hs, err := Query(ctx, p.DB, `SELECT sha256 FROM _jenab_objects UNION SELECT sha256 FROM _jenab_object_versions`, nil,
		func(r *sql.Rows) (string, error) {
			var h string
			return h, r.Scan(&h)
		})
	if err != nil {
		return nil, err
	}
	used := make(map[string]bool, len(hs))
	for _, h := range hs {
		used[h] = true
	}
	return used, nil
}

// lifecycle reads the rules; a missing or broken entry means no expiry.
func (p *ProjectDB) lifecycle(ctx context.Context) Lifecycle {
	var l Lifecycle
	rows, err := Query(ctx, p.DB, `SELECT value FROM _jenab_meta WHERE key = 'lifecycle'`, nil, func(r *sql.Rows) (string, error) {
		var v string
		return v, r.Scan(&v)
	})
	if err == nil && len(rows) == 1 {
		json.Unmarshal([]byte(rows[0]), &l)
	}
	return l
}
