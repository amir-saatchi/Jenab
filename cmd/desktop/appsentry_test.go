package main

import (
	"errors"
	"testing"
)

// entry is an Apps entry in memory.
type entry map[string]string

func (e entry) GetStringValue(name string) (string, uint32, error) {
	v, ok := e[name]
	if !ok {
		return "", 0, errors.New("no value " + name)
	}
	return v, 1, nil
}

func (e entry) SetStringValue(name, value string) error { e[name] = value; return nil }

func TestTheAppsEntryTakesTheVersionAfterAnUpdate(t *testing.T) {
	const dir = `C:\Users\someone\AppData\Local\Programs\Jenab`
	for _, c := range []struct {
		name, loc, have, version, want string
		wrote                          bool
	}{
		{"updated", dir, "0.1.1", "0.1.2", "0.1.2", true},
		{"the folder in another case, quoted, with a separator", `"c:\users\someone\appdata\local\programs\jenab\"`, "0.1.1", "0.1.2", "0.1.2", true},
		{"up to date", dir, "0.1.2", "0.1.2", "0.1.2", false},
		{"another copy, as a build in the repo", `D:\src\jenab\bin`, "0.1.1", "0.1.2", "0.1.1", false},
		{"a build without a version", dir, "0.1.1", "dev", "0.1.1", false},
		{"no folder in the entry", "", "0.1.1", "0.1.2", "0.1.1", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			e := entry{"InstallLocation": c.loc, "DisplayVersion": c.have, "DisplayName": "Jenab"}
			wrote, err := refreshEntry(e, dir, c.version)
			if err != nil {
				t.Fatal(err)
			}
			if wrote != c.wrote || e["DisplayVersion"] != c.want {
				t.Errorf("wrote %v, version %q; want %v, %q", wrote, e["DisplayVersion"], c.wrote, c.want)
			}
			if e["DisplayName"] != "Jenab" || e["InstallLocation"] != c.loc {
				t.Errorf("other values changed: %v", e)
			}
		})
	}
	// An entry with no version gets one.
	e := entry{"InstallLocation": dir}
	if wrote, err := refreshEntry(e, dir, "0.1.2"); err != nil || !wrote || e["DisplayVersion"] != "0.1.2" {
		t.Errorf("wrote %v, %v: %v", wrote, err, e)
	}
}
