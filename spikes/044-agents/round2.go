package main

import (
	"fmt"
	"strings"
)

// round is the round the process runs: 1 is the ticket's design under
// test, 2 the proposed SPEC 8.6 text (the -round flag).
var round = 1

// roleDesc is create_chat's role description, and delegateLine the line
// added to Mother's delegation guidance ("" in round 1).
var (
	roleDesc = "The chat's custom prompt, at most 500 tokens, added after its system prompt. " +
		"Say what the chat is for, how it works and what it reports. The system prompt already has the general rules."
	delegateLine string
)

// useRound2 puts the proposed SPEC 8.6 text in place: Read gains
// read_config, run_status and open_page; Views and pipelines becomes Build
// and run; the role description and a delegation line change. The
// scenarios' groups are renamed to match. The refusal keeps its wording,
// so it names the new groups.
func useRound2() {
	round = 2
	groups = []group{
		{"read", "Read", "read the project's tables, views, pages, pipelines and runs",
			[]string{"query", "describe_table", "read_config", "run_status", "open_page"}},
		{"change_data", "Change data", "add rows and ask the schema agent for tables or columns", []string{"insert", "request_schema_change"}},
		{"build_run", "Build and run", "build, check and run views, pages and pipelines",
			[]string{"save_view", "save_page", "save_pipeline", "validate", "dry_run", "run_pipeline"}},
		groups[3], groups[4], groups[5], // files, web, memory
	}
	motherGroups = []string{"read", "files", "memory"}
	// As round 2 ran it; the proposal drops "; don't repeat them", which
	// changed nothing.
	roleDesc = "The chat's custom prompt, at most 500 tokens, in the user's language. " +
		"Say what the chat is for, how it works and what it reports back. The system prompt already has the general rules; don't repeat them."
	delegateLine = "You can't change data, build or run, or use the web. Work that needs them goes to a chat that has them: " +
		"send it to one, or create one with only the groups the job needs."
	for i := range scenarios {
		sc := &scenarios[i]
		sc.Need, sc.Forbid, sc.KeyGroups = rename(sc.Need), rename(sc.Forbid), rename(sc.KeyGroups)
		if sc.ID == "reviewer" {
			// Reading a pipeline's config is now in Read; dry_run and
			// validate are in Build and run, which the reviewer may use
			// but doesn't need.
			sc.Need = []string{"read"}
		}
	}
}

// rename maps round 1's group names to round 2's.
func rename(gs []string) []string {
	out := make([]string, 0, len(gs))
	for _, g := range gs {
		switch g {
		case "read_data":
			out = append(out, "read")
		case "views_pipelines":
			out = append(out, "build_run")
		default:
			out = append(out, g)
		}
	}
	return out
}

func setRound(n int) error {
	switch n {
	case 1:
		return nil
	case 2:
		useRound2()
		return nil
	}
	return fmt.Errorf("no round %d", n)
}

// checkRound fails when a run was made in another round than this
// process's: its groups would be scored against the wrong names.
func checkRound(runs []runResult) error {
	for _, r := range runs {
		if max(r.Round, 1) != round {
			return fmt.Errorf("%s ran in round %d; pass -round %d", strings.TrimSpace(r.name()), max(r.Round, 1), max(r.Round, 1))
		}
	}
	return nil
}
