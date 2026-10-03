package project

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestSyncedFolders(t *testing.T) {
	env := map[string]string{
		"OneDriveCommercial": `D:\Work\OneDrive - Nord`,
		"APPDATA":            `C:\Users\amir\AppData\Roaming`,
	}
	files := map[string]string{
		"C:/Users/amir/AppData/Roaming/Dropbox/info.json": `{"personal": {"path": "E:\\Box\\Dropbox", "host": 1}}`,
		"/home/amir/.dropbox/info.json":                   `{"business": {"path": "/data/Dropbox (Nord)"}}`,
	}
	getenv := func(k string) string { return env[k] }
	readFile := func(f string) ([]byte, error) {
		if s, ok := files[f]; ok {
			return []byte(s), nil
		}
		return nil, os.ErrNotExist
	}
	tests := []struct {
		goos, home, path, want string
	}{
		// Windows
		{"windows", `C:\Users\amir`, `C:\Users\amir\OneDrive\Jenab`, "OneDrive"},
		{"windows", `C:\Users\amir`, `C:\Users\amir\onedrive - Nord\Documents\Jenab`, "OneDrive"},
		{"windows", `C:\Users\amir`, `D:\Work\OneDrive - Nord\Jenab`, "OneDrive"}, // moved, from the environment
		{"windows", `C:\Users\amir`, `E:\Box\Dropbox\Jenab`, "Dropbox"},           // moved, from info.json
		{"windows", `C:\Users\amir`, `C:\Users\amir\iCloudDrive\Jenab`, "iCloud Drive"},
		{"windows", `C:\Users\amir`, `C:\Users\amir\Google Drive\Jenab`, "Google Drive"},
		{"windows", `C:\Users\amir`, `C:\Users\amir\Jenab`, ""},
		{"windows", `C:\Users\amir`, `C:\Users\amir\Documents\OneDrive notes\Jenab`, ""}, // only folders right in home
		{"windows", `C:\Users\amir`, `D:\Work\Jenab`, ""},
		// macOS: File Provider folders, iCloud, and the old places
		{"darwin", "/Users/amir", "/Users/amir/Library/CloudStorage/OneDrive-Personal/Jenab", "OneDrive"},
		{"darwin", "/Users/amir", "/Users/amir/Library/CloudStorage/GoogleDrive-amir@example.com/My Drive/Jenab", "Google Drive"},
		{"darwin", "/Users/amir", "/Users/amir/Library/CloudStorage/Dropbox/Jenab", "Dropbox"},
		{"darwin", "/Users/amir", "/Users/amir/Library/Mobile Documents/com~apple~CloudDocs/Jenab", "iCloud Drive"},
		{"darwin", "/Users/amir", "/Users/amir/dropbox/Jenab", "Dropbox"}, // case-insensitive
		{"darwin", "/Users/amir", "/Users/amir/Jenab", ""},
		{"darwin", "/Users/amir", "/Users/amir/Library/Application Support/Jenab", ""},
		// Linux
		{"linux", "/home/amir", "/home/amir/Dropbox/Jenab", "Dropbox"},
		{"linux", "/home/amir", "/home/amir/OneDrive/Jenab", "OneDrive"},
		{"linux", "/home/amir", "/data/Dropbox (Nord)/Jenab", "Dropbox"}, // from info.json
		{"linux", "/home/amir", "/home/amir/dropbox/Jenab", ""},          // case matters
		{"linux", "/home/amir", "/home/amir/Jenab", ""},
	}
	for _, tt := range tests {
		got := syncedBy(tt.path, syncRules(tt.goos, tt.home, getenv, readFile), caseless(tt.goos))
		if got != tt.want {
			t.Errorf("%s %s: got %q, want %q", tt.goos, tt.path, got, tt.want)
		}
	}
}

func TestNetworkNames(t *testing.T) {
	for p, want := range map[string]bool{
		`\\nas\share\Jenab`:       true,
		`//nas/share/Jenab`:       true,
		`\\?\UNC\nas\share\Jenab`: true,
		`\\?\unc\nas\share`:       true,
		`\\?\C:\Users\amir\Jenab`: false,
		`\\.\PhysicalDrive0`:      false,
		`C:\Users\amir\Jenab`:     false,
		`/home/amir/Jenab`:        false,
	} {
		if got := uncNetwork(p); got != want {
			t.Errorf("uncNetwork(%q) = %v", p, got)
		}
	}
	for m, want := range map[uint32]bool{0x6969: true, 0xFF534D42: true, 0xFE534D42: true, 0xEF53: false, 0x9123683E: false, 0x01021994: false} {
		if networkMagic(m) != want {
			t.Errorf("networkMagic(%#x) = %v", m, !want)
		}
	}
	for n, want := range map[string]bool{"smbfs": true, "nfs": true, "afpfs": true, "webdav": true, "apfs": false, "hfs": false, "msdos": false} {
		if networkFSType(n) != want {
			t.Errorf("networkFSType(%q) = %v", n, !want)
		}
	}
}

// TestLocalFolderIsFine checks a real local folder; folder_windows_test.go
// has the OneDrive and network cases.
func TestLocalFolderIsFine(t *testing.T) {
	if w := CheckFolder(filepath.Join(t.TempDir(), "not", "yet")); w != nil {
		t.Errorf("local temp folder: %+v", w)
	}
}

func TestResolveFollowsLinks(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "Dropbox")
	os.Mkdir(target, 0o755)
	link := filepath.Join(dir, "Jenab")
	if err := os.Symlink(target, link); err != nil {
		if errors.Is(err, os.ErrPermission) || runtime.GOOS == "windows" {
			t.Skipf("no symlinks here: %v", err)
		}
		t.Fatal(err)
	}
	got := resolve(filepath.Join(link, "projects"))
	want, _ := filepath.EvalSymlinks(target)
	if got != filepath.Join(want, "projects") {
		t.Errorf("resolve = %s, want %s", got, filepath.Join(want, "projects"))
	}
}
