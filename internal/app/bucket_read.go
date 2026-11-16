package app

import (
	"context"
	"io"

	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/project"
)

// NativeBucket is the bucket as the native app reads it (SPEC 4.5): the
// BucketService, and Read, which hands a file's bytes to the screen in the
// same process. The app has no webview, so it reads files here, not
// through the objects route. It is its own type, so the services keep
// no Read.
type NativeBucket struct{ *BucketService }

// ObjectBytes is the start of a file's current version: Object is its
// head, Data at most the bytes asked for, and Cut tells that the file
// has more.
type ObjectBytes struct {
	Object
	Data []byte
	Cut  bool
}

// Read reads up to max bytes of a file's current version, leasing the
// project while it reads. The bytes are untrusted: the screen shows them
// as text or as a picture, by the type detected when the file was stored
// (4.7), and never runs them.
func (b NativeBucket) Read(ctx context.Context, p id.Project, key string, max int64) (ob ObjectBytes, err error) {
	s := b.BucketService
	defer s.guard("bucket.read", &err)
	err = open(ctx, s.projects, p, func(proj *project.Project) error {
		so, body, err := proj.DB.OpenObject(ctx, key)
		if err != nil {
			return err
		}
		defer body.Close()
		ob.Object = object(so)
		ob.Data, err = io.ReadAll(io.LimitReader(body, max))
		ob.Cut = so.Size > int64(len(ob.Data))
		return err
	})
	return ob, err
}
