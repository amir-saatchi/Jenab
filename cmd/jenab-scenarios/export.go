package main

import (
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/amir-saatchi/jenab/internal/scenario"
)

// export gathers the passing runs' examples of training sets into
// chat.jsonl and subagents.jsonl (SPIKE-046). The scrub check fails it on
// e-mail addresses, home folders, API keys, and this machine's user: the
// git user's name and e-mail, the login name and the home folder.
func export(args []string, w io.Writer) error {
	fs := flag.NewFlagSet("export", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprint(fs.Output(), "usage: jenab-scenarios export [flags] <report or dir>...\n\n"+
			"A report is a run's <stamp>.json; a dir is searched for them. Runs need -examples.\n\n")
		fs.PrintDefaults()
	}
	var (
		out    = fs.String("out", "export", "the folder for chat.jsonl and subagents.jsonl")
		splits = fs.String("splits", "train", "the sets' splits to take, comma-separated; a report of another split is an error")
		deny   = fs.String("deny", "", "more regexes the examples must not match, comma-separated, such as a name")
	)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() == 0 {
		fs.Usage()
		return fmt.Errorf("no reports")
	}
	reports, err := reportFiles(fs.Args())
	if err != nil {
		return err
	}
	if len(reports) == 0 {
		return fmt.Errorf("no reports in %s", strings.Join(fs.Args(), ", "))
	}
	scrub, err := scenario.NewScrubber(localIDs(), split(*deny))
	if err != nil {
		return err
	}
	st, err := scenario.Export(reports, scenario.ExportOptions{Out: *out, Splits: split(*splits), Scrub: scrub})
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "%d reports, %d runs, %d passed: %d chat examples, %d subagent examples in %s\n",
		st.Reports, st.Runs, st.Passed, st.Chat, st.Subagent, *out)
	return nil
}

var stampFile = regexp.MustCompile(`^\d{8}-\d{6}\.json$`)

// reportFiles are the report JSON files: the files given, and the
// <stamp>.json files under the folders given.
func reportFiles(args []string) ([]string, error) {
	var out []string
	for _, a := range args {
		st, err := os.Stat(a)
		if err != nil {
			return nil, err
		}
		if !st.IsDir() {
			out = append(out, a)
			continue
		}
		err = filepath.WalkDir(a, func(p string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() && stampFile.MatchString(d.Name()) {
				out = append(out, p)
			}
			return err
		})
		if err != nil {
			return nil, err
		}
	}
	slices.Sort(out)
	return slices.Compact(out), nil
}

// Login names that say nothing about a person, such as a container's.
var genericUsers = []string{"root", "user", "admin", "administrator", "runner", "ubuntu", "colab", "jovyan", "vscode", "codespace", "default"}

// localIDs are this machine's user: names for the scrub check, never
// printed.
func localIDs() map[string]string {
	ids := map[string]string{}
	for _, k := range []string{"user.name", "user.email"} {
		if b, err := exec.Command("git", "config", "--get", k).Output(); err == nil {
			ids["the git "+k] = strings.TrimSpace(string(b))
		}
	}
	if u, err := user.Current(); err == nil {
		login := u.Username
		if i := strings.LastIndexAny(login, `\/`); i >= 0 {
			login = login[i+1:]
		}
		if !slices.Contains(genericUsers, strings.ToLower(login)) {
			ids["the login name"] = login
			ids["the home folder"] = u.HomeDir
		}
		if !slices.Contains(genericUsers, strings.ToLower(u.Name)) {
			ids["the user's full name"] = u.Name
		}
	}
	return ids
}
