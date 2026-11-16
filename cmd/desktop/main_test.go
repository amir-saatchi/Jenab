package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/amir-saatchi/jenab/internal/app"
	"github.com/amir-saatchi/jenab/internal/lifecycle"
)

func TestUploaderPostsTheFileAsTheRouteWants(t *testing.T) {
	path := filepath.Join(t.TempDir(), "یادداشت 1.md")
	if err := os.WriteFile(path, []byte("# Notes"), 0o600); err != nil {
		t.Fatal(err)
	}
	var gotPath, gotName, gotBody string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotName, _ = url.PathUnescape(r.Header.Get(app.UploadNameHeader))
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		json.NewEncoder(w).Encode(app.Uploaded{Key: "uploads/x.md", Link: "jenab://p1/uploads/x.md", Name: gotName, Size: int64(len(b))})
	})
	up, err := uploader(h)(context.Background(), "p1", path)
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/upload/p1" || gotName != "یادداشت 1.md" || gotBody != "# Notes" {
		t.Errorf("posted %q %q %q", gotPath, gotName, gotBody)
	}
	if up.Link != "jenab://p1/uploads/x.md" || up.Size != 7 {
		t.Errorf("got %+v", up)
	}

	refuse := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		w.Write([]byte(`{"error":"the project is open read-only, so files can't be added"}`))
	})
	if _, err := uploader(refuse)(context.Background(), "p1", path); err == nil || err.Error() != "the project is open read-only, so files can't be added" {
		t.Errorf("err = %v", err)
	}
}

// The copy an update starts takes the lock at once when the old copy is
// gone, and waits for it while it quits. Taking the lock twice, as the
// first version did, held the new copy until the garbage collector closed
// the first file (TASK-059).
func TestTheRestartedCopyLocksOnce(t *testing.T) {
	root := t.TempDir()
	start := time.Now()
	lock, err := lockApp(root, []string{afterUpdate})
	if err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Fatalf("took the free lock in %v", d)
	}
	if _, err := lockApp(root, nil); !errors.Is(err, lifecycle.ErrRunning) {
		t.Fatalf("a second copy: %v; want ErrRunning", err)
	}
	released := make(chan struct{})
	go func() {
		time.Sleep(300 * time.Millisecond)
		lock.Close()
		close(released)
	}()
	next, err := lockApp(root, []string{"--after-update"})
	if err != nil {
		t.Fatal(err)
	}
	<-released
	next.Close()
}
