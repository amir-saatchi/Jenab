# SPIKE-002 — Library survey
**Type:** Spike
**Status:** Done
**Gate:** 2

## Question
Which libraries are available for each need, and which should we pick?

Needs: desktop shell, SQLite driver (see SPIKE-001), YAML, JSON Schema validation, expression engine, cron parsing, keychain, object store interface, readable-text extraction from HTML, LLM SDKs (Anthropic, OpenAI-compatible, local), ULID, frontend table and chart components.

For each option: maintained (last release), license, cgo needed, fit with SPEC.

## Done when
Every need has 2–3 options compared and one recommended.

## Result
Checked on 2026-09-27. Versions come from the Go module proxy and the npm registry; features from pkg.go.dev, GitHub and project docs. Every recommended Go library is cgo-free, except the macOS part of Wails.

| Need | Recommended | Version (date) | License | Alternatives considered | Why |
|---|---|---|---|---|---|
| Desktop shell | Wails v3 | v3.0.0-beta.26 (2026-09-25) | MIT | Wails v2 v2.16.0 (no system tray, open issue #4990); webview_go (window only) | Native tray and an asset handler; v2 will not get a tray. Decided in SPIKE-003: pin the tag, use the OS frame, rerun its self-test before each upgrade. Fallback: v2.16 + `fyne.io/systray` |
| SQLite driver | `modernc.org/sqlite` (**decided**) | v1.59.0 | BSD-3 | ncruces/go-sqlite3 v0.35.6 (fallback; has an authorizer); mattn/go-sqlite3 (needs cgo) | Most used cgo-free driver, `database/sql`, FTS5 built in, `sqlite.Limit`. No authorizer needed: SQL guard from SPIKE-007 |
| YAML | `go.yaml.in/yaml/v3` via `yaml.Node` | v3.0.5 (2026-07-26) | MIT + Apache-2.0 | gopkg.in/yaml.v3 (archived 2025); yaml/v4 (still an RC); goccy/go-yaml | Official continuation; line and column on nodes. Move to v4 when it reaches GA. Tested in SPIKE-013, with the extra rules Go must add |
| JSON Schema | `github.com/santhosh-tekuri/jsonschema/v6` | v6.0.3 (2026-06-28) | Apache-2.0 | kaptinlin/jsonschema (pre-1.0); google/jsonschema-go (ignores `format`) | Full 2020-12 support, errors with JSON-pointer paths. Tested in SPIKE-013, with the extra rules Go must add |
| Expressions | `github.com/expr-lang/expr` | v1.17.8 (2026-02-14) | MIT | google/cel-go (no `??`, heavier) | Has `??` and `?.`, builtin allowlist, `MaxNodes`, memory budget. No wall-clock timeout (SPIKE-005) |
| Cron parsing | `github.com/adhocore/gronx` (matching and next run only), behind our own syntax check and DST layer | v1.20.5 (2026-09-27) | MIT | netresearch/go-cron v0.16.1 (default AND day matching; fallback with `DowOrDom`); robfig/cron v3 (no release since 2020); hashicorp/cronexpr | Changed by SPIKE-014: gronx was the only library with correct day matching and calendar edge cases (67/67 with the DST layer) |
| Keychain | `github.com/zalando/go-keyring` | v0.2.8 (2026-03-23) | MIT | 99designs/keyring (abandoned); byteness/keyring (cgo on macOS) | cgo-free on all three OSes. Size limits (~2.5–3 KB) are fine for API keys (SPIKE-006) |
| Object store | Own small interface, local folder | — | — | gocloud.dev/blob v0.46.0 (large dependency tree); minio-go v7.3.0 | About 200 lines for v1; add an S3 backend (minio-go) later behind the same interface |
| Readable text | `codeberg.org/readeck/go-readability/v2`, then `JohannesKaufmann/html-to-markdown/v2` | v2.1.2 (2026-06-18) / v2.5.2 (2026-06-07) | MIT / MIT | go-trafilatura v2.2.2 (slightly better text, 2–3× the memory); go-shiori (deprecated); go-domdistiller | Changed by SPIKE-015: readeck came close on text (98 vs 100 of 109), used the least memory and had no slow first call. Go decodes the charset first |
| LLM SDKs | `anthropics/anthropic-sdk-go` + `openai/openai-go/v3`, behind our own `Provider` interface | v1.75.0 / v3.66.0 (2026-09) | MIT / Apache-2.0 | genkit, eino, any-llm-go, langchaingo (frameworks, some stale) | The harness owns its agent loop; local models via openai-go with a custom base URL (to verify). Tested in SPIKE-012 against Genkit, Eino, any-llm-go and goai; the rules Go adds are in SPEC 3.8 |
| IDs | `github.com/oklog/ulid/v2` | v2.1.2 (2026-07-23) | Apache-2.0 | google/uuid v7 | Sortable 26-character IDs, as SPEC 2.1 uses |
| Starlark | `go.starlark.net` | commit 89a6a09411d5 (2026-09-08, no tags) | BSD-3 | goja, gopher-lua, risor, tengo (SPIKE-016) | Only engine with nothing reachable by default plus a step limit. No memory limit, so it runs in a child process capped by a Job Object (SPIKE-016) |
| SQL table extraction fallback | Not needed | — | — | `github.com/rqlite/sql` | The text check and EXPLAIN check from SPIKE-007 cover it |
| Frontend table | `@tanstack/react-table` | 9.2.4 (2026-08-28) | MIT | ag-grid-community 36.2.0 (much larger) | Headless and small. v9 API differs from older v8 tutorials |
| Frontend charts | `recharts` | 3.10.1 (2026-07-25) | MIT | echarts 6.1.0 (larger); uPlot 1.6.32 (tiny, low-level) | React-style API. Add uPlot only if large time series get slow |
| Frontend components | `shadcn/ui` (components copied into the repo by the `shadcn` CLI) | CLI 4.21.0 (2026-09-04) | MIT | — | Added at the user's request |
| Frontend state | `zustand` | 5.0.15 (2026-08-13) | MIT | — | Added at the user's request |
| Frontend tooling | Bun (installs packages and runs scripts, instead of npm) | 1.4.2 (2026-09-05); 1.3.2 on this machine | MIT | npm, pnpm | The user's choice. Used for installs, scripts and the Wails frontend build |

**Risks and open points**
- **Wails v3 is beta** and releases almost daily. Pin the exact tag. macOS builds likely need Xcode command-line tools (not confirmed).
- **SQLite driver:** pin the modernc version; the SPIKE-001 and SPIKE-007 checks become regression tests.
- **Cron:** tested in SPIKE-014; gronx was chosen, and go-cron defaults to AND day matching.
- **Local-model base URL:** confirmed in SPIKE-012 (openai-go against Ollama's OpenAI-compatible endpoint).
- **Bundle sizes** above are whole-package figures; tree-shaken sizes will be smaller.

## Decision
**Decided (2026-09-28):** the "Recommended" column, as changed by the spikes that tested each need:
- **SQLite driver:** SPIKE-001
- **Desktop shell:** SPIKE-003
- **Expressions:** SPIKE-005
- **Keychain on Windows:** SPIKE-006
- **LLM SDKs:** SPIKE-012
- **YAML and JSON Schema:** SPIKE-013
- **Cron:** SPIKE-014, which changed it to gronx
- **Readable text:** SPIKE-015, which changed it to readeck
- **Starlark:** SPIKE-016
- **Frontend:** React tables, charts, shadcn/ui and Zustand, confirmed by the user, with Bun instead of npm
- **IDs, the object store, and no SQL table-extraction library:** taken as recommended, with no spike needed

The rows go into Gate-2 and the proposal's technology table.
