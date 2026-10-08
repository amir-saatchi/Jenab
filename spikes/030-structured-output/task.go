package main

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Task is one request with a fixed right answer.
type Task struct {
	ID          string `yaml:"id"`
	Kind        string `yaml:"kind"` // select, extract or decide
	Lang        string `yaml:"lang"`
	Count       int    `yaml:"count"`  // select: items to pick
	SchemaFile  string `yaml:"schema"` // extract: a file in data/schemas
	Instruction string `yaml:"instruction"`
	Input       string `yaml:"input"`
	Expect      any    `yaml:"expect"`

	Schema json.RawMessage `yaml:"-"` // the output schema
	Items  int             `yaml:"-"` // select and decide: items in the input
}

var topics = []string{"billing", "delivery", "product", "account", "other"}

func loadTasks(dir string) ([]*Task, error) {
	files, err := filepath.Glob(filepath.Join(dir, "tasks", "*.yaml"))
	if err != nil {
		return nil, err
	}
	var out []*Task
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		t := &Task{}
		if err := yaml.Unmarshal(b, t); err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		out = append(out, t)
	}
	out = append(out, longTask())
	for _, t := range out {
		if err := t.prepare(dir); err != nil {
			return nil, fmt.Errorf("%s: %w", t.ID, err)
		}
	}
	return out, nil
}

func (t *Task) prepare(dir string) error {
	for _, l := range strings.Split(t.Input, "\n") {
		if strings.HasPrefix(l, "[") {
			t.Items++
		}
	}
	switch t.Kind {
	case "select":
		t.Schema = mustJSON(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"picks": map[string]any{
					"type": "array", "minItems": t.Count, "maxItems": t.Count,
					"description": "The chosen items, in the asked order",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"index":  map[string]any{"type": "integer", "minimum": 0, "maximum": t.Items - 1, "description": "The item's index"},
							"reason": map[string]any{"type": "string"},
						},
						"required":             []string{"index", "reason"},
						"additionalProperties": false,
					},
				},
			},
			"required":             []string{"picks"},
			"additionalProperties": false,
		})
	case "decide":
		t.Schema = mustJSON(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"answers": map[string]any{
					"type": "array", "minItems": t.Items, "maxItems": t.Items,
					"description": "One answer per message",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"index":     map[string]any{"type": "integer", "minimum": 0, "maximum": t.Items - 1, "description": "The message's index"},
							"complaint": map[string]any{"type": "boolean"},
							"topic":     map[string]any{"type": "string", "enum": topics},
							"urgency":   map[string]any{"type": "integer", "minimum": 1, "maximum": 5},
						},
						"required":             []string{"index", "complaint", "topic", "urgency"},
						"additionalProperties": false,
					},
				},
			},
			"required":             []string{"answers"},
			"additionalProperties": false,
		})
	case "extract":
		b, err := os.ReadFile(filepath.Join(dir, "schemas", t.SchemaFile))
		if err != nil {
			return err
		}
		if !json.Valid(b) {
			return fmt.Errorf("schema %s is not valid JSON", t.SchemaFile)
		}
		t.Schema = b
	default:
		return fmt.Errorf("unknown kind %q", t.Kind)
	}
	if t.Items == 0 && t.Kind != "extract" {
		return fmt.Errorf("no items in the input")
	}
	_, err := compileSchema(t.ID, t.Schema)
	return err
}

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

// longTask is select over 700 generated headlines, about 15,000 tokens, near
// llm.select's max_input_tokens (SPEC 6.5). Five are about Ethereum.
func longTask() *Task {
	r := rand.New(rand.NewPCG(30, 30))
	coins := []string{"Bitcoin", "Solana", "Cardano", "XRP", "Dogecoin", "Litecoin", "Polkadot", "Avalanche", "Chainlink", "Tron"}
	moves := []string{"rises 4% as traders return", "falls after a large holder sells", "network upgrade goes live", "sees record daily volume", "faces a new lawsuit", "adds support for a new wallet", "hits a three-month high", "drops below a key level", "foundation names a new head", "fees fall to a yearly low"}
	other := []string{
		"Oil prices steady after OPEC meeting", "Ethiopia's coffee exports grow again", "Ethernet standard gets a faster version",
		"Ethics board reviews hospital AI use", "Diethyl ether prices rise for labs", "Ethan Hawke film wins a festival award",
		"Gold rises as the dollar weakens", "Heavy rain floods streets in the city centre", "Central bank holds interest rates",
		"A startup raises funding for AI chips", "New subway line opens downtown", "Ethiopian Airlines adds new routes",
		"Stock markets close higher on tech gains", "A chip maker reports record sales", "Airline strikes cancel hundreds of flights",
	}
	sources := []string{"CoinDesk", "Reuters", "Bloomberg", "The Block", "Decrypt", "FT", "AP", "BBC", "Wired", "CNBC"}
	eth := map[int][2]string{
		37:  {"2027-01-14", "Ethereum validators approve a new client release"},
		151: {"2026-08-03", "Ether options open interest reaches a record"},
		333: {"2027-03-20", "ETH rallies after the network upgrade date is set"},
		489: {"2026-11-29", "Ethereum layer-2 fees drop by half"},
		652: {"2026-06-17", "Spot Ethereum ETFs add assets for a fifth week"},
	}
	var b strings.Builder
	for i := range 700 {
		date, head := "", ""
		if e, ok := eth[i]; ok {
			date, head = e[0], e[1]
		} else {
			date = dateAfter("2026-06-01", r.IntN(300))
			if r.IntN(3) == 0 {
				head = other[r.IntN(len(other))]
			} else {
				head = coins[r.IntN(len(coins))] + " " + moves[r.IntN(len(moves))]
			}
		}
		fmt.Fprintf(&b, "[%d] %s · %s · %s\n", i, date, sources[r.IntN(len(sources))], head)
	}
	order := []int{333, 37, 489, 151, 652}
	exp := make([]any, len(order))
	for i, v := range order {
		exp[i] = v
	}
	return &Task{ID: "long-en", Kind: "select", Lang: "en", Count: 5,
		Instruction: "Pick the 5 items about Ethereum (ETH), newest first. For each, give its index and a short reason.",
		Input:       b.String(), Expect: exp}
}

// dateAfter is the day n days after start, both YYYY-MM-DD.
func dateAfter(start string, n int) string {
	var y, m, d int
	fmt.Sscanf(start, "%d-%d-%d", &y, &m, &d)
	days := []int{31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31}
	for range n {
		d++
		if d > days[m-1] {
			d, m = 1, m+1
			if m > 12 {
				m, y = 1, y+1
			}
		}
	}
	return fmt.Sprintf("%04d-%02d-%02d", y, m, d)
}

func taskIDs(ts []*Task) []string {
	var out []string
	for _, t := range ts {
		out = append(out, t.ID)
	}
	slices.Sort(out)
	return out
}
