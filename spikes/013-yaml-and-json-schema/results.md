# SPIKE-013 results

Go go1.26.0, windows/amd64, 2026-09-28.

Libraries: go.yaml.in/yaml/v3 v3.0.5, go.yaml.in/yaml/v4 v4.0.0-rc.6, github.com/goccy/go-yaml v1.19.2;
github.com/santhosh-tekuri/jsonschema/v6 v6.0.3, github.com/kaptinlin/jsonschema v0.7.7, github.com/google/jsonschema-go v0.4.3.

"Burrow loader" = parse to the library's syntax tree, then one Go walk that builds the value, records a line:column per JSON pointer,
and rejects anchors, aliases, merge keys, tags, non-string keys, duplicate keys and nesting over 64 levels (load.go).


## 1. Valid sample pipeline

`fixtures/valid.yaml` (SPEC 9.2 plus inputs, retry, a block-scalar SQL and app.notify) and `fixtures/valid.json`, the same config written by hand as JSON.

| loader | file | loads | santhosh v6 | kaptinlin v0.7.7 | google v0.4.3 | same value as encoding/json of valid.json |
|---|---|---|---|---|---|---|
| yaml/v3 | valid.yaml | yes | valid | valid | valid | yes |
| yaml/v3 | valid.json | yes | valid | valid | valid | yes |
| yaml/v4 | valid.yaml | yes | valid | valid | valid | yes |
| yaml/v4 | valid.json | yes | valid | valid | valid | yes |
| goccy | valid.yaml | yes | valid | valid | valid | yes |
| goccy | valid.json | yes | valid | valid | valid | yes |

## 2. Broken files: all errors in one pass, line mapping, messages

Each file in `fixtures/broken/` lists its expected errors as `# expect: LINE POINTER KEYWORD` (JSON files use a `.expect` side file).
"found" = expected errors reported in one Validate call; "extra" = other errors reported. Lines are taken from the santhosh
errors mapped through each loader's pointer map (key position for unknown/missing fields and whole objects, value position otherwise).

| file | expected | santhosh found / extra | kaptinlin found / extra | google errors returned | lines right v3 / v4 / goccy | v3, v4, goccy same line:col |
|---|---|---|---|---|---|---|
| b01_steps_not_list.yaml | 1 | 1 / 0 | 1 / 0 | 1 | 1/1 / 1/1 / 1/1 | yes |
| b02_version_string.yaml | 1 | 1 / 0 | 1 / 0 | 1 | 1/1 / 1/1 / 1/1 | yes |
| b03_missing_id.yaml | 1 | 1 / 0 | 1 / 1 | 1 | 1/1 / 1/1 / 1/1 | yes |
| b04_missing_trigger_and_steps.yaml | 1 | 1 / 0 | 1 / 2 | 1 | 1/1 / 1/1 / 1/1 | yes |
| b05_unknown_top_field.yaml | 1 | 1 / 0 | 1 / 1 | 1 | 1/1 / 1/1 / 1/1 | yes |
| b06_unknown_step_field.yaml | 1 | 1 / 0 | 1 / 1 | 1 | 1/1 / 1/1 / 1/1 | yes |
| b07_typo_in_with.yaml | 2 | 2 / 0 | 2 / 2 | 1 | 2/2 / 2/2 / 2/2 | yes |
| b08_bad_enum_on_conflict.yaml | 1 | 1 / 0 | 1 / 0 | 1 | 1/1 / 1/1 / 1/1 | yes |
| b09_unknown_step_name.yaml | 1 | 1 / 0 | 1 / 0 | 1 | 1/1 / 1/1 / 1/1 | yes |
| b10_bad_cron.yaml | 1 | 1 / 0 | 1 / 2 | 0 | 1/1 / 1/1 / 1/1 | yes |
| b11_bad_timezone.yaml | 1 | 1 / 0 | 1 / 2 | 0 | 1/1 / 1/1 / 1/1 | yes |
| b12_bad_durations.yaml | 3 | 3 / 0 | 3 / 0 | 1 | 3/3 / 3/3 / 3/3 | yes |
| b13_bad_ids.yaml | 2 | 2 / 0 | 2 / 0 | 1 | 2/2 / 2/2 / 2/2 | yes |
| b14_nested_step_errors.yaml | 2 | 2 / 0 | 2 / 0 | 1 | 2/2 / 2/2 / 2/2 | yes |
| b15_inputs.yaml | 3 | 2 / 1 | 3 / 3 | 1 | 2/3 / 2/3 / 2/3 | yes |
| b16_trigger_without_schedule_or_manual.yaml | 1 | 1 / 0 | 1 / 0 | 1 | 1/1 / 1/1 / 1/1 | yes |
| b17_empty_steps.yaml | 1 | 1 / 0 | 1 / 0 | 1 | 1/1 / 1/1 / 1/1 | yes |
| b18_many_errors.yaml | 11 | 11 / 0 | 11 / 6 | 1 | 11/11 / 11/11 / 11/11 | yes |
| b19_nulls.yaml | 2 | 2 / 0 | 2 / 0 | 1 | 2/2 / 2/2 / 2/2 | no: steps[0].with: v3 9:10, goccy 9:11 |
| b20_yaml11_words.yaml | 4 | 4 / 0 | 3 / 0 | 1 | 4/4 / 4/4 / 4/4 | yes |
| b21_duplicate_key.yaml | parse error at line 8 | - | - | - | yes (line 8) / yes (line 8) / yes (line 8) | - |
| b22_bad_indent.yaml | parse error at line 8 | - | - | - | yes (line 8) / yes (line 8) / **no** (line 7) | - |
| b23_many_errors.json | 6 | 6 / 0 | 5 / 2 | 1 | 6/6 / 6/6 / 6/6 | yes |
| b24_duplicate_key.json | parse error at line 8 | - | - | - | yes (line 8) / yes (line 8) / yes (line 8) | - |

Totals for santhosh v6: 46 expected errors found, 1 extra.

Totals for kaptinlin v0.7.7: 45 expected errors found, 22 extra.

google v0.4.3 on b18_many_errors.yaml, 50 runs: 3 different first errors returned.

b15: santhosh v6.0.3 puts the `propertyNames` error at the wrong path. Its validator stores its internal location slice
without copying it (`verr.InstanceLocation = vd.vloc` in validator.go), so a later property overwrites it. Other keywords copy it.

### Messages Burrow would return (santhosh v6 errors, yaml/v3 positions, SPEC path)

```
b01_steps_not_list.yaml
  line 5:8 steps: got string, want array
b02_version_string.yaml
  line 1:10 version: value must be 1
b03_missing_id.yaml
  line 1:1 (top level): missing property 'id'
b04_missing_trigger_and_steps.yaml
  line 1:1 (top level): missing properties 'trigger', 'steps'
b05_unknown_top_field.yaml
  line 3:1 schedule: unknown field "schedule"
b06_unknown_step_field.yaml
  line 12:5 steps[1].depends_on: unknown field "depends_on"
b07_typo_in_with.yaml
  line 8:5 steps[0].with: missing property 'url'
  line 9:7 steps[0].with.urls: unknown field "urls"
b08_bad_enum_on_conflict.yaml
  line 10:20 steps[0].with.on_conflict: value must be one of 'error', 'ignore', 'upsert'
b09_unknown_step_name.yaml
  line 7:10 steps[0].use: value must be one of 'http.get', 'http.download', 'web.search', 'html.extract', 'json.extract', 'transform.map', 'transform.filter', 'db.query', 'db.insert', 'db.insert_many', 'db.update', 'llm.select', 'llm.extract', 'bucket.put', 'bucket.list', 'app.notify', 'script.starlark'
b10_bad_cron.yaml
  line 4:13 trigger.schedule: '0 8 * *' is not valid cron: needs 5 fields (minute hour day month weekday), got 4
b11_bad_timezone.yaml
  line 5:13 trigger.timezone: 'Europe/Berln' is not valid iana-tz: unknown IANA time zone
b12_bad_durations.yaml
  line 9:31 steps[0].retry.backoff: got number, want string
  line 12:16 steps[0].with.timeout: '2d' is not valid go-duration: not a Go duration like 500ms, 60s, 5m, 2h
  line 3:10 timeout: '5 minutes' is not valid go-duration: not a Go duration like 500ms, 60s, 5m, 2h
b13_bad_ids.yaml
  line 2:5 id: 'Daily-BTC' does not match pattern '^[a-z][a-z0-9_]*$'
  line 6:9 steps[0].id: '1price' does not match pattern '^[a-z][a-z0-9_]*$'
b14_nested_step_errors.yaml
  line 15:18 steps[1].with.freshness: value must be one of 'day', 'week', 'month', 'any'
  line 14:14 steps[1].with.limit: must match one of: got string, want integer | 'twenty' does not match pattern '^\\$\\{\\{.*\\}\\}$'
b15_inputs.yaml
  line 4:17 inputs.coin.type: value must be one of 'string', 'number', 'boolean', 'date'
  line 6:3 inputs.days: missing property 'type'
  line 9:1 steps.Max-Days: invalid name "Max-Days": 'Max-Days' does not match pattern '^[a-z][a-z0-9_]*$'
b16_trigger_without_schedule_or_manual.yaml
  line 3:1 trigger: must match one of: missing property 'schedule' | missing property 'manual'
b17_empty_steps.yaml
  line 5:1 steps: minItems: got 0, want 1
b18_many_errors.yaml
  line 22:1 on_eror: unknown field "on_eror"
  line 12:23 steps[0].with.fail_on_status: got string, want boolean
  line 11:12 steps[0].with.url: 'ftp://api.example.com/btc' is not valid http-url: only http and https URLs are allowed
  line 15:5 steps[1].with: missing property 'row'
  line 17:20 steps[1].with.on_conflict: value must be one of 'error', 'ignore', 'upsert'
  line 18:5 steps[2]: missing property 'id'
  line 21:7 steps[2].with.limit: unknown field "limit"
  line 6:13 trigger.catch_up: value must be one of 'once', 'skip'
  line 4:13 trigger.schedule: 'every morning' is not valid cron: needs 5 fields (minute hour day month weekday), got 2
  line 5:13 trigger.timezone: 'Mars/Olympus' is not valid iana-tz: unknown IANA time zone
  line 1:10 version: value must be 1
b19_nulls.yaml
  line 3:14 description: got null, want string
  line 9:10 steps[0].with: got null, want object
b20_yaml11_words.yaml
  line 3:11 on_error: value must be one of 'stop', 'continue'
  line 11:23 steps[0].with.fail_on_status: got string, want boolean
  line 4:1 trigger: must match one of: missing property 'schedule' | manual: value must be true
  line 5:11 trigger.manual: got string, want boolean
b21_duplicate_key.yaml
  yaml/v3: line 8:5: duplicate key "id"
  yaml/v4: line 8:5: duplicate key "id"
  goccy: line 8:5: mapping key "id" already defined at [6:5]
b22_bad_indent.yaml
  yaml/v3: line 8:0: yaml: line 8: mapping values are not allowed in this context
  yaml/v4: line 8:10: go-yaml load error in scanner at L8.C10: mapping values are not allowed in this context
  goccy: line 7:10: mapping value is not allowed in this context
b23_many_errors.json
  line 9:15 on_error: value must be one of 'stop', 'continue'
  line 6:100 steps[0].with.timeout: got number, want string
  line 7:37 steps[1].with: missing property 'sql'
  line 7:47 steps[1].with.query: unknown field "query"
  line 4:3 trigger: must match one of: missing property 'schedule' | manual: value must be true
  line 4:26 trigger.manual: got string, want boolean
b24_duplicate_key.json
  yaml/v3: line 8:3: duplicate key "id"
  yaml/v4: line 8:3: duplicate key "id"
  goccy: line 8:3: mapping key "id" already defined at [3:3]
```

### Raw messages from the other validators (selected files)

```
b07_typo_in_with.yaml, kaptinlin v0.7.7
  /steps/0/with additionalProperties: Additional property 'urls' does not match the schema
  /steps/0/with required: Required property 'url' is missing
  /steps/0/with/url type: Value is null but should be string
  /steps/0/with/urls schema: No values are allowed because the schema is set to 'false'
b07_typo_in_with.yaml, google v0.4.3
  ? ?: validating https://burrow.local/schema/pipeline.json: validating /properties/steps: validating /properties/steps/items: validating /$defs/step: validating /$defs/step/allOf/0: validating /$defs/step/allOf/0/then: validating /$defs/step/allOf/0/then/properties/with: validating /$defs/with_http_get: unexpected additional properties ["urls"]
b14_nested_step_errors.yaml, kaptinlin v0.7.7
  /steps/1/with/freshness enum: Value daily should be one of the allowed values: day, week, month, any
  /steps/1/with/limit anyOf: Value does not match anyOf schema
b14_nested_step_errors.yaml, google v0.4.3
  ? ?: validating https://burrow.local/schema/pipeline.json: validating /properties/steps: validating /properties/steps/items: validating /$defs/step: validating /$defs/step/allOf/1: validating /$defs/step/allOf/1/then: validating /$defs/step/allOf/1/then/properties/with: validating /$defs/with_web_search: validating /$defs/with_web_search/properties/limit: validating /$defs/int_or_expr: anyOf: did not validate against any of [<anonymous schema> <anonymous schema>]:
  validating /$defs/int_or_expr/anyOf/0: type: twenty has type "string", want "integer"
  validating /$defs/int_or_expr/anyOf/1: validating /$defs/expr: pattern: "twenty" does not match regular expression "^\\$\\{\\{.*\\}\\}$"
b18_many_errors.yaml, kaptinlin v0.7.7
   additionalProperties: Additional property 'on_eror' does not match the schema
  /on_eror schema: No values are allowed because the schema is set to 'false'
  /steps/0/with/fail_on_status type: Value is string but should be boolean
  /steps/0/with/url format: Value does not match format 'http-url'
  /steps/1/with required: Required property 'row' is missing
  /steps/1/with/on_conflict enum: Value replace should be one of the allowed values: error, ignore, upsert
  /steps/1/with/row anyOf: Value does not match anyOf schema
  /steps/2 required: Required property 'id' is missing
  /steps/2/id type: Value is null but should be string
  /steps/2/with additionalProperties: Additional property 'limit' does not match the schema
  /steps/2/with/limit schema: No values are allowed because the schema is set to 'false'
  /trigger required: Required property 'manual' is missing
  /trigger/catch_up enum: Value sometimes should be one of the allowed values: once, skip
  /trigger/manual const: Value does not match the constant value
  /trigger/schedule format: Value does not match format 'cron'
  /trigger/timezone format: Value does not match format 'iana-tz'
  /version const: Value does not match the constant value
b18_many_errors.yaml, google v0.4.3
  ? ?: validating https://burrow.local/schema/pipeline.json: validating /properties/trigger: validating /properties/trigger/properties/catch_up: enum: sometimes does not equal any of: [once skip]
```

## 3. YAML 1.1 surprises

Each input is decoded as `v: <input>` (key rows: the whole document). Native = the library's own `Unmarshal`/`Load` into `any`.
Burrow loader = load.go (the value the validator sees). "json.Marshal" = can the native value be stored as JSON as-is.

| input | yaml/v3 native | yaml/v4 native | goccy native | Burrow loader (v3 / v4 / goccy) |
|---|---|---|---|---|
| `no` | `string "no"` | `string "no"` | `string "no"` | `string "no"` (all three) |
| `on` | `string "on"` | `string "on"` | `string "on"` | `string "on"` (all three) |
| `yes` | `string "yes"` | `string "yes"` | `string "yes"` | `string "yes"` (all three) |
| `off` | `string "off"` | `string "off"` | `string "off"` | `string "off"` (all three) |
| `y` | `string "y"` | `string "y"` | `string "y"` | `string "y"` (all three) |
| `True` | `bool true` | `bool true` | `bool true` | `bool true` (all three) |
| `0755` | `int 493` | `int 493` | `uint64 493` | `int64 493` (all three) |
| `010` | `int 8` | `int 8` | `uint64 8` | `int64 8` (all three) |
| `0o755` | `int 493` | `int 493` | `uint64 493` | `int64 493` (all three) |
| `0x1F` | `int 31` | `int 31` | `uint64 31` | `int64 31` (all three) |
| `1_000` | `int 1000` | `int 1000` | `uint64 1000` | `int64 1000` (all three) |
| `+1` | `int 1` | `int 1` | `uint64 1` | `int64 1` (all three) |
| `1e3` | `float64 1000` | `float64 1000` | `string "1e3"` | `float64 1000 / float64 1000 / string "1e3"` |
| `.inf` | `float64 +Inf (json.Marshal fails)` | `float64 +Inf (json.Marshal fails)` | `float64 +Inf (json.Marshal fails)` | `error: line 1:4: NaN and infinity are not allowed` (all three) |
| `2026-09-28` | `time.Time 2026-09-28T00:00:00Z` | `time.Time 2026-09-28T00:00:00Z` | `string "2026-09-28"` | `string "2026-09-28"` (all three) |
| `2026-09-28T08:00:00Z` | `time.Time 2026-09-28T08:00:00Z` | `time.Time 2026-09-28T08:00:00Z` | `string "2026-09-28T08:00:00Z"` | `string "2026-09-28T08:00:00Z"` (all three) |
| `~` | `null` | `null` | `null` | `null` (all three) |
| `12:30` | `string "12:30"` | `string "12:30"` | `string "12:30"` | `string "12:30"` (all three) |
| `"0755"` | `string "0755"` | `string "0755"` | `string "0755"` | `string "0755"` (all three) |
| `on: 1` | `map[string]any{"on": int 1}` | `map[string]any{"on": int 1}` | `map[string]any{"on": uint64 1}` | `map[string]any{"on": int64 1}` (all three) |
| `no: 1` | `map[string]any{"no": int 1}` | `map[string]any{"no": int 1}` | `map[string]any{"no": uint64 1}` | `map[string]any{"no": int64 1}` (all three) |
| `1: x` | `map[any]any{int 1: string "x"} (json.Marshal fails)` | `map[any]any{int 1: string "x"} (json.Marshal fails)` | `map[string]any{"1": string "x"}` | `error: line 1:1: key "1" must be a string (quote it)` (all three) |
| `2026-09-28: x` | `map[any]any{time.Time 2026-09-28 00:00:00 +0000 UTC: string "x"} (json.Marshal fails)` | `map[any]any{time.Time 2026-09-28 00:00:00 +0000 UTC: string "x"} (json.Marshal fails)` | `map[string]any{"2026-09-28": string "x"}` | `error: line 1:1: key "2026-09-28" must be a string (quote it) / error: line 1:1: key "2026-09-28" must be a string (quote it) / map[string]any{"2026-09-28": ...` |
| `~: x` | `map[any]any{<nil> <nil>: string "x"} (json.Marshal fails)` | `map[any]any{<nil> <nil>: string "x"} (json.Marshal fails)` | `map[string]any{"null": string "x"}` | `error: line 1:1: key "~" must be a string (quote it) / error: line 1:1: key "~" must be a string (quote it) / error: line 1:1: key "null" must be a string (q...` |

### JSON read by the YAML loaders

Configs may arrive as JSON. Same value as `encoding/json` (after both go through `encoding/json` once)?

| JSON | encoding/json | yaml/v3 | yaml/v4 | goccy |
|---|---|---|---|---|
| `{"v": 1e3}` | `float64 1000` | same | same | **differs**: `string "1e3"` |
| `{"v": 1E+2}` | `float64 100` | same | same | **differs**: `string "1E+2"` |
| `{"v": 1.0}` | `float64 1` | same | same | same |
| `{"v": -0.5}` | `float64 -0.5` | same | same | same |
| `{"v": 12345678901234567890}` | `float64 1.2345678901234567e+19` | error: `line 1:7: integer too large` | error: `line 1:7: integer too large` | error: `line 1:7: integer too large` |
| `{"v": "é😀"}` | `string "é😀"` | same | same | same |
| `{"v": "a\tb\/c"}` | `string "a\tb/c"` | error: `line 0:0: yaml: found unknown escape character` | error: `line 1:7: go-yaml load error in scanner (while scanning a quoted scalar) at L1.C7-C12: found unknown escape character` | same |
| `{"v": null}` | `null` | same | same | same |
| `{"v":"x","w":[1,{"a":true}]}` | `string "x"` | same | same | same |

## 4. Duplicate keys, alias bombs, deep nesting, merge keys

### Duplicate keys

| input | library | tree only | native decode | Burrow loader |
|---|---|---|---|---|
| `id: a↵id: b↵` | yaml/v3 | **accepted** | rejected: `yaml: unmarshal errors:` | rejected: `line 2:1: duplicate key "id"` |
| `id: a↵id: b↵` | yaml/v4 | **accepted** | rejected: `yaml: construct errors: line 2: mapping key "id" already defined at line 1` | rejected: `line 2:1: duplicate key "id"` |
| `id: a↵id: b↵` | goccy | rejected: `[2:1] mapping key "id" already defined at [1:1]` | rejected: `[2:1] mapping key "id" already defined at [1:1]` | rejected: `line 2:1: mapping key "id" already defined at [1:1]` |
| `steps:↵  - id: a↵    use: x↵    use: y↵` | yaml/v3 | **accepted** | rejected: `yaml: unmarshal errors:` | rejected: `line 4:5: duplicate key "use"` |
| `steps:↵  - id: a↵    use: x↵    use: y↵` | yaml/v4 | **accepted** | rejected: `yaml: construct errors: line 4: mapping key "use" already defined at line 3` | rejected: `line 4:5: duplicate key "use"` |
| `steps:↵  - id: a↵    use: x↵    use: y↵` | goccy | rejected: `[4:5] mapping key "use" already defined at [3:5]` | rejected: `[4:5] mapping key "use" already defined at [3:5]` | rejected: `line 4:5: mapping key "use" already defined at [3:5]` |
| `{"id": "a", "id": "b"}` | yaml/v3 | **accepted** | rejected: `yaml: unmarshal errors:` | rejected: `line 1:13: duplicate key "id"` |
| `{"id": "a", "id": "b"}` | yaml/v4 | **accepted** | rejected: `yaml: construct errors: line 1: mapping key "id" already defined at line 1` | rejected: `line 1:13: duplicate key "id"` |
| `{"id": "a", "id": "b"}` | goccy | rejected: `[1:13] mapping key "id" already defined at [1:2]` | rejected: `[1:13] mapping key "id" already defined at [1:2]` | rejected: `line 1:13: mapping key "id" already defined at [1:2]` |
| `{"id": "a", "id": "b"}` | encoding/json | - | **accepted** → `map[string]any{"id": string "b"}` | - |
| `{"id": "a", "id": "b"}` | santhosh UnmarshalJSON | - | **accepted** → `map[string]any{"id": string "b"}` | - |

### Alias bomb (billion laughs) and deep nesting

Each cell is a separate child process (10 s timeout, killed above 1 GB of Go memory). MB = peak Go memory of the child.
tree = parse to the syntax tree only; native = library decode into `any`; Burrow = load.go (rejects aliases, depth over 64).

| input | library | tree | native | Burrow loader |
|---|---|---|---|---|
| bomb, 3 levels (10^4 strings) (220 bytes) | yaml/v3 | accepted (0 ms, 6 MB) | stopped: `yaml: document contains excessive aliasing` (0 ms, 10 MB) | stopped: `line 1:5: anchors (&a0) are not allowed; write the value out` (0 ms, 10 MB) |
| bomb, 3 levels (10^4 strings) (220 bytes) | yaml/v4 | accepted (0 ms, 10 MB) | stopped: `go-yaml load error in constructor at L1.C22: document contains excessive aliasing` (1 ms, 10 MB) | stopped: `line 1:5: anchors (&a0) are not allowed; write the value out` (0 ms, 10 MB) |
| bomb, 3 levels (10^4 strings) (220 bytes) | goccy | accepted (0 ms, 10 MB) | value built, 69149 bytes as JSON (1 ms, 10 MB) | stopped: `line 1:5: anchors (&a0) are not allowed; write the value out` (0 ms, 10 MB) |
| bomb, 5 levels (10^6 strings) (320 bytes) | yaml/v3 | accepted (0 ms, 10 MB) | stopped: `yaml: document contains excessive aliasing` (1 ms, 10 MB) | stopped: `line 1:5: anchors (&a0) are not allowed; write the value out` (0 ms, 10 MB) |
| bomb, 5 levels (10^6 strings) (320 bytes) | yaml/v4 | accepted (0 ms, 6 MB) | stopped: `go-yaml load error in constructor at L1.C22: document contains excessive aliasing` (1 ms, 10 MB) | stopped: `line 1:5: anchors (&a0) are not allowed; write the value out` (0 ms, 10 MB) |
| bomb, 5 levels (10^6 strings) (320 bytes) | goccy | accepted (0 ms, 10 MB) | value built, 6913603 bytes as JSON (67 ms, 36 MB) | stopped: `line 1:5: anchors (&a0) are not allowed; write the value out` (0 ms, 10 MB) |
| bomb, 8 levels (10^9 strings) (470 bytes) | yaml/v3 | accepted (0 ms, 10 MB) | stopped: `yaml: document contains excessive aliasing` (1 ms, 10 MB) | stopped: `line 1:5: anchors (&a0) are not allowed; write the value out` (0 ms, 10 MB) |
| bomb, 8 levels (10^9 strings) (470 bytes) | yaml/v4 | accepted (0 ms, 10 MB) | stopped: `go-yaml load error in constructor at L1.C22: document contains excessive aliasing` (1 ms, 10 MB) | stopped: `line 1:5: anchors (&a0) are not allowed; write the value out` (0 ms, 10 MB) |
| bomb, 8 levels (10^9 strings) (470 bytes) | goccy | accepted (0 ms, 10 MB) | **killed at 2326 ms, over 1 GB** | stopped: `line 1:5: anchors (&a0) are not allowed; write the value out` (0 ms, 10 MB) |
| `[` nested 1,000 deep (2004 bytes) | yaml/v3 | accepted (2 ms, 11 MB) | value built, 2006 bytes as JSON (4 ms, 15 MB) | stopped: `line 1:68: nesting deeper than 64 levels` (2 ms, 11 MB) |
| `[` nested 1,000 deep (2004 bytes) | yaml/v4 | accepted (2 ms, 11 MB) | value built, 2006 bytes as JSON (6 ms, 15 MB) | stopped: `line 1:68: nesting deeper than 64 levels` (3 ms, 11 MB) |
| `[` nested 1,000 deep (2004 bytes) | goccy | accepted (4 ms, 11 MB) | value built, 2006 bytes as JSON (4 ms, 15 MB) | stopped: `line 1:68: nesting deeper than 64 levels` (5 ms, 15 MB) |
| `[` nested 10,000 deep (20004 bytes) | yaml/v3 | accepted (12 ms, 19 MB) | value built, 20006 bytes as JSON (27 ms, 28 MB) | stopped: `line 1:68: nesting deeper than 64 levels` (14 ms, 16 MB) |
| `[` nested 10,000 deep (20004 bytes) | yaml/v4 | accepted (14 ms, 24 MB) | value built, 20006 bytes as JSON (32 ms, 36 MB) | stopped: `line 1:68: nesting deeper than 64 levels` (13 ms, 24 MB) |
| `[` nested 10,000 deep (20004 bytes) | goccy | accepted (112 ms, 178 MB) | stopped: `exceeded max depth` (106 ms, 183 MB) | stopped: `line 1:68: nesting deeper than 64 levels` (118 ms, 178 MB) |
| `[` nested 1,000,000 deep (2000004 bytes) | yaml/v3 | stopped: `yaml: exceeded max depth of 10000` (8 ms, 20 MB) | stopped: `yaml: exceeded max depth of 10000` (9 ms, 19 MB) | stopped: `line 0:0: yaml: exceeded max depth of 10000` (6 ms, 19 MB) |
| `[` nested 1,000,000 deep (2000004 bytes) | yaml/v4 | stopped: `go-yaml load error in scanner (while increasing flow level) at L1.C10004: exceeded max depth of 10000` (4 ms, 20 MB) | stopped: `go-yaml load error in scanner (while increasing flow level) at L1.C10004: exceeded max depth of 10000` (3 ms, 20 MB) | stopped: `line 1:10004: go-yaml load error in scanner (while increasing flow level) at L1.C10004: exceeded max depth of 10000` (4 ms, 19 MB) |
| `[` nested 1,000,000 deep (2000004 bytes) | goccy | **killed at 1123 ms, over 1 GB** | **killed at 1048 ms, over 1 GB** | **killed at 1079 ms, over 1 GB** |
| `- ` nested 1,000 deep (2002 bytes) | yaml/v3 | accepted (3 ms, 11 MB) | value built, 2003 bytes as JSON (4 ms, 15 MB) | stopped: `line 1:131: nesting deeper than 64 levels` (1 ms, 10 MB) |
| `- ` nested 1,000 deep (2002 bytes) | yaml/v4 | accepted (1 ms, 10 MB) | value built, 2003 bytes as JSON (5 ms, 15 MB) | stopped: `line 1:131: nesting deeper than 64 levels` (1 ms, 10 MB) |
| `- ` nested 1,000 deep (2002 bytes) | goccy | accepted (5 ms, 15 MB) | value built, 2003 bytes as JSON (3 ms, 15 MB) | stopped: `line 1:131: nesting deeper than 64 levels` (4 ms, 11 MB) |
| `- ` nested 100,000 deep (200002 bytes) | yaml/v3 | stopped: `yaml: exceeded max depth of 10000` (10 ms, 15 MB) | stopped: `yaml: exceeded max depth of 10000` (11 ms, 15 MB) | stopped: `line 0:0: yaml: exceeded max depth of 10000` (11 ms, 19 MB) |
| `- ` nested 100,000 deep (200002 bytes) | yaml/v4 | stopped: `go-yaml load error in scanner (while increasing indent level) at <unknown position>-L1.C20001: exceeded max depth of 10000` (12 ms, 19 MB) | stopped: `go-yaml load error in scanner (while increasing indent level) at <unknown position>-L1.C20001: exceeded max depth of 10000` (12 ms, 19 MB) | stopped: `line 1:20001: go-yaml load error in scanner (while increasing indent level) at <unknown position>-L1.C20001: exceeded max depth of 10000` (10 ms, 15 MB) |
| `- ` nested 100,000 deep (200002 bytes) | goccy | **killed at 544 ms, over 1 GB** | **killed at 541 ms, over 1 GB** | **killed at 553 ms, over 1 GB** |

### Merge keys `<<`

Input: `defaults: &d { timeout: 60s, continue_on_error: true }↵step:↵  <<: *d↵  id: price↵  timeout: 10s↵`

| library | native `step` | Burrow loader |
|---|---|---|
| yaml/v3 | `map[string]any{"continue_on_error": bool true, "id": string "price", "timeout": string "10s"}` | rejected: `line 1:11: anchors (&d) are not allowed; write the value out` |
| yaml/v4 | `map[string]any{"continue_on_error": bool true, "id": string "price", "timeout": string "10s"}` | rejected: `line 1:11: anchors (&d) are not allowed; write the value out` |
| goccy | `map[string]any{"continue_on_error": bool true, "id": string "price", "timeout": string "10s"}` | rejected: `line 1:11: anchors (&d) are not allowed; write the value out` |

## 5. Speed (p50)

Inputs: valid.yaml with its steps repeated. Small: 6386 bytes, 32 steps, 300 runs. Large: 1049370 bytes, 5472 steps, 15 runs.
All inputs are valid, so no error path is timed. Timer: Windows performance counter (clock.go).

| operation | 6 KB | 1024 KB |
|---|---|---|
| yaml/v3 native decode into any | 623 µs | 99.2 ms |
| yaml/v3 Burrow loader (tree + walk + positions) | 605 µs | 119.5 ms |
| yaml/v4 native decode into any | 675 µs | 116.3 ms |
| yaml/v4 Burrow loader (tree + walk + positions) | 662 µs | 128.4 ms |
| goccy native decode into any | 1.2 ms | 204.1 ms |
| goccy Burrow loader (tree + walk + positions) | 1.0 ms | 206.7 ms |
| santhosh v6 validate | 576 µs | 102.2 ms |
| kaptinlin v0.7.7 validate | 1.8 ms | 292.2 ms |
| google v0.4.3 validate | 1.4 ms | 233.8 ms |
| **yaml/v3 Burrow loader + santhosh validate** | 1.8 ms | 275.8 ms |
| same config as JSON: encoding/json decode + santhosh validate | 760 µs | 148.1 ms |
| same config as JSON: yaml/v3 Burrow loader + santhosh validate | 1.5 ms | 225.7 ms |

## 6. JSON → YAML for display

Stored JSON is turned into YAML for the editor. v3/v4: JSON parsed to `yaml.Node`, styles reset, multi-line strings set to literal `|`.
goccy: decoded with `UseOrderedMap`, marshalled with `UseLiteralStyleIfMultiline`. "round trip" = the YAML loads back (Burrow loader of
the same library, and yaml/v3 as a second reader) to exactly the stored JSON value.

| library | input | key order kept | round trip, same lib | round trip, read by v3 | SQL as block scalar | strings that changed |
|---|---|---|---|---|---|---|
| yaml/v3 | valid.json | yes | yes | yes | yes | none |
| yaml/v4 | valid.json | yes | yes | yes | yes | none |
| goccy | valid.json | yes | yes | yes | yes | none |
| yaml/v3 | display.json | yes | yes | yes | yes | none |
| yaml/v4 | display.json | yes | yes | yes | yes | none |
| goccy | display.json | yes | **no** | **no** | yes | `numbers: []interface {} [1 1.5 -0.25 1e+21 0] -> []interface {} [1 1.5 -0.25 1e21 0]; tab: string "a\tb" -> string "ab"; float: string "1e3" -> float64 1000` |

`display.json` rendered by yaml/v3:

```yaml
zeta_first: keys are not sorted
alpha_second: 1
no: no
on: on
yes: yes
octal: "0755"
float: "1e3"
date: "2026-09-28"
tilde: "~"
null_word: "null"
true_word: "true"
clock: 12:30
digits: "123"
empty: ""
leading_space: ' lead'
trailing_space: 'trail '
colon_space: 'a: b'
hash: '#not a comment'
dash: '- not a list'
at: '@at'
star: '*not_alias'
amp: '&not_anchor'
expr: ${{ steps.price.json.close }}
quote: '''single'' and "double"'
tab: "a\tb"
unicode: Grüße, سلام
long: This is a long description line that goes past eighty characters so we can see whether the encoder folds it.
sql: |
  SELECT date, price
  FROM btc_prices
  WHERE date >= :from_date
sql_no_final_newline: |-
  SELECT 1
  UNION SELECT 2
numbers:
  - 1
  - 1.5
  - -0.25
  - 1e21
  - 0
empty_object: {}
empty_list: []
nested:
  b:
    d: 4
    c: 3
  a:
    - y: 1
      x: 2
```

`display.json` rendered by yaml/v4:

```yaml
zeta_first: keys are not sorted
alpha_second: 1
no: no
on: on
yes: yes
octal: '0755'
float: '1e3'
date: '2026-09-28'
tilde: '~'
null_word: 'null'
true_word: 'true'
clock: 12:30
digits: '123'
empty: ''
leading_space: ' lead'
trailing_space: 'trail '
colon_space: 'a: b'
hash: '#not a comment'
dash: '- not a list'
at: '@at'
star: '*not_alias'
amp: '&not_anchor'
expr: ${{ steps.price.json.close }}
quote: '''single'' and "double"'
tab: "a\tb"
unicode: Grüße, سلام
long: This is a long description line that goes past eighty characters so we can see
  whether the encoder folds it.
sql: |
  SELECT date, price
  FROM btc_prices
  WHERE date >= :from_date
sql_no_final_newline: |-
  SELECT 1
  UNION SELECT 2
numbers:
- 1
- 1.5
- -0.25
- 1e21
- 0
empty_object: {}
empty_list: []
nested:
  b:
    d: 4
    c: 3
  a:
  - y: 1
    x: 2
```

`display.json` rendered by goccy:

```yaml
zeta_first: keys are not sorted
alpha_second: 1
"no": "no"
"on": "on"
"yes": "yes"
octal: "0755"
float: 1e3
date: "2026-09-28"
tilde: "~"
null_word: "null"
true_word: "true"
clock: "12:30"
digits: "123"
empty: ""
leading_space: " lead"
trailing_space: "trail "
colon_space: "a: b"
hash: "#not a comment"
dash: "- not a list"
at: "@at"
star: "*not_alias"
amp: "&not_anchor"
expr: ${{ steps.price.json.close }}
quote: "'single' and \"double\""
tab: a	b
unicode: Grüße, سلام
long: This is a long description line that goes past eighty characters so we can see whether the encoder folds it.
sql: |
  SELECT date, price
  FROM btc_prices
  WHERE date >= :from_date
sql_no_final_newline: |-
  SELECT 1
  UNION SELECT 2
numbers:
  - 1
  - 1.5
  - -0.25
  - 1e21
  - 0
empty_object: {}
empty_list: []
nested:
  b:
    d: 4
    c: 3
  a:
    - "y": 1
      x: 2
```

## 7. `format` checks

Schema per row: `{"type": "string", "format": F}`. santhosh "default" = no `AssertFormat()` (2020-12 treats format as an annotation);
"assert" = `AssertFormat()` plus the custom formats of formats.go. kaptinlin: `SetAssertFormat(true)` plus the same custom formats
(its format callback returns only true/false, so the reason is lost). google has no format assertion and no custom formats.
Bold = result differs from the expected one.

| format | value | expected | santhosh default | santhosh assert | kaptinlin assert | google |
|---|---|---|---|---|---|---|
| date-time | `2026-09-27T08:00:03Z` | valid | valid | valid | valid | valid |
| date-time | `2026-09-27 08:00:03` | invalid | **valid** | rejected: `'2026-09-27 08:00:03' is not valid date-time: less than 20 characters long` | rejected: `Value does not match format 'date-time'` | **valid** |
| date-time | `2026-02-30T08:00:00Z` | invalid | **valid** | rejected: `'2026-02-30T08:00:00Z' is not valid date-time: invalid date element: parsing time "2026-02-30": day out of range` | rejected: `Value does not match format 'date-time'` | **valid** |
| date | `2026-09-28` | valid | valid | valid | valid | valid |
| date | `2026-9-28` | invalid | **valid** | rejected: `'2026-9-28' is not valid date: parsing time "2026-9-28" as "2006-01-02": cannot parse "9-28" as "01"` | rejected: `Value does not match format 'date'` | **valid** |
| uri | `https://api.example.com/btc?days=1` | valid | valid | valid | valid | valid |
| uri | `api.example.com/btc` | invalid | **valid** | rejected: `'api.example.com/btc' is not valid uri: relative url` | rejected: `Value does not match format 'uri'` | **valid** |
| uri | `https://exa mple.com/` | invalid | **valid** | rejected: `'https://exa mple.com/' is not valid uri: parse "https://exa mple.com/": invalid character " " in host name` | rejected: `Value does not match format 'uri'` | **valid** |
| email | `someone@example.com` | valid | valid | valid | valid | valid |
| email | `someone@` | invalid | **valid** | rejected: `'someone@' is not valid email: invalid domain: label must be 1 to 63 characters long` | rejected: `Value does not match format 'email'` | **valid** |
| duration | `PT5M` | valid | valid | valid | valid | valid |
| duration | `5m` | invalid | **valid** | rejected: `'5m' is not valid duration: must start with P` | rejected: `Value does not match format 'duration'` | **valid** |
| cron | `0 8 * * *` | valid | valid | valid | valid | valid |
| cron | `*/15 9-17 * * MON-FRI` | valid | valid | valid | valid | valid |
| cron | `0 8 * *` | invalid | **valid** | rejected: `'0 8 * *' is not valid cron: needs 5 fields (minute hour day month weekday), got 4` | rejected: `Value does not match format 'cron'` | **valid** |
| cron | `61 * * * *` | invalid | **valid** | rejected: `'61 * * * *' is not valid cron: minute: "61" is not in 0-59` | rejected: `Value does not match format 'cron'` | **valid** |
| iana-tz | `Europe/Berlin` | valid | valid | valid | valid | valid |
| iana-tz | `Asia/Tehran` | valid | valid | valid | valid | valid |
| iana-tz | `Europe/Berln` | invalid | **valid** | rejected: `'Europe/Berln' is not valid iana-tz: unknown IANA time zone` | rejected: `Value does not match format 'iana-tz'` | **valid** |
| iana-tz | `Local` | invalid | **valid** | rejected: `'Local' is not valid iana-tz: not an IANA time zone name` | rejected: `Value does not match format 'iana-tz'` | **valid** |
| go-duration | `500ms` | valid | valid | valid | valid | valid |
| go-duration | `2h` | valid | valid | valid | valid | valid |
| go-duration | `2d` | invalid | **valid** | rejected: `'2d' is not valid go-duration: not a Go duration like 500ms, 60s, 5m, 2h` | rejected: `Value does not match format 'go-duration'` | **valid** |
| go-duration | `5 minutes` | invalid | **valid** | rejected: `'5 minutes' is not valid go-duration: not a Go duration like 500ms, 60s, 5m, 2h` | rejected: `Value does not match format 'go-duration'` | **valid** |
| http-url | `https://api.example.com/btc/daily` | valid | valid | valid | valid | valid |
| http-url | `https://api.example.com/${{ inputs.coin }}` | valid | valid | valid | valid | valid |
| http-url | `${{ item.url }}` | valid | valid | valid | valid | valid |
| http-url | `ftp://api.example.com/btc` | invalid | **valid** | rejected: `'ftp://api.example.com/btc' is not valid http-url: only http and https URLs are allowed` | rejected: `Value does not match format 'http-url'` | **valid** |
| http-url | `file:///C:/Windows/win.ini` | invalid | **valid** | rejected: `'file:///C:/Windows/win.ini' is not valid http-url: only http and https URLs are allowed` | rejected: `Value does not match format 'http-url'` | **valid** |
