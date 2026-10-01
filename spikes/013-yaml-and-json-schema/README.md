# SPIKE-013 — YAML and JSON Schema

Throwaway code for [SPIKE-013](../../Docs/Tickets/SPIKE-013-yaml-and-json-schema.md).

```bash
go run . > results.md    # about 1 minute; pure Go, builds with CGO_ENABLED=0
```

- `fixtures/pipeline.schema.json`: a 2020-12 JSON Schema for pipelines (SPEC 6), with per-step `with` schemas through `allOf` + `if`/`then`
- `fixtures/valid.yaml`, `fixtures/valid.json`: the same valid pipeline in both formats; `fixtures/display.json`: strings that need quoting, for test 6
- `fixtures/broken/`: 24 broken files; each lists its expected errors as `# expect: LINE POINTER KEYWORD` (JSON files in a `.expect` side file)
- `load.go`: the "Burrow loader" for yaml/v3, yaml/v4 and goccy: parse to the syntax tree, then one walk that builds the value, maps every JSON pointer to line:column, and rejects anchors, aliases, merge keys, tags, non-string keys, duplicate keys and nesting over 64
- `validate.go`: the three validators, error flattening (leaf errors; `anyOf` kept as one error), pointer → position, SPEC-style path (`steps[2].with.table`)
- `formats.go`: custom formats `cron`, `iana-tz`, `go-duration`, `http-url` (literal part only when the URL holds `${{ }}`)
- `errors.go`, `yaml11.go`, `safety.go`, `speed.go`, `display.go`, `formattest.go`: tests 1–7
- `clock.go`: high-resolution timer (copied from SPIKE-005)
- `results.md`: output of the last run

The bomb and deep-nesting cases run in a child process (the same binary, `SPIKE013_CHILD` set) with a 10 s timeout and a 1 GB memory watchdog.

kaptinlin/jsonschema is pinned to v0.7.7 (2026-03-29): every release since v0.7.8 needs Go 1.26.2 or newer (v0.9.10 needs 1.27.0), so it does not build on Go 1.26.0 without downloading a toolchain.

goccy's block-mapping token is its first `:`, so `load.go` uses the first key's position for mappings, the same as `yaml.Node`.
