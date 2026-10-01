# SPIKE-013 — YAML and JSON Schema
**Type:** Spike
**Status:** Done
**Gate:** 2

## Question
Which YAML library and which JSON Schema validator should parse and check views, pipelines and configs (SPEC 1, 10)?

1. YAML and JSON parse to the same structure; YAML 1.1 surprises (`no`, `on`, `0755`, dates) are handled.
2. Every validation error points to a line and column in the YAML, and all errors come back together (SPEC 10).
3. Duplicate keys are rejected, and alias bombs are stopped.
4. Error messages are clear enough for an LLM to fix everything in one attempt.
5. Speed for a large view or pipeline file.

Candidates: `go.yaml.in/yaml/v3`, yaml v4 RC, `goccy/go-yaml`; `santhosh-tekuri/jsonschema/v6`, `kaptinlin/jsonschema`, `google/jsonschema-go`.

## Done when
One YAML library and one validator are chosen, with how YAML positions are joined to schema errors.

## Result
Run on 2026-09-28 with Go 1.26.0, no cgo. Code, fixtures and full output are in `spikes/013-yaml-and-json-schema/` (`results.md`). The test used a pipeline schema written from SPEC 6, a valid sample and 24 broken files with 47 expected errors.

**YAML**

| | `go.yaml.in/yaml/v3` v3.0.5 (2026-07-26) | `yaml/v4` v4.0.0-rc.6 (2026-06-17, RC) | `goccy/go-yaml` v1.19.2 (2026-01-08) |
|---|---|---|---|
| Error lines right (24 files) | 23 | 23 | 22 |
| Duplicate keys in the node tree | accepted | accepted | rejected |
| Alias bomb (10^9 strings) | stopped in 1 ms | stopped in 1 ms | killed above 1 GB |
| 100,000 levels deep | stopped at depth 10,000 | stopped | killed above 1 GB |
| Load 6 KB / 1 MB | 0.6 ms / 120 ms | 0.7 ms / 128 ms | 1.0 ms / 207 ms |
| JSON → YAML → JSON gives the same value | yes | yes | no (a tab lost, `1e3` changed) |

**JSON Schema**

| | Expected errors found (of 47) | Extra errors | Validate 6 KB / 1 MB |
|---|---|---|---|
| `santhosh-tekuri/jsonschema/v6` v6.0.3 (2026-06-28), Apache-2.0 | 46 | 1 | 0.6 ms / 102 ms |
| `kaptinlin/jsonschema` v0.7.7 | 45 | 22 | 1.8 ms / 292 ms |
| `google/jsonschema-go` v0.4.3 | stops at the first error | — | 1.4 ms / 234 ms |

**Findings**
- **santhosh:**
  - Returns every error in one pass, with clear messages, e.g. `line 4:13 trigger.schedule: '0 8 * *' is not valid cron: needs 5 fields`.
  - One bug: `propertyNames` errors get the wrong path. This is the missed error, and the same file in every YAML library.
- **kaptinlin:**
  - Newer releases (v0.7.8 to v0.9.10) need Go 1.26.2 or newer, so v0.7.7 was tested.
  - It adds noise, such as "Value is null" next to "missing property" and leaked `anyOf` branches.
- **google:**
  - Stops at the first error, which differed in 3 of 50 runs.
  - Ignores `format`, so a bad cron or time zone passes.
- **YAML 1.1:**
  - All three libraries read `no`, `on`, `yes` and `off` as strings.
  - All three turn `0755` and `010` into octal numbers.
  - v3 and v4 turn dates into timestamps; Jenab's loader keeps them as text.
- **Duplicate keys:** `encoding/json` and santhosh's JSON reader keep the last duplicate key silently, so JSON input needs its own duplicate check too.
- **Not tested:**
  - view schemas
  - custom keywords
  - comments kept through editing
  - multi-document files
  - yaml v4's own limit options

## Decision
**Decided (2026-09-28):**
- **YAML:** `go.yaml.in/yaml/v3` through `yaml.Node`; move to v4 when it reaches GA.
- **JSON Schema:** `santhosh-tekuri/jsonschema/v6` with format checks on and custom formats `cron`, `iana-tz`, `go-duration` and `http-url`.
- **Positions:**
  - One Go walk over the node tree builds the value and a map from JSON pointer to line:column.
  - Missing fields, unknown fields and whole objects point at the key; everything else points at the value.
  - Errors show the SPEC path, e.g. `steps[2].with.table`.
- **Rules Go adds:**
  - Reject:
    - duplicate keys (also in JSON)
    - anchors, aliases, merge keys and tags
    - non-string keys
    - `NaN` and `Inf`
    - leading-zero numbers
    - depth over 64
  - Cap the input size.
  - Keep dates as text.
  - Check unique step IDs and `propertyNames` in Go.
- **Display:** reset node styles and use `|` for multi-line strings.

The rules are in SPEC 1 and 10.
