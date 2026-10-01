package main

// Skills (SPEC 8.9 draft): Markdown files with front matter (name, description, load_with) and
// a body of at most 3,000 tokens. The skill list (names and descriptions) goes at the end of
// block 1 of the system prompt; load_skill(name) returns a body; in condition C the first call
// of a load_with tool also returns the skill with the tool's result.

import (
	"embed"
	"fmt"
	"regexp"
	"strings"

	"go.yaml.in/yaml/v3"
)

//go:embed skills/*.md
var skillFS embed.FS

//go:embed prompts/*.md
var promptFS embed.FS

type Skill struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	LoadWith    []string `yaml:"load_with"`
	Body        string   `yaml:"-"`
}

// the order of the skill list (fixed, the same for every model)
var skillOrder = []string{"config-guide", "pipelines", "migrations", "database-design", "sql-queries", "web-research", "delegation"}

var (
	skills   = map[string]*Skill{}
	loadWith = map[string][]string{} // tool -> skills
	prompts  = map[string]string{}
	intro    string // guide v2 up to section 1 (the part that stays in every condition)
)

var skillName = regexp.MustCompile(`^[a-z0-9-]+$`)

const (
	skillMaxTokens  = 3000
	chatSkillMax    = 6
	chatSkillTokens = 10000
)

// estTokens is a rough estimate (chars/3.5); the calibration measures the real counts.
func estTokens(s string) int { return len(s) * 2 / 7 }

func parseSkill(src string) (*Skill, error) {
	src = strings.ReplaceAll(src, "\r\n", "\n")
	if !strings.HasPrefix(src, "---\n") {
		return nil, fmt.Errorf("no front matter")
	}
	fm, body, ok := strings.Cut(src[4:], "\n---\n")
	if !ok {
		return nil, fmt.Errorf("front matter not closed")
	}
	var s Skill
	if err := yaml.Unmarshal([]byte(fm), &s); err != nil {
		return nil, err
	}
	s.Body = strings.TrimRight(body, "\n")
	switch {
	case !skillName.MatchString(s.Name):
		return nil, fmt.Errorf("bad name %q", s.Name)
	case s.Description == "" || len([]rune(s.Description)) > 200:
		return nil, fmt.Errorf("%s: description must be 1-200 characters (has %d)", s.Name, len([]rune(s.Description)))
	case s.Body == "":
		return nil, fmt.Errorf("%s: empty body", s.Name)
	case estTokens(s.Body) > skillMaxTokens:
		return nil, fmt.Errorf("%s: body about %d tokens, over %d", s.Name, estTokens(s.Body), skillMaxTokens)
	}
	return &s, nil
}

func loadSkills() {
	ents, _ := skillFS.ReadDir("skills")
	for _, e := range ents {
		b, _ := skillFS.ReadFile("skills/" + e.Name())
		s, err := parseSkill(string(b))
		if err != nil {
			panic("skill " + e.Name() + ": " + err.Error())
		}
		if s.Name+".md" != e.Name() {
			panic("skill file name does not match its name: " + e.Name())
		}
		if skills[s.Name] != nil {
			panic("duplicate skill " + s.Name)
		}
		skills[s.Name] = s
		for _, t := range s.LoadWith {
			loadWith[t] = append(loadWith[t], s.Name)
		}
	}
	for _, n := range skillOrder {
		if skills[n] == nil {
			panic("missing skill " + n)
		}
	}
	if len(skills) != len(skillOrder) {
		panic("skill list and skill files differ")
	}
	pe, _ := promptFS.ReadDir("prompts")
	for _, e := range pe {
		b, _ := promptFS.ReadFile("prompts/" + e.Name())
		prompts[strings.TrimSuffix(e.Name(), ".md")] = strings.TrimSpace(strings.ReplaceAll(string(b), "\r\n", "\n"))
	}
}

// skillListHeader is the fixed text before the list (same for every model and condition B/C).
const skillListHeader = "## Skills\nSkills hold the formats, rules and examples for one kind of work. Before you do that kind of work, load its skill with load_skill(name); the text arrives as the tool result and stays in this chat. Load only what the task needs."

// skillList renders the list; skills in `loaded` are marked.
func skillList(loaded map[string]bool) string {
	var b strings.Builder
	b.WriteString(skillListHeader + "\n")
	for _, n := range skillOrder {
		mark := ""
		if loaded[n] {
			mark = " (loaded)"
		}
		fmt.Fprintf(&b, "- %s%s: %s\n", n, mark, skills[n].Description)
	}
	return b.String()
}

// skillText is what load_skill and load_with return for one skill.
func skillText(name string) string {
	return fmt.Sprintf("Skill loaded: %s\n\n%s\n", name, skills[name].Body)
}

// rebuildGuide puts guide v2 back together from the intro and the five split skills; the test
// checks that this gives the guide byte for byte (condition A has exactly the B/C guide text).
func rebuildGuide() string {
	mig := skills["migrations"].Body
	design := skills["database-design"].Body
	i := strings.Index(mig, "\nSteps run in order")
	if i < 0 {
		return ""
	}
	mig = mig[:i+1] + design + " " + mig[i+1:]
	return intro + "\n" + mig + "\n\n" + skills["pipelines"].Body + "\n\n" + skills["config-guide"].Body + "\n\n" + skills["sql-queries"].Body + "\n"
}
