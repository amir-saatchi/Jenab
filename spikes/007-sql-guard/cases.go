package main

type queryCase struct {
	cat   string
	sql   string // {OTHER} and {DIR} are replaced with paths
	allow bool
}

var queryCases = []queryCase{
	// Normal queries that must pass
	{"allowed", "SELECT * FROM data", true},
	{"allowed", "SELECT name, avg(val) AS a FROM data WHERE val > :v GROUP BY name ORDER BY a DESC LIMIT 10", true},
	{"allowed", "WITH t AS (SELECT * FROM data) SELECT count(*) FROM t", true},
	{"allowed", "SELECT d.name, c.note FROM data d JOIN child c ON c.parent_id = d.id", true},
	{"allowed", "SELECT * FROM data WHERE name = 'a;b'", true},
	{"allowed", "SELECT * FROM data WHERE name = '_burrow_meta'", true},
	{"allowed", "SELECT 1;", true},
	{"allowed", "SELECT \"name\" FROM \"data\" /* note */ -- trailing comment", true},
	{"allowed", "SELECT d.id, j.value FROM data d, json_each(d.tags) j LIMIT 5", true},
	{"allowed", "VALUES (1), (2)", true},
	{"allowed", "WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i+1 FROM n WHERE i < 100) SELECT sum(i) FROM n", true},

	// Reading internal tables
	{"internal read", "SELECT * FROM _burrow_meta", false},
	{"internal read", "SELECT * FROM \"_burrow_meta\"", false},
	{"internal read", "SELECT * FROM [_BURROW_META]", false},
	{"internal read", "SELECT * FROM `_burrow_meta`", false},
	{"internal read", "SELECT * FROM main._burrow_meta", false},
	{"internal read", "WITH x AS (SELECT * FROM _burrow_meta) SELECT * FROM x", false},
	{"internal read", "SELECT * FROM (SELECT * FROM _burrow_meta)", false},
	{"internal read", "SELECT * FROM data WHERE id IN (SELECT id FROM _burrow_changes)", false},
	{"internal read", "SELECT * FROM data WHERE name IN _burrow_flags", false},
	{"internal read", "SELECT (SELECT value FROM _burrow_meta LIMIT 1) FROM data", false},
	{"internal read", "SELECT name FROM data UNION SELECT key FROM _burrow_meta", false},
	{"internal read", "SELECT * FROM data ORDER BY (SELECT count(*) FROM _burrow_changes)", false},
	{"internal read", "SELECT * FROM data WHERE EXISTS (SELECT 1 FROM _burrow_meta WHERE key = 'x')", false},
	{"internal read", "SELECT key FROM _burrow_meta WHERE key = 'project_id'", false},
	{"schema read", "SELECT * FROM sqlite_schema", false},
	{"schema read", "SELECT * FROM sqlite_master", false},
	{"schema read", "SELECT * FROM main.sqlite_schema", false},
	{"schema read", "SELECT * FROM sqlite_temp_schema", false},

	// Schema information through virtual tables
	{"virtual table", "SELECT * FROM pragma_table_info('_burrow_meta')", false},
	{"virtual table", "SELECT * FROM \"pragma_table_info\"('_burrow_meta')", false},
	{"virtual table", "SELECT * FROM pragma_table_list", false},
	{"virtual table", "SELECT * FROM pragma_database_list", false},
	{"virtual table", "SELECT * FROM dbstat", false},
	{"virtual table", "SELECT * FROM sqlite_dbpage", false},
	{"virtual table", "SELECT * FROM bytecode('SELECT * FROM _burrow_meta')", false},

	// Writes
	{"write", "DELETE FROM data", false},
	{"write", "UPDATE data SET name = 'x'", false},
	{"write", "REPLACE INTO data(id, name) VALUES (1, 'x')", false},
	{"write", "WITH x AS (SELECT 1) DELETE FROM data", false},
	{"write", "WITH x AS (SELECT 1) INSERT INTO data(name) VALUES ('x')", false},
	{"write", "CREATE TEMP TABLE t AS SELECT * FROM _burrow_meta", false},
	{"write", "CREATE TEMP VIEW v AS SELECT * FROM data", false},

	// Connection state and files
	{"connection", "PRAGMA query_only = 0", false},
	{"connection", "PRAGMA writable_schema = 1", false},
	{"connection", "ATTACH '{OTHER}' AS o", false},
	{"connection", "VACUUM INTO '{DIR}/copy.db'", false},
	{"connection", "BEGIN IMMEDIATE", false},
	{"connection", "ANALYZE", false},

	// A second statement hidden after the first
	{"multi-statement", "SELECT 1; DELETE FROM data", false},
	{"multi-statement", "SELECT 1; PRAGMA query_only = 0", false},
	{"multi-statement", "SELECT ';'; DELETE FROM data", false},
	{"multi-statement", "SELECT 1 /* ; */; DELETE FROM data", false},
	{"multi-statement", "SELECT 1 -- x\n; DELETE FROM data", false},
	{"multi-statement", "SELECT 1; ATTACH '{OTHER}' AS o", false},
	{"multi-statement", "SELECT 1\x00; DELETE FROM data", false},

	// Dangerous functions
	{"function", "SELECT load_extension('evil')", false},
	{"function", "SELECT fts3_tokenizer('simple')", false},

	// Resource exhaustion (blocked by limits and the timeout)
	{"exhaustion", "WITH RECURSIVE r(i) AS (SELECT 1 UNION ALL SELECT i+1 FROM r) SELECT count(*) FROM r", false},
	{"exhaustion", "SELECT count(*) FROM data a, data b, data c, data d", false},
	{"exhaustion", "SELECT length(randomblob(200000000))", false},
	{"exhaustion", "SELECT length(printf('%.*c', 200000000, 'x'))", false},
	{"exhaustion", "WITH RECURSIVE r(i, s) AS (SELECT 1, 'x' UNION ALL SELECT i+1, s || s FROM r WHERE i < 40) SELECT max(length(s)) FROM r", false},
}
