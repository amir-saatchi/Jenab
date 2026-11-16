package app

import (
	"context"
	"strings"
	"testing"

	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/limit"
	"github.com/amir-saatchi/jenab/internal/store"
)

// The native app reads a file's bytes through the service (SPEC 4.5): the
// current version, cut at the limit, and not_found for a missing key.
func TestNativeBucketRead(t *testing.T) {
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
	for _, body := range []string{"one", "# یادداشت\n\nسلام"} {
		if _, err := proj.DB.PutObject(ctx, limit.Interactive, id.SourceUser, "notes/a.md", strings.NewReader(body), store.PutOptions{Source: "upload"}); err != nil {
			t.Fatal(err)
		}
	}
	proj.Release()

	nb := NativeBucket{e.svc.Bucket}
	ob, err := nb.Read(ctx, op.ID, "notes/a.md", 1<<20)
	if err != nil || string(ob.Data) != "# یادداشت\n\nسلام" || ob.Cut || ob.Version != 2 || ob.Key != "notes/a.md" {
		t.Fatalf("Read = %+v, %v", ob, err)
	}
	ob, err = nb.Read(ctx, op.ID, "notes/a.md", 4)
	if err != nil || len(ob.Data) != 4 || !ob.Cut {
		t.Fatalf("Read cut = %q %v, %v", ob.Data, ob.Cut, err)
	}
	if _, err := nb.Read(ctx, op.ID, "notes/gone.md", 10); err == nil {
		t.Fatal("a missing file was read")
	} else {
		uiErr(t, err, KindNotFound)
	}
}
