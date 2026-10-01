package main

// Engine keys, in table order.
const (
	eStar = "starlark"
	eGoja = "goja"
	eLua  = "gopher-lua"
	eRis  = "risor"
	eTen  = "tengo"
)

// hostile is one runaway script, written once per language. A missing entry
// means the language cannot express it.
type hostile struct {
	id, name string
	memory   bool // also run with the watchdog and the 256 MB job
	noSteps  bool // run without the step / alloc limit, to test cancel alone
	src      map[string]string
}

var hostiles = []hostile{
	{id: "trivial", name: "baseline: trivial script", src: map[string]string{
		eStar: `result = 1 + 1`,
		eGoja: `1 + 1`,
		eLua:  `return 1 + 1`,
		eRis:  `1 + 1`,
		eTen:  `result := 1 + 1`,
	}},
	{id: "loop", name: "infinite loop", src: map[string]string{
		eStar: "while True:\n    pass",
		eGoja: `for (;;) {}`,
		eLua:  `while true do end`,
		eRis:  `filter(range(9000000000000000000), x => false)`, // no loop keywords in Risor v2
		eTen:  `for {}`,
	}},
	{id: "loop-nosteps", name: "infinite loop, step limit off", noSteps: true, src: map[string]string{
		eStar: "while True:\n    pass",
		eGoja: `for (;;) {}`,
		eLua:  `while true do end`,
		eRis:  `filter(range(9000000000000000000), x => false)`,
		eTen:  `for {}`,
	}},
	{id: "list", name: "`[0] * 10**9`", memory: true, src: map[string]string{
		eStar: "x = [0] * 1000000000\nresult = len(x)",
		eGoja: `new Array(1e9).fill(0).length`,
		eLua:  `local t = {} for i = 1, 1e9 do t[i] = 0 end return #t`,
		eRis:  `len(list(range(1000000000)))`,
		eTen:  "x := range(0, 1000000000)\nresult := len(x)",
	}},
	{id: "str", name: "`\"x\" * 10**9`", memory: true, src: map[string]string{
		eStar: "x = \"x\" * 1000000000\nresult = len(x)",
		eGoja: `"x".repeat(1e9).length`,
		eLua:  `return #string.rep("x", 1e9)`,
		eRis:  `len("x".repeat(1000000000))`,
		eTen:  "x := bytes(1000000000)\nresult := len(x)", // no string repeat in Tengo
	}},
	{id: "double", name: "string doubling, 40 times", memory: true, src: map[string]string{
		eStar: "s = \"x\"\nfor i in range(40):\n    s = s + s\nresult = len(s)",
		eGoja: `let s = "x"; for (let i = 0; i < 40; i++) s = s + s; s.length`,
		eLua:  `local s = "x" for i = 1, 40 do s = s .. s end return #s`,
		eRis:  "function d(s, n) {\n  if (n == 0) { return len(s) }\n  return d(s + s, n - 1)\n}\nd(\"x\", 40)",
		eTen:  "s := \"x\"\nfor i := 0; i < 40; i++ { s = s + s }\nresult := len(s)",
	}},
	{id: "dict", name: "dict growth, 10**9 keys", memory: true, src: map[string]string{
		eStar: "d = {}\nfor i in range(1000000000):\n    d[\"k%d\" % i] = i\nresult = len(d)",
		eGoja: `const d = {}; for (let i = 0; i < 1e9; i++) d["k" + i] = i; Object.keys(d).length`,
		eLua:  `local d = {} for i = 1, 1e9 do d["k" .. i] = i end return 0`,
		eRis:  "let d = {}\nfilter(range(1000000000), i => { d[\"k\" + string(i)] = i; return false })\nlen(d)",
		eTen:  "d := {}\nfor i := 0; i < 1000000000; i++ { d[\"k\" + string(i)] = i }\nresult := len(d)",
	}},
	{id: "sort", name: "sort 2,000,000 items, 100 times", memory: true, src: map[string]string{
		eStar: "x = list(range(2000000))\nfor i in range(100):\n    y = sorted(x, reverse=True)\nresult = len(y)",
		eGoja: `const x = Array.from({length: 2e6}, (_, i) => i); let y; for (let k = 0; k < 100; k++) y = x.slice().sort(); y.length`,
		eLua:  `local x = {} for i = 1, 2e6 do x[i] = 2e6 - i end local y for k = 1, 100 do y = {} for i = 1, #x do y[i] = x[i] end table.sort(y) end return #y`,
		eRis:  "let x = reversed(list(range(2000000)))\nlet y = null\nfilter(range(100), k => { y = sorted(x); return false })\nlen(y)",
		// Tengo has no sort function without its stdlib.
	}},
	{id: "recursion", name: "deep recursion", src: map[string]string{
		eStar: "def f(n):\n    return 1 + f(n + 1)\nresult = f(0)",
		eGoja: `function f(n) { return 1 + f(n + 1) } f(0)`,
		eLua:  `local function f(n) return 1 + f(n + 1) end return f(0)`,
		eRis:  "function f(n) { return 1 + f(n + 1) }\nf(0)",
		eTen:  "f := undefined\nf = func(n) { return 1 + f(n + 1) }\nresult := f(0)",
	}},
	{id: "nest", name: "list nested 10**6 deep, then `str()`", memory: true, src: map[string]string{
		eStar: "x = []\nfor i in range(1000000):\n    x = [x]\nresult = len(str(x))",
		eGoja: `let x = []; for (let i = 0; i < 1e6; i++) x = [x]; JSON.stringify(x).length`,
		// gopher-lua has no builtin that walks nested tables.
		eRis: "let x = []\nfilter(range(1000000), i => { x = [x]; return false })\nlen(string(x))",
		eTen: "x := []\nfor i := 0; i < 1000000; i++ { x = [x] }\nresult := len(string(x))",
	}},
	{id: "bigint", name: "huge integer `2**10**7`", src: map[string]string{
		// Starlark has no ** and rejects shifts >= 512; squaring 23 times gives 2**(2**23), about 2**10**7.
		eStar: "x = 2\nfor i in range(23):\n    x = x * x\nresult = x",
		eGoja: `2n ** 10000000n`,
		eLua:  `return 2^10^7`,
		eRis:  `2 ** 10 ** 7`,
		eTen:  "x := 2\nfor i := 0; i < 23; i++ { x = x * x }\nresult := x",
	}},
	{id: "bigsquare", name: "integer squared 34 times", memory: true, src: map[string]string{
		eStar: "x = 2\nfor i in range(34):\n    x = x * x\nresult = x",
		eGoja: `let x = 2n; for (let i = 0; i < 34; i++) x = x * x; x`,
		eLua:  `local x = 2 for i = 1, 34 do x = x * x end return x`,
		eRis:  "function sq(x, n) {\n  if (n == 0) { return x }\n  return sq(x * x, n - 1)\n}\nsq(2, 34)",
		eTen:  "x := 2\nfor i := 0; i < 34; i++ { x = x * x }\nresult := x",
	}},
}

func hostileByID(id string) *hostile {
	for i := range hostiles {
		if hostiles[i].id == id {
			return &hostiles[i]
		}
	}
	return nil
}

// probe tries to reach something outside the sandbox. A probe passes when the
// script fails because the name, module or function does not exist.
type probe struct {
	name string
	src  map[string]string
}

const winIni = "C:/Windows/win.ini"

var probes = []probe{
	{"read a file", map[string]string{
		eStar: `result = open("` + winIni + `").read()`,
		eGoja: `require("fs").readFileSync("` + winIni + `")`,
		eLua:  `local f = io.open("` + winIni + `") return f:read("*a")`,
		eRis:  `os.read_file("` + winIni + `")`,
		eTen:  "os := import(\"os\")\nresult := os.read_file(\"" + winIni + "\")",
	}},
	{"network (127.0.0.1:9)", map[string]string{
		eStar: `result = http.get("http://127.0.0.1:9/")`,
		eGoja: `fetch("http://127.0.0.1:9/")`,
		eLua:  `local s = require("socket") return s.connect("127.0.0.1", 9)`,
		eRis:  `http.get("http://127.0.0.1:9/")`,
		eTen:  "http := import(\"http\")\nresult := http.get(\"http://127.0.0.1:9/\")",
	}},
	{"environment variable", map[string]string{
		eStar: `result = os.getenv("PATH")`,
		eGoja: `process.env.PATH`,
		eLua:  `return os.getenv("PATH")`,
		eRis:  `os.getenv("PATH")`,
		eTen:  "os := import(\"os\")\nresult := os.getenv(\"PATH\")",
	}},
	{"current time", map[string]string{
		eStar: `result = time.now()`,
		eGoja: `Date.now()`,
		eLua:  `return os.time()`,
		eRis:  `time.now()`,
		eTen:  "times := import(\"times\")\nresult := times.now()",
	}},
	{"random number", map[string]string{
		eStar: `result = random.random()`,
		eGoja: `Math.random()`,
		eLua:  `return math.random()`,
		eRis:  `rand.random()`,
		eTen:  "rand := import(\"rand\")\nresult := rand.int()",
	}},
	{"load code from a file", map[string]string{
		eStar: "load(\"" + winIni + "\", \"x\")\nresult = x",
		eGoja: `load("` + winIni + `")`,
		eLua:  `return loadfile("` + winIni + `")`,
		eRis:  `load("` + winIni + `")`,
		eTen:  "x := import(\"./win\")\nresult := x",
	}},
	{"import a module", map[string]string{
		eStar: "import os\nresult = os",
		eGoja: `require("os")`,
		eLua:  `return require("os")`,
		eRis:  "import os\nos",
		eTen:  "os := import(\"os\")\nresult := os",
	}},
}

// Determinism: 1,000 keys inserted in a scrambled order, then listed.
var detSrc = map[string]string{
	eStar: "d = {}\nfor i in range(1000):\n    d[\"k%d\" % ((i * 7919) % 1000)] = i\nresult = [k for k in d]",
	eGoja: `const d = {}; for (let i = 0; i < 1000; i++) d["k" + ((i * 7919) % 1000)] = i; Object.keys(d)`,
	eLua:  `local d = {} for i = 0, 999 do d["k" .. ((i * 7919) % 1000)] = i end local ks = {} for k in pairs(d) do ks[#ks+1] = k end return ks`,
	eRis:  "let d = {}\nfilter(range(1000), i => { d[\"k\" + string((i * 7919) % 1000)] = i; return false })\nkeys(d)",
	eTen:  "d := {}\nfor i := 0; i < 1000; i++ { d[\"k\" + string((i * 7919) % 1000)] = i }\nks := []\nfor k, _ in d { ks = append(ks, k) }\nresult := ks",
}

// The typical transform: group 10,000 rows by symbol, average the price, format a label.
var transformSrc = map[string]string{
	eStar: `rows = db.query("` + transformSQL + `", ["` + transformSince + `"])
groups = {}
for r in rows:
    g = groups.get(r["symbol"])
    if g == None:
        g = {"sum": 0.0, "n": 0, "last": ""}
        groups[r["symbol"]] = g
    g["sum"] += r["price"]
    g["n"] += 1
    if r["day"] > g["last"]:
        g["last"] = r["day"]
out = []
for sym in sorted(groups.keys()):
    g = groups[sym]
    avg = round(g["sum"] / g["n"], 2)
    out.append({"symbol": sym, "avg": avg, "days": g["n"],
                "label": "%s: %s until %s" % (sym, avg, format_date(add_days(g["last"], 1), "` + transformDateFmt + `"))})
result = out`,
	eGoja: `const rows = db.query("` + transformSQL + `", ["` + transformSince + `"]);
const groups = {};
for (const r of rows) {
  let g = groups[r.symbol];
  if (g === undefined) { g = {sum: 0, n: 0, last: ""}; groups[r.symbol] = g; }
  g.sum += r.price; g.n++;
  if (r.day > g.last) g.last = r.day;
}
const out = [];
for (const sym of Object.keys(groups).sort()) {
  const g = groups[sym];
  const avg = round(g.sum / g.n, 2);
  out.push({symbol: sym, avg: avg, days: g.n, label: sym + ": " + avg + " until " + format_date(add_days(g.last, 1), "` + transformDateFmt + `")});
}
out`,
	eLua: `local rows = db.query("` + transformSQL + `", {"` + transformSince + `"})
local groups, names = {}, {}
for _, r in ipairs(rows) do
  local g = groups[r.symbol]
  if g == nil then g = {sum = 0, n = 0, last = ""}; groups[r.symbol] = g; names[#names + 1] = r.symbol end
  g.sum = g.sum + r.price; g.n = g.n + 1
  if r.day > g.last then g.last = r.day end
end
table.sort(names)
local out = {}
for _, sym in ipairs(names) do
  local g = groups[sym]
  local avg = round(g.sum / g.n, 2)
  out[#out + 1] = {symbol = sym, avg = avg, days = g.n, label = sym .. ": " .. tostring(avg) .. " until " .. format_date(add_days(g.last, 1), "` + transformDateFmt + `")}
end
return out`,
	eRis: `let rows = db.query("` + transformSQL + `", ["` + transformSince + `"])
let groups = {}
rows.each(r => {
  let g = groups.get(r.symbol, null)
  if (g == null) {
    g = {sum: 0.0, n: 0, last: ""}
    groups[r.symbol] = g
  }
  g.sum = g.sum + r.price
  g.n = g.n + 1
  if (r.day > g.last) { g.last = r.day }
})
sorted(keys(groups)).map(sym => {
  let g = groups[sym]
  let avg = round(g.sum / g.n, 2)
  return {symbol: sym, avg: avg, days: g.n, label: sym + ": " + string(avg) + " until " + format_date(add_days(g.last, 1), "` + transformDateFmt + `")}
})`,
	// Tengo has no sort without its stdlib; the query already orders by symbol.
	eTen: `rows := db.query("` + transformSQL + `", ["` + transformSince + `"])
groups := {}
names := []
for r in rows {
  g := groups[r.symbol]
  if is_undefined(g) {
    g = {sum: 0.0, n: 0, last: ""}
    groups[r.symbol] = g
    names = append(names, r.symbol)
  }
  g.sum += r.price
  g.n += 1
  if r.day > g.last { g.last = r.day }
}
out := []
for sym in names {
  g := groups[sym]
  avg := round(g.sum / g.n, 2)
  out = append(out, {symbol: sym, avg: avg, days: g.n, label: sym + ": " + string(avg) + " until " + format_date(add_days(g.last, 1), "` + transformDateFmt + `")})
}
result := out`,
}
