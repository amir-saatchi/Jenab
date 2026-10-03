package project

import (
	"encoding/json"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
)

// FolderWarning says why the data folder is a bad place for SQLite files:
// a network drive, or a folder a sync service copies file by file, which
// can copy the -wal file apart from its database and corrupt it (SPEC 2.1).
type FolderWarning struct {
	Folder  string `json:"folder"`
	Network bool   `json:"network"`
	Service string `json:"service,omitempty"` // OneDrive, Dropbox, iCloud Drive or Google Drive
	Text    string `json:"text"`
}

// CheckFolder returns a warning for dir, or nil. Links are followed first,
// so a link to a synced folder is caught. dir need not exist yet.
func CheckFolder(dir string) *FolderWarning {
	if dir == "" {
		return nil
	}
	p := resolve(dir)
	if isNetwork(p) {
		return &FolderWarning{Folder: dir, Network: true,
			Text: "The data folder is on a network drive. SQLite needs a local disk, so projects may be damaged. Choose a local folder in Settings."}
	}
	home, _ := os.UserHomeDir()
	s := syncedBy(p, syncRules(runtime.GOOS, home, os.Getenv, os.ReadFile), caseless(runtime.GOOS))
	if s == "" {
		s = volumeService(p)
	}
	if s == "" {
		return nil
	}
	return &FolderWarning{Folder: dir, Service: s,
		Text: "The data folder is inside a folder synced by " + s + ". Syncing can damage project databases. Choose a folder outside it in Settings, and back up with Export."}
}

// resolve makes dir absolute and follows links in the part that exists.
func resolve(dir string) string {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return dir
	}
	rest := ""
	for p := abs; ; {
		if r, err := filepath.EvalSymlinks(p); err == nil {
			return filepath.Join(r, rest)
		}
		parent := filepath.Dir(p)
		if parent == p {
			return abs
		}
		rest = filepath.Join(filepath.Base(p), rest)
		p = parent
	}
}

// syncRule matches dir and everything below it or, with prefix set, every
// child of dir whose name starts with prefix.
type syncRule struct {
	dir, prefix, service string
}

// syncRules lists the usual places of the sync services on goos. The
// paths are in slash form. getenv and readFile are os.Getenv and
// os.ReadFile, swapped in tests.
func syncRules(goos, home string, getenv func(string) string, readFile func(string) ([]byte, error)) []syncRule {
	var rs []syncRule
	if home != "" {
		h := filepath.ToSlash(home)
		rs = append(rs,
			syncRule{h, "OneDrive", "OneDrive"}, // also "OneDrive - Company"
			syncRule{h, "Dropbox", "Dropbox"},   // also "Dropbox (Personal)"
			syncRule{h, "Google Drive", "Google Drive"},
			syncRule{h, "My Drive", "Google Drive"},
		)
		switch goos {
		case "windows":
			rs = append(rs, syncRule{h, "iCloudDrive", "iCloud Drive"})
		case "darwin":
			cs := path.Join(h, "Library", "CloudStorage") // File Provider folders
			rs = append(rs,
				syncRule{path.Join(h, "Library", "Mobile Documents"), "", "iCloud Drive"},
				syncRule{cs, "OneDrive", "OneDrive"},
				syncRule{cs, "Dropbox", "Dropbox"},
				syncRule{cs, "GoogleDrive", "Google Drive"},
			)
		}
	}
	if goos == "windows" {
		for _, v := range []string{"OneDrive", "OneDriveConsumer", "OneDriveCommercial"} {
			if d := getenv(v); d != "" {
				rs = append(rs, syncRule{filepath.ToSlash(d), "", "OneDrive"})
			}
		}
	}
	// Dropbox can be moved anywhere; its info.json says where.
	var infos []string
	if goos == "windows" {
		for _, v := range []string{"APPDATA", "LOCALAPPDATA"} {
			if d := getenv(v); d != "" {
				infos = append(infos, path.Join(filepath.ToSlash(d), "Dropbox", "info.json"))
			}
		}
	} else if home != "" {
		infos = append(infos, path.Join(filepath.ToSlash(home), ".dropbox", "info.json"))
	}
	for _, f := range infos {
		b, err := readFile(f)
		if err != nil {
			continue
		}
		var info map[string]struct {
			Path string `json:"path"`
		}
		if json.Unmarshal(b, &info) != nil {
			continue
		}
		for _, a := range info {
			if a.Path != "" {
				rs = append(rs, syncRule{filepath.ToSlash(a.Path), "", "Dropbox"})
			}
		}
	}
	return rs
}

// syncedBy returns the service whose folder holds p, or "".
func syncedBy(p string, rules []syncRule, caseless bool) string {
	p = filepath.ToSlash(p)
	norm := func(s string) string {
		s = strings.TrimSuffix(s, "/")
		if caseless {
			s = strings.ToLower(s)
		}
		return s
	}
	np := norm(p)
	for _, r := range rules {
		d := norm(r.dir)
		if d == "" {
			continue
		}
		if r.prefix == "" {
			if np == d || strings.HasPrefix(np, d+"/") {
				return r.service
			}
			continue
		}
		rest, ok := strings.CutPrefix(np, d+"/")
		if !ok {
			continue
		}
		first, _, _ := strings.Cut(rest, "/")
		if strings.HasPrefix(first, norm(r.prefix)) {
			return r.service
		}
	}
	return ""
}

// caseless reports whether paths on goos usually ignore case.
func caseless(goos string) bool { return goos == "windows" || goos == "darwin" }

// uncNetwork reports a Windows network path: \\server\share or
// \\?\UNC\server\share, but not \\?\C:\ or \\.\ device paths.
func uncNetwork(p string) bool {
	p = strings.ReplaceAll(p, "/", `\`)
	switch {
	case strings.HasPrefix(strings.ToUpper(p), `\\?\UNC\`):
		return true
	case strings.HasPrefix(p, `\\?\`), strings.HasPrefix(p, `\\.\`):
		return false
	}
	return strings.HasPrefix(p, `\\`)
}

// networkMagic reports a Linux statfs type of a network file system.
func networkMagic(t uint32) bool {
	switch t {
	case 0x6969, // NFS
		0x517B,     // SMB
		0xFF534D42, // CIFS
		0xFE534D42, // SMB2
		0x5346414F, // AFS
		0x564C,     // NCP
		0x47504653, // GPFS
		0x00C36400, // Ceph
		0x013111A8, // IBRIX
		0x6B414653: // kAFS
		return true
	}
	return false
}

// networkFSType reports a macOS file system type name of a network file
// system.
func networkFSType(name string) bool {
	switch name {
	case "nfs", "smbfs", "afpfs", "webdav", "cifs", "ftp":
		return true
	}
	return false
}
