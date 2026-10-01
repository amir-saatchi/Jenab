# SPIKE-007 results: github.com/ncruces/go-sqlite3 v0.35.6

SQLite 3.53.4. Guarded readers: `query_only`, `SQLITE_LIMIT_ATTACHED = 0`, `SQLITE_LIMIT_LENGTH = 16 MiB`, timeout 2s.

Virtual table modules in this build: fts5, fts5vocab, generate_series, pragma_module_list.

## Connection behaviour

| Check | Result |
|---|---|
| Default `SQLITE_LIMIT_ATTACHED` / `SQLITE_LIMIT_LENGTH` | 10 / 1000000000 |
| `ATTACH` on `query_only` without the limit | succeeded |
| `ATTACH` with `SQLITE_LIMIT_ATTACHED = 0` | error: sqlite3: SQL logic error: too many attached databases - max 0 |
| Only *prepare* `PRAGMA query_only = 0`, never execute | prepare: succeeded; `query_only` is now **0** |
| `EXPLAIN PRAGMA query_only = 0` | succeeded; `query_only` is now **0** |

## Agent query cases

Each layer is tested on its own, on a fresh connection. In Burrow they run in order: 1 → 2 → 3, and a query stops at the first layer that rejects it.

- **1 Text**: single statement, starts with SELECT/WITH/VALUES, no internal, reserved, `pragma_*` or denied names
- **2 EXPLAIN**: compiled program opens no internal table, `sqlite_schema` or virtual table (except `json_each`/`json_tree`), no write opcodes
- **3 Connection**: the raw query on a guarded reader (`query_only`, limits, timeout)

| # | Kind | Query | Expect | 1 Text | 2 EXPLAIN | 3 Connection | Result |
|---|---|---|---|---|---|---|---|
| 1 | allowed | `SELECT * FROM data` | allow | pass | pass | ran, 200 rows | OK |
| 2 | allowed | `SELECT name, avg(val) AS a FROM data WHERE val > :v GROUP BY name ORDER BY a DESC LIMIT 10` | allow | pass | pass | ran, 0 rows | OK |
| 3 | allowed | `WITH t AS (SELECT * FROM data) SELECT count(*) FROM t` | allow | pass | pass | ran, 1 rows | OK |
| 4 | allowed | `SELECT d.name, c.note FROM data d JOIN child c ON c.parent_id = d.id` | allow | pass | pass | ran, 100 rows | OK |
| 5 | allowed | `SELECT * FROM data WHERE name = 'a;b'` | allow | pass | pass | ran, 0 rows | OK |
| 6 | allowed | `SELECT * FROM data WHERE name = '_burrow_meta'` | allow | pass | pass | ran, 0 rows | OK |
| 7 | allowed | `SELECT 1;` | allow | pass | pass | ran, 1 rows | OK |
| 8 | allowed | `SELECT "name" FROM "data" /* note */ -- trailing comment` | allow | pass | pass | ran, 200 rows | OK |
| 9 | allowed | `SELECT d.id, j.value FROM data d, json_each(d.tags) j LIMIT 5` | allow | pass | pass | ran, 5 rows | OK |
| 10 | allowed | `VALUES (1), (2)` | allow | pass | pass | ran, 2 rows | OK |
| 11 | allowed | `WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i+1 FROM n WHERE i < 100) SELECT sum(i) FROM n` | allow | pass | pass | ran, 1 rows | OK |
| 12 | internal read | `SELECT * FROM _burrow_meta` | block | **reject**: internal name _burrow_meta | **reject**: reads _burrow_meta | ran, 2 rows | OK |
| 13 | internal read | `SELECT * FROM "_burrow_meta"` | block | **reject**: internal name _burrow_meta | **reject**: reads _burrow_meta | ran, 2 rows | OK |
| 14 | internal read | `SELECT * FROM [_BURROW_META]` | block | **reject**: internal name _BURROW_META | **reject**: reads _burrow_meta | ran, 2 rows | OK |
| 15 | internal read | `` SELECT * FROM `_burrow_meta` `` | block | **reject**: internal name _burrow_meta | **reject**: reads _burrow_meta | ran, 2 rows | OK |
| 16 | internal read | `SELECT * FROM main._burrow_meta` | block | **reject**: internal name _burrow_meta | **reject**: reads _burrow_meta | ran, 2 rows | OK |
| 17 | internal read | `WITH x AS (SELECT * FROM _burrow_meta) SELECT * FROM x` | block | **reject**: internal name _burrow_meta | **reject**: reads _burrow_meta | ran, 2 rows | OK |
| 18 | internal read | `SELECT * FROM (SELECT * FROM _burrow_meta)` | block | **reject**: internal name _burrow_meta | **reject**: reads _burrow_meta | ran, 2 rows | OK |
| 19 | internal read | `SELECT * FROM data WHERE id IN (SELECT id FROM _burrow_changes)` | block | **reject**: internal name _burrow_changes | **reject**: reads _burrow_changes | ran, 1 rows | OK |
| 20 | internal read | `SELECT * FROM data WHERE name IN _burrow_flags` | block | **reject**: internal name _burrow_flags | **reject**: reads _burrow_flags | ran, 0 rows | OK |
| 21 | internal read | `SELECT (SELECT value FROM _burrow_meta LIMIT 1) FROM data` | block | **reject**: internal name _burrow_meta | **reject**: reads _burrow_meta | ran, 200 rows | OK |
| 22 | internal read | `SELECT name FROM data UNION SELECT key FROM _burrow_meta` | block | **reject**: internal name _burrow_meta | **reject**: reads _burrow_meta | ran, 52 rows | OK |
| 23 | internal read | `SELECT * FROM data ORDER BY (SELECT count(*) FROM _burrow_changes)` | block | **reject**: internal name _burrow_changes | **reject**: reads _burrow_changes | ran, 200 rows | OK |
| 24 | internal read | `SELECT * FROM data WHERE EXISTS (SELECT 1 FROM _burrow_meta WHERE key = 'x')` | block | **reject**: internal name _burrow_meta | **reject**: reads _burrow_meta | ran, 0 rows | OK |
| 25 | internal read | `SELECT key FROM _burrow_meta WHERE key = 'project_id'` | block | **reject**: internal name _burrow_meta | **reject**: reads _burrow_meta | ran, 1 rows | OK |
| 26 | schema read | `SELECT * FROM sqlite_schema` | block | **reject**: reserved name sqlite_schema | **reject**: reads root page 1 (sqlite_schema) | ran, 7 rows | OK |
| 27 | schema read | `SELECT * FROM sqlite_master` | block | **reject**: reserved name sqlite_master | **reject**: reads root page 1 (sqlite_schema) | ran, 7 rows | OK |
| 28 | schema read | `SELECT * FROM main.sqlite_schema` | block | **reject**: reserved name sqlite_schema | **reject**: reads root page 1 (sqlite_schema) | ran, 7 rows | OK |
| 29 | schema read | `SELECT * FROM sqlite_temp_schema` | block | **reject**: reserved name sqlite_temp_schema | **reject**: reads schema 1 (temp or attached) | ran, 0 rows | OK |
| 30 | virtual table | `SELECT * FROM pragma_table_info('_burrow_meta')` | block | **reject**: pragma function pragma_table_info | **reject**: virtual table | ran, 2 rows | OK |
| 31 | virtual table | `SELECT * FROM "pragma_table_info"('_burrow_meta')` | block | **reject**: pragma function pragma_table_info | **reject**: virtual table | ran, 2 rows | OK |
| 32 | virtual table | `SELECT * FROM pragma_table_list` | block | **reject**: pragma function pragma_table_list | **reject**: virtual table | ran, 7 rows | OK |
| 33 | virtual table | `SELECT * FROM pragma_database_list` | block | **reject**: pragma function pragma_database_list | **reject**: virtual table | ran, 1 rows | OK |
| 34 | virtual table | `SELECT * FROM dbstat` | block | **reject**: denied name dbstat | **reject**: prepare failed: sqlite3: SQL logic error: no such table: dbstat | error: sqlite3: SQL logic error: no such table: dbstat | OK |
| 35 | virtual table | `SELECT * FROM sqlite_dbpage` | block | **reject**: reserved name sqlite_dbpage | **reject**: prepare failed: sqlite3: SQL logic error: no such table: sqlite_dbpage | error: sqlite3: SQL logic error: no such table: sqlite_dbpage | OK |
| 36 | virtual table | `SELECT * FROM bytecode('SELECT * FROM _burrow_meta')` | block | **reject**: denied name bytecode | **reject**: prepare failed: sqlite3: SQL logic error: no such table: bytecode | error: sqlite3: SQL logic error: no such table: bytecode | OK |
| 37 | write | `DELETE FROM data` | block | **reject**: must start with SELECT, WITH or VALUES | **reject**: opcode OpenWrite | error: sqlite3: attempt to write a readonly database | OK |
| 38 | write | `UPDATE data SET name = 'x'` | block | **reject**: must start with SELECT, WITH or VALUES | **reject**: opcode OpenWrite | error: sqlite3: attempt to write a readonly database | OK |
| 39 | write | `REPLACE INTO data(id, name) VALUES (1, 'x')` | block | **reject**: must start with SELECT, WITH or VALUES | **reject**: opcode OpenWrite | error: sqlite3: attempt to write a readonly database | OK |
| 40 | write | `WITH x AS (SELECT 1) DELETE FROM data` | block | pass | **reject**: opcode OpenWrite | error: sqlite3: attempt to write a readonly database | OK |
| 41 | write | `WITH x AS (SELECT 1) INSERT INTO data(name) VALUES ('x')` | block | pass | **reject**: opcode OpenWrite | error: sqlite3: attempt to write a readonly database | OK |
| 42 | write | `CREATE TEMP TABLE t AS SELECT * FROM _burrow_meta` | block | **reject**: must start with SELECT, WITH or VALUES | **reject**: opcode SetCookie | error: sqlite3: attempt to write a readonly database | OK |
| 43 | write | `CREATE TEMP VIEW v AS SELECT * FROM data` | block | **reject**: must start with SELECT, WITH or VALUES | **reject**: opcode SetCookie | error: sqlite3: attempt to write a readonly database | OK |
| 44 | connection | `PRAGMA query_only = 0` | block | **reject**: must start with SELECT, WITH or VALUES | pass ⚠ state changed | ran, 0 rows ⚠ query_only turned off | OK |
| 45 | connection | `PRAGMA writable_schema = 1` | block | **reject**: must start with SELECT, WITH or VALUES | pass | ran, 0 rows | OK |
| 46 | connection | `ATTACH '{OTHER}' AS o` | block | **reject**: must start with SELECT, WITH or VALUES | **reject**: calls sqlite_attach | error: sqlite3: SQL logic error: too many attached databases - max 0 | OK |
| 47 | connection | `VACUUM INTO '{DIR}/copy.db'` | block | **reject**: must start with SELECT, WITH or VALUES | **reject**: opcode Vacuum | error: sqlite3: SQL logic error: too many attached databases - max 0 | OK |
| 48 | connection | `BEGIN IMMEDIATE` | block | **reject**: must start with SELECT, WITH or VALUES | **reject**: write transaction | error: sqlite3: attempt to write a readonly database | OK |
| 49 | connection | `ANALYZE` | block | **reject**: must start with SELECT, WITH or VALUES | **reject**: opcode SetCookie | error: sqlite3: attempt to write a readonly database | OK |
| 50 | multi-statement | `SELECT 1; DELETE FROM data` | block | **reject**: more than one statement | **reject**: prepare failed: sqlite3: multiple statements | error: sqlite3: multiple statements | OK |
| 51 | multi-statement | `SELECT 1; PRAGMA query_only = 0` | block | **reject**: more than one statement | **reject**: prepare failed: sqlite3: multiple statements | error: sqlite3: multiple statements | OK |
| 52 | multi-statement | `SELECT ';'; DELETE FROM data` | block | **reject**: more than one statement | **reject**: prepare failed: sqlite3: multiple statements | error: sqlite3: multiple statements | OK |
| 53 | multi-statement | `SELECT 1 /* ; */; DELETE FROM data` | block | **reject**: more than one statement | **reject**: prepare failed: sqlite3: multiple statements | error: sqlite3: multiple statements | OK |
| 54 | multi-statement | `SELECT 1 -- x↵; DELETE FROM data` | block | **reject**: more than one statement | **reject**: prepare failed: sqlite3: multiple statements | error: sqlite3: multiple statements | OK |
| 55 | multi-statement | `SELECT 1; ATTACH '{OTHER}' AS o` | block | **reject**: more than one statement | **reject**: prepare failed: sqlite3: multiple statements | error: sqlite3: multiple statements | OK |
| 56 | multi-statement | `SELECT 1\0; DELETE FROM data` | block | **reject**: NUL byte in query | pass | ran, 1 rows | OK |
| 57 | function | `SELECT load_extension('evil')` | block | **reject**: denied name load_extension | **reject**: prepare failed: sqlite3: SQL logic error: no such function: load_ex... | error: sqlite3: SQL logic error: no such function: load_extension | OK |
| 58 | function | `SELECT fts3_tokenizer('simple')` | block | **reject**: denied name fts3_tokenizer | **reject**: prepare failed: sqlite3: SQL logic error: no such function: fts3_to... | error: sqlite3: SQL logic error: no such function: fts3_tokenizer | OK |
| 59 | exhaustion | `WITH RECURSIVE r(i) AS (SELECT 1 UNION ALL SELECT i+1 FROM r) SELECT count(*) FROM r` | block | pass | pass | error: sqlite3: interrupted (2s) | OK |
| 60 | exhaustion | `SELECT count(*) FROM data a, data b, data c, data d` | block | pass | pass | error: sqlite3: interrupted (2s) | OK |
| 61 | exhaustion | `SELECT length(randomblob(200000000))` | block | pass | pass | error: sqlite3: string or blob too big | OK |
| 62 | exhaustion | `SELECT length(printf('%.*c', 200000000, 'x'))` | block | pass | pass | error: sqlite3: string or blob too big | OK |
| 63 | exhaustion | `WITH RECURSIVE r(i, s) AS (SELECT 1, 'x' UNION ALL SELECT i+1, s \|\| s FROM r WHERE i < 40) SELECT max(length(s)) FROM r` | block | pass | pass | error: sqlite3: string or blob too big | OK |

**Agent queries: 63 of 63 cases correct.** Blocked cases stopped by layer 1: 45, layer 2: 2, layer 3: 5.

## Cost per query

Typical view query (GROUP BY over 2,000 rows), average of 2,000 runs, root-page map cached by `schema_version`.

| Step | Time |
|---|---|
| 1 Text check | 2.3µs |
| 2 EXPLAIN check | 76µs |
| Running the query itself | 1.053ms |

## Schema guard (migrations)

Each case runs in `BEGIN IMMEDIATE`, then the guard, then `ROLLBACK`. The writer connection has `SQLITE_LIMIT_ATTACHED = 0`.

| Case | Expect | SQLite | Guard | Rolled back clean | Result |
|---|---|---|---|---|---|
| create table with composite key | pass | ok | pass | true | OK |
| create WITHOUT ROWID table | pass | ok | pass | true | OK |
| add column | pass | ok | pass | true | OK |
| rename column | pass | ok | pass | true | OK |
| create index | pass | ok | pass | true | OK |
| rebuild table (documented procedure) | pass | ok | pass | true | OK |
| drop internal table | reject | ok | **reject**: internal schema changed | true | OK |
| rename internal table | reject | ok | **reject**: internal schema changed | true | OK |
| add column to internal table | reject | ok | **reject**: internal schema changed | true | OK |
| index on internal table | reject | ok | **reject**: internal schema changed | true | OK |
| new table with internal name | reject | ok | **reject**: internal schema changed | true | OK |
| rename agent table to internal name | reject | ok | **reject**: internal schema changed | true | OK |
| trigger that clears the change log | reject | ok | **reject**: trigger t not allowed | true | OK |
| temp trigger on the writer connection | reject | ok | **reject**: temp schema objects not allowed | true | OK |
| SQL view | reject | ok | **reject**: view v not allowed | true | OK |
| table without primary key | reject | ok | **reject**: table nopk has no primary key | true | OK |
| virtual table | reject | ok | **reject**: virtual table f not allowed | true | OK |
| rebuild that loses parent rows | reject | ok | **reject**: foreign key violation in child (parent data) | true | OK |
| attach on the writer | reject | error: sqlite3: SQL logic error: too many attached databases - max 0 | - | true | OK |

**Schema guard: 19 of 19 cases correct.**

Guard cost with 104 agent tables: 1.072ms per migration.
