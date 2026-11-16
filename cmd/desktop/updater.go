package main

import (
	"context"
	"os"
	"slices"
	"time"

	"github.com/egoist/mygo"

	"github.com/amir-saatchi/jenab/internal/lifecycle"
	"github.com/amir-saatchi/jenab/internal/update"
)

// afterUpdate is the argument the copy an update restarts gets: it waits
// for the old copy to let go of the app lock instead of failing.
const afterUpdate = "--after-update"

// relaunched reports whether an update started this copy.
func relaunched(args []string) bool { return slices.Contains(args, afterUpdate) }

// lockWait is how long the restarted copy waits for the old one, whose
// shutdown waits for the chats (lifecycle.Shutdown).
const lockWait = time.Minute

// lockApp takes the app lock in root, waiting for the old copy when an
// update started this one. It locks once: a second lock in the same
// process would wait for the first.
func lockApp(root string, args []string) (*os.File, error) {
	if relaunched(args) {
		return lifecycle.LockRootWait(root, lockWait)
	}
	return lifecycle.LockRoot(root)
}

// mygoUpdater is MyGo's signed updater (mygo.Updater) for internal/update.
// It reads the feed and public key that `go tool mygo build` links in from
// mygo.json; builds without them, such as go run, are not Enabled. Every
// archive must carry an ed25519 signature that verifies against that key,
// its size must match the manifest, and it downloads over https (plain
// http only from the loopback interface).
type mygoUpdater struct{}

func (mygoUpdater) Enabled() bool { return mygo.Updater.Enabled() }

func (mygoUpdater) Check(ctx context.Context, prereleases bool) (update.Available, error) {
	mygo.Updater.SetPrereleases(prereleases)
	up, err := mygo.Updater.Check(ctx)
	if err != nil || up == nil {
		return nil, err
	}
	return mygoVersion{up}, nil
}

type mygoVersion struct{ u *mygo.Update }

func (v mygoVersion) Version() string { return v.u.Version }
func (v mygoVersion) Notes() string   { return v.u.Notes }
func (v mygoVersion) Install(ctx context.Context, progress func(done, total int64)) error {
	return v.u.Install(ctx, progress)
}

// relaunch quits the app the normal way and starts the installed version,
// which waits for this copy's lock (afterUpdate).
func relaunch() { mygo.App.Relaunch(afterUpdate) }
