package app

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/limit"
	"github.com/amir-saatchi/jenab/internal/store"
)

func TestBucket(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t, t.TempDir())
	op, err := e.svc.Project.Create(ctx, "Files")
	if err != nil {
		t.Fatal(err)
	}
	proj, err := e.pm.Open(ctx, op.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := proj.DB.PutObject(ctx, limit.Interactive, id.SourceUser, "notes/a.txt", strings.NewReader("one"), store.PutOptions{Source: "upload"}); err != nil {
		t.Fatal(err)
	}
	if _, err := proj.DB.PutObject(ctx, limit.Interactive, id.SourceUser, "notes/a.txt", strings.NewReader("two"), store.PutOptions{Source: "upload"}); err != nil {
		t.Fatal(err)
	}
	proj.Release()

	pg, err := e.svc.Bucket.List(ctx, op.ID, "", "")
	if err != nil || len(pg.Objects) != 0 || !slices.Equal(pg.Folders, []string{"notes/"}) {
		t.Fatalf("List = %+v, %v", pg, err)
	}
	pg, err = e.svc.Bucket.List(ctx, op.ID, "notes/", "")
	if err != nil || len(pg.Objects) != 1 || pg.Objects[0].Key != "notes/a.txt" || pg.Objects[0].Version != 2 || pg.Objects[0].Expires != nil {
		t.Fatalf("List notes/ = %+v, %v", pg, err)
	}
	if vs, err := e.svc.Bucket.Versions(ctx, op.ID, "notes/a.txt"); err != nil || len(vs) != 2 {
		t.Fatalf("Versions = %+v, %v", vs, err)
	}
	if _, err := e.svc.Bucket.List(ctx, op.ID, "/bad", ""); err == nil {
		t.Fatal("a bad prefix")
	} else {
		uiErr(t, err, KindInvalid)
	}

	// The objects route, as Wails mounts it: the route cut off the path.
	h := objectHandler(e.pm)
	for _, path := range []string{"/" + string(op.ID) + "/notes/a.txt", objectPath(op.ID, "notes/a.txt")} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		body, _ := io.ReadAll(rec.Body)
		if rec.Code != http.StatusOK || string(body) != "two" || rec.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Errorf("%s: %d %q", path, rec.Code, body)
		}
	}
	for _, path := range []string{"/" + string(op.ID) + "/notes/none.txt", "/" + id.New() + "/notes/a.txt"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s: %d", path, rec.Code)
		}
	}
	if e.pm.Busy() {
		t.Error("the objects route kept a lease")
	}
}

func objectPath(p id.Project, key string) string { return "/objects/" + string(p) + "/" + key }
