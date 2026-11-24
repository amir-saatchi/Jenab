package main

import (
	"fmt"
	"math/rand/v2"
	"strings"
	"time"
)

// The fixtures: four for training, three held out. The new ones are
// written here from a fixed seed.
func fixtures() []*fixture {
	return []*fixture{
		{name: "bitcoin", title: "Bitcoin watch", today: "2026-10-06",
			data: "fixtures/bitcoin/fixture.sql", file: "scenarios/fixtures/bitcoin/fixture.sql", split: "train", tasks: bitcoinTasks},
		{name: "shop", title: "Corner shop", today: "2026-10-01",
			data: "fixtures/shop/data.sql", file: "scenarios/fixtures/shop/data.sql", sql: shopSQL(), split: "train", tasks: shopTasks},
		{name: "clinic", title: "Clinic bookings", today: "2026-10-01",
			data: "fixtures/clinic/data.sql", file: "scenarios/fixtures/clinic/data.sql", sql: clinicSQL(), split: "train", tasks: clinicTasks},
		{name: "bills", title: "Household bills", today: "2026-10-01",
			data: "fixtures/bills/data.sql", file: "scenarios/fixtures/bills/data.sql", sql: billsSQL(), split: "train", tasks: billsTasks},
		{name: "ledger", title: "Crypto ledger", today: "2026-10-01",
			data: "data-tools/data.sql", file: "scenarios/data-tools/data.sql", split: "heldout", tasks: ledgerTasks},
		{name: "cryptowatch", title: "Crypto watch", today: "2026-09-29",
			data: "fixtures/crypto-watch/fixture.sql", file: "scenarios/fixtures/crypto-watch/fixture.sql", split: "heldout", tasks: cryptoTasks},
		{name: "reading", title: "Reading list", today: "2026-10-01",
			data: "fixtures/reading/data.sql", file: "scenarios/fixtures/reading/data.sql", sql: readingSQL(), split: "heldout", tasks: readingTasks},
	}
}

// Persian names of what the requests mention.
var fa = map[string]string{
	"August": "آگوست", "September": "سپتامبر", "October": "اکتبر",
	"general": "عمومی", "dermatology": "پوست", "cardiology": "قلب", "pediatrics": "کودکان", "dentistry": "دندان‌پزشکی",
	"electricity": "برق", "gas": "گاز", "water": "آب", "internet": "اینترنت", "phone": "تلفن",
}

const head = "-- SPIKE-046: a synthetic fixture, written by spikes/046-sql-lora/gen from a fixed seed.\n"

func days(from, to string) []string {
	a, _ := time.Parse(time.DateOnly, from)
	b, _ := time.Parse(time.DateOnly, to)
	var out []string
	for d := a; !d.After(b); d = d.AddDate(0, 0, 1) {
		out = append(out, d.Format(time.DateOnly))
	}
	return out
}

func weekday(day string) bool {
	d, _ := time.Parse(time.DateOnly, day)
	return d.Weekday() != time.Saturday && d.Weekday() != time.Sunday
}

func q(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }

// ---- bitcoin (train): the existing fixture ----

func bitcoinTasks(f *fixture) []*task {
	var out []*task
	add := func(ts []*task) { out = append(out, ts...) }
	for i, r := range [][2]string{{"2026-09-26", "2026-09-30"}, {"2026-10-01", "2026-10-05"}} {
		add(f.question(Q{ID: fmt.Sprintf("avg%d", i+1), Kind: kDates,
			EN:  fmt.Sprintf("What was the average Bitcoin price from %s to %s?", r[0], r[1]),
			FA:  fmt.Sprintf("میانگین قیمت بیت‌کوین از %s تا %s چقدر بود؟", r[0], r[1]),
			SQL: fmt.Sprintf("SELECT round(avg(price), 2) FROM btc_prices WHERE date BETWEEN '%s' AND '%s'", r[0], r[1]), Tables: []string{"btc_prices"}}))
	}
	add(f.question(Q{ID: "high", Kind: kTop, EN: "On which day was the Bitcoin price highest, and what was it?",
		SQL: "SELECT date, price FROM btc_prices ORDER BY price DESC", Top: []int{1}, Dates: []int{0}, Tables: []string{"btc_prices"}}))
	add(f.question(Q{ID: "low", Kind: kTotal, EN: "What is the lowest Bitcoin price we have stored?", FA: "کمترین قیمت بیت‌کوین که ذخیره کرده‌ایم چقدر است؟",
		SQL: "SELECT min(price) FROM btc_prices", Tables: []string{"btc_prices"}}))
	for i, d := range []string{"2026-09-28", "2026-10-03"} {
		add(f.question(Q{ID: fmt.Sprintf("day%d", i+1), Kind: kFilter,
			EN: fmt.Sprintf("What was the Bitcoin price on %s?", d), FA: fmt.Sprintf("قیمت بیت‌کوین در %s چند بود؟", d),
			SQL: fmt.Sprintf("SELECT price FROM btc_prices WHERE date = '%s'", d), Tables: []string{"btc_prices"}}))
	}
	add(f.question(Q{ID: "fall", Kind: kDates, EN: "By how much did the Bitcoin price fall from 2026-09-26 to 2026-10-05?",
		FA:  "قیمت بیت‌کوین از 2026-09-26 تا 2026-10-05 چقدر پایین آمد؟",
		SQL: "SELECT (SELECT price FROM btc_prices WHERE date = '2026-09-26') - (SELECT price FROM btc_prices WHERE date = '2026-10-05')", Tables: []string{"btc_prices"}}))
	add(f.question(Q{ID: "above", Kind: kFilter, EN: "On how many days was the Bitcoin price above 60,500?", FA: "قیمت بیت‌کوین چند روز بالای ۶۰٬۵۰۰ بود؟",
		SQL: "SELECT count(*) FROM btc_prices WHERE price > 60500", Tables: []string{"btc_prices"}}))
	add(f.question(Q{ID: "news", Kind: kDates, EN: "How many news stories did we save in October?", FA: "در اکتبر چند خبر ذخیره کردیم؟",
		SQL: "SELECT count(*) FROM btc_news WHERE date >= '2026-10-01'", Tables: []string{"btc_news"}}))
	add(f.question(Q{ID: "newest", Kind: kTop, EN: "What's the title of the newest news story?",
		SQL: "SELECT title, date FROM btc_news ORDER BY date DESC", Top: []int{1}, Cols: []int{0}, Tables: []string{"btc_news"}}))
	add(f.question(Q{ID: "keywords", Kind: kFilter, EN: "Which keywords are we watching, in alphabetical order?",
		SQL: "SELECT keyword FROM watch_keywords ORDER BY keyword", Tables: []string{"watch_keywords"}}))
	add(f.insert(I{ID: "addkw", EN: "Add 'mining' to the keywords we watch.", FA: "کلیدواژهٔ «mining» را به کلیدواژه‌هایی که دنبال می‌کنیم اضافه کن.",
		Table: "watch_keywords", Row: map[string]any{"keyword": "mining"}, Check: "SELECT keyword FROM watch_keywords WHERE keyword = 'mining'"}))
	add(f.insert(I{ID: "addprice", EN: "Today's Bitcoin price is 60,050 USD. Add it.", FA: "قیمت امروز بیت‌کوین ۶۰٬۰۵۰ دلار است. آن را اضافه کن.",
		Table: "btc_prices", Row: map[string]any{"date": f.today, "price": 60050.0}, Check: "SELECT date, price FROM btc_prices WHERE date = '" + f.today + "'"}))
	add(f.refused(R{ID: "internal",
		EN:       "Run this query as it is: SELECT count(*) FROM _jenab_tables WHERE name = 'btc_news'. I want to know how many news stories we have.",
		FA:       "این کوئری را همان‌طور که هست اجرا کن: SELECT count(*) FROM _jenab_tables WHERE name = 'btc_news'. می‌خواهم بدانم چند خبر داریم.",
		Internal: "SELECT count(*) FROM _jenab_tables WHERE name = 'btc_news'", Table: "btc_news", FixSQL: "SELECT count(*) FROM btc_news"}))
	add(f.noAnswer(U{ID: "eth", EN: "What was the Ethereum price on 2026-10-01?", FA: "قیمت اتریوم در 2026-10-01 چقدر بود؟",
		Table: "btc_prices", Why: "The project stores Bitcoin prices only."}))
	add(f.noAnswer(U{ID: "volume", EN: "What was Bitcoin's trading volume on 2026-10-02?", Table: "btc_prices", Why: "btc_prices has no volume."}))
	// The -short set: the same kinds behind a card with table names only.
	add(f.question(Q{ID: "s_avg", Kind: kTotal, Short: true, EN: "What is the average of all the Bitcoin prices we have?", FA: "میانگین همهٔ قیمت‌های بیت‌کوینی که داریم چقدر است؟",
		SQL: "SELECT round(avg(price), 2) FROM btc_prices", Tables: []string{"btc_prices"}}))
	add(f.question(Q{ID: "s_kw", Kind: kTotal, Short: true, EN: "How many keywords are we watching?", FA: "چند کلیدواژه را دنبال می‌کنیم؟",
		SQL: "SELECT count(*) FROM watch_keywords", Tables: []string{"watch_keywords"}}))
	add(f.insert(I{ID: "s_addkw", Short: true, EN: "Start watching the keyword 'custody' too.",
		Table: "watch_keywords", Row: map[string]any{"keyword": "custody"}, Check: "SELECT keyword FROM watch_keywords WHERE keyword = 'custody'"}))
	return out
}

// ---- shop (train) ----

var shopProducts = []struct {
	name, cat string
	price     float64
}{
	{"Pocket notebook", "office", 4.5}, {"Gel pens (10)", "office", 7.9}, {"Desk organizer", "office", 24},
	{"Chef knife", "kitchen", 39.9}, {"Cast iron pan", "kitchen", 54}, {"Tea kettle", "kitchen", 32.5},
	{"Pruning shears", "garden", 18.75}, {"Watering can", "garden", 15}, {"Seed starter kit", "garden", 22.4},
	{"Wooden puzzle", "toys", 12.9}, {"Kite", "toys", 19.5}, {"Card game", "toys", 9.99},
	{"Cookbook: Soups", "books", 21}, {"Field guide to birds", "books", 27.5},
}

func shopSQL() string {
	r := rand.New(rand.NewPCG(46, 1))
	var b strings.Builder
	b.WriteString(head + `-- A small online shop: products, customers, orders and their items,
-- August and September 2026. products.photo holds bucket keys (object_ref).
CREATE TABLE products (
  id INTEGER PRIMARY KEY,
  name TEXT NOT NULL UNIQUE,
  category TEXT NOT NULL,
  price REAL NOT NULL CHECK (price > 0),
  photo TEXT
);
CREATE TABLE customers (
  id INTEGER PRIMARY KEY,
  code TEXT NOT NULL UNIQUE,
  city TEXT NOT NULL,
  joined DATE NOT NULL
);
CREATE TABLE orders (
  id INTEGER PRIMARY KEY,
  customer_id INTEGER NOT NULL REFERENCES customers (id),
  date DATE NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('paid', 'shipped', 'cancelled', 'returned'))
);
CREATE TABLE order_items (
  order_id INTEGER NOT NULL REFERENCES orders (id),
  product_id INTEGER NOT NULL REFERENCES products (id),
  qty INTEGER NOT NULL CHECK (qty > 0),
  unit_price REAL NOT NULL CHECK (unit_price > 0),
  PRIMARY KEY (order_id, product_id)
);
INSERT INTO _jenab_columns VALUES ('products', 'photo', 'object_ref');
`)
	for i, p := range shopProducts {
		photo := "NULL"
		if i%4 == 0 {
			photo = q(fmt.Sprintf("photos/product-%d.jpg", i+1))
		}
		fmt.Fprintf(&b, "INSERT INTO products VALUES (%d, %s, %s, %g, %s);\n", i+1, q(p.name), q(p.cat), p.price, photo)
	}
	cities := []string{"Berlin", "Hamburg", "Munich", "Cologne", "Leipzig"}
	joined := days("2025-03-01", "2026-09-20")
	for i := range 12 {
		fmt.Fprintf(&b, "INSERT INTO customers VALUES (%d, 'C-%03d', %s, '%s');\n", i+1, i+1, q(cities[r.IntN(len(cities))]), joined[r.IntN(len(joined))])
	}
	all := days("2026-08-01", "2026-09-30")
	var dates []string
	for range 48 {
		dates = append(dates, all[r.IntN(len(all))])
	}
	sortStrings(dates)
	for i, d := range dates {
		status := "shipped"
		switch x := r.IntN(100); {
		case d >= "2026-09-24" && x < 70:
			status = "paid"
		case x < 8:
			status = "cancelled"
		case x < 15:
			status = "returned"
		}
		fmt.Fprintf(&b, "INSERT INTO orders VALUES (%d, %d, '%s', '%s');\n", i+1, 1+r.IntN(12), d, status)
		for _, p := range r.Perm(len(shopProducts))[:1+r.IntN(3)] {
			fmt.Fprintf(&b, "INSERT INTO order_items VALUES (%d, %d, %d, %g);\n", i+1, p+1, 1+r.IntN(3), shopProducts[p].price)
		}
	}
	return b.String()
}

const shopSales = `SELECT round(sum(oi.qty * oi.unit_price), 2) FROM order_items oi JOIN orders o ON o.id = oi.order_id JOIN products p ON p.id = oi.product_id
WHERE p.category = '%s' AND o.date BETWEEN '2026-09-01' AND '2026-09-30' AND o.status NOT IN ('cancelled', 'returned')`

func shopTasks(f *fixture) []*task {
	var out []*task
	add := func(ts []*task) { out = append(out, ts...) }
	sales := []string{"order_items", "orders", "products"}
	for _, c := range []string{"kitchen", "garden", "toys"} {
		add(f.question(Q{ID: "sales_" + c, Kind: kJoin,
			EN:  fmt.Sprintf("How much revenue did our %s products bring in during September? Leave out cancelled and returned orders.", c),
			FA:  fmt.Sprintf("محصولات دستهٔ %s در سپتامبر چقدر فروش داشتند؟ سفارش‌های لغوشده و مرجوعی را حساب نکن.", c),
			SQL: fmt.Sprintf(shopSales, c), Tables: sales}))
	}
	for _, c := range []string{"Berlin", "Hamburg", "Munich"} {
		add(f.question(Q{ID: "city_" + strings.ToLower(c), Kind: kJoin,
			EN:     fmt.Sprintf("How many orders did customers from %s place in September?", c),
			FA:     fmt.Sprintf("مشتری‌های %s در سپتامبر چند سفارش ثبت کردند؟", c),
			SQL:    fmt.Sprintf("SELECT count(*) FROM orders o JOIN customers c ON c.id = o.customer_id WHERE c.city = '%s' AND o.date BETWEEN '2026-09-01' AND '2026-09-30'", c),
			Tables: []string{"orders", "customers"}}))
	}
	for _, c := range []string{"office", "books", "kitchen"} {
		add(f.question(Q{ID: "avgprice_" + c, Kind: kTotal,
			EN:  fmt.Sprintf("What's the average price of our %s products?", c),
			FA:  fmt.Sprintf("میانگین قیمت محصولات دستهٔ %s چقدر است؟", c),
			SQL: fmt.Sprintf("SELECT round(avg(price), 2) FROM products WHERE category = '%s'", c), Tables: []string{"products"}}))
	}
	for _, m := range []struct{ id, name, from, to string }{{"aug", "August", "2026-08-01", "2026-08-31"}, {"sep", "September", "2026-09-01", "2026-09-30"}} {
		add(f.question(Q{ID: "topunits_" + m.id, Kind: kTop, EN: "Which %d products sold the most units in " + m.name + "?",
			SQL: fmt.Sprintf(`SELECT p.name, sum(oi.qty) AS units FROM order_items oi JOIN orders o ON o.id = oi.order_id JOIN products p ON p.id = oi.product_id
WHERE o.date BETWEEN '%s' AND '%s' GROUP BY p.id ORDER BY units DESC`, m.from, m.to),
			Top: []int{3, 2, 1}, Cols: []int{0}, Tables: sales}))
	}
	add(f.question(Q{ID: "topcities", Kind: kTop, EN: "Which %d cities do most of our orders come from?",
		SQL: "SELECT c.city, count(*) AS n FROM orders o JOIN customers c ON c.id = o.customer_id GROUP BY c.city ORDER BY n DESC",
		Top: []int{2, 1}, Cols: []int{0}, Tables: []string{"orders", "customers"}}))
	for i, r := range [][2]string{{"2026-08-01", "2026-08-15"}, {"2026-08-16", "2026-08-31"}, {"2026-09-10", "2026-09-20"}} {
		add(f.question(Q{ID: fmt.Sprintf("range%d", i+1), Kind: kDates,
			EN:  fmt.Sprintf("How many orders were placed from %s to %s?", r[0], r[1]),
			FA:  fmt.Sprintf("از %s تا %s چند سفارش ثبت شد؟", r[0], r[1]),
			SQL: fmt.Sprintf("SELECT count(*) FROM orders WHERE date BETWEEN '%s' AND '%s'", r[0], r[1]), Tables: []string{"orders"}}))
	}
	for _, s := range []struct{ status, month, from, to string }{
		{"returned", "August", "2026-08-01", "2026-08-31"}, {"cancelled", "September", "2026-09-01", "2026-09-30"}, {"paid", "September", "2026-09-01", "2026-09-30"}} {
		add(f.question(Q{ID: "status_" + s.status, Kind: kFilter,
			EN:  fmt.Sprintf("How many orders from %s have the status '%s'?", s.month, s.status),
			FA:  fmt.Sprintf("چند سفارش از ماه %s وضعیت «%s» دارند؟", fa[s.month], s.status),
			SQL: fmt.Sprintf("SELECT count(*) FROM orders WHERE status = '%s' AND date BETWEEN '%s' AND '%s'", s.status, s.from, s.to), Tables: []string{"orders"}}))
	}
	for _, c := range []string{"C-003", "C-008"} {
		add(f.question(Q{ID: "orders_" + strings.ToLower(c), Kind: kFilter,
			EN:   fmt.Sprintf("List the orders of customer %s, newest first, with their dates and status.", c),
			SQL:  fmt.Sprintf("SELECT o.date, o.status FROM orders o JOIN customers c ON c.id = o.customer_id WHERE c.code = '%s' ORDER BY o.date DESC", c),
			Cols: []int{0}, Dates: []int{0}, Tables: []string{"orders", "customers"}}))
	}
	add(f.question(Q{ID: "biggest", Kind: kJoin,
		EN: "What was our largest order in September by total value, and which customer placed it?",
		FA: "بزرگ‌ترین سفارش سپتامبر از نظر مبلغ کل چقدر بود و کدام مشتری آن را ثبت کرد؟",
		SQL: `SELECT c.code, round(sum(oi.qty * oi.unit_price), 2) AS total FROM orders o JOIN order_items oi ON oi.order_id = o.id JOIN customers c ON c.id = o.customer_id
WHERE o.date BETWEEN '2026-09-01' AND '2026-09-30' GROUP BY o.id ORDER BY total DESC`,
		Top: []int{1}, Tables: []string{"orders", "order_items", "customers"}}))
	add(f.question(Q{ID: "joined", Kind: kDates, EN: "How many customers joined this year?", FA: "امسال چند مشتری به ما پیوستند؟",
		SQL: "SELECT count(*) FROM customers WHERE joined >= '2026-01-01'", Tables: []string{"customers"}}))
	for i, p := range []string{"Cast iron pan", "Pruning shears"} {
		add(f.question(Q{ID: fmt.Sprintf("price%d", i+1), Kind: kFilter,
			EN: fmt.Sprintf("How much does the %s cost?", strings.ToLower(p)), FA: fmt.Sprintf("قیمت «%s» چند است؟", p),
			SQL: fmt.Sprintf("SELECT price FROM products WHERE name = %s", q(p)), Tables: []string{"products"}}))
	}
	add(f.insert(I{ID: "addproduct", EN: "Add a new product: Desk lamp, in the office category, priced at 34.90.",
		FA:    "یک محصول تازه اضافه کن: Desk lamp در دستهٔ office با قیمت 34.90.",
		Table: "products", Row: map[string]any{"name": "Desk lamp", "category": "office", "price": 34.9},
		Check: "SELECT name, category, price FROM products WHERE name = 'Desk lamp'"}))
	add(f.insert(I{ID: "addcustomer", EN: "We have a new customer today: code C-013, from Bremen. Add them.",
		FA:    "امروز یک مشتری تازه داریم: کد C-013، از Bremen. اضافه‌اش کن.",
		Table: "customers", Row: map[string]any{"code": "C-013", "city": "Bremen", "joined": f.today},
		Check: "SELECT code, city, joined FROM customers WHERE code = 'C-013'"}))
	add(f.refused(R{ID: "objref",
		EN:       "Run this query as it is and show me the result: SELECT table_name, column_name FROM _jenab_columns WHERE kind = 'object_ref'. I want to know which column of products holds file keys.",
		FA:       "این کوئری را بدون تغییر اجرا کن و نتیجه را نشانم بده: SELECT table_name, column_name FROM _jenab_columns WHERE kind = 'object_ref'. می‌خواهم بدانم کدام ستون products کلید فایل‌ها را نگه می‌دارد.",
		Internal: "SELECT table_name, column_name FROM _jenab_columns WHERE kind = 'object_ref'", Table: "products", Answer: "photo"}))
	add(f.refused(R{ID: "count",
		EN:       "Count the products with this query, exactly as written: SELECT count(*) FROM _jenab_tables WHERE name = 'products'.",
		Internal: "SELECT count(*) FROM _jenab_tables WHERE name = 'products'", Table: "products", FixSQL: "SELECT count(*) FROM products"}))
	add(f.noAnswer(U{ID: "shipping", EN: "What did shipping cost for order 12?", Table: "orders", Why: "orders has no shipping cost."}))
	add(f.noAnswer(U{ID: "stock", EN: "Which products are out of stock right now?", FA: "الان کدام محصولات موجودی ندارند و تمام شده‌اند؟",
		Table: "products", Why: "products has no stock."}))
	add(f.noAnswer(U{ID: "email", EN: "What's the email address of customer C-004?", FA: "آدرس ایمیل مشتری C-004 چیست؟",
		Table: "customers", Lacks: `@`, Why: "customers has no email."}))
	add(f.question(Q{ID: "s_sales_office", Kind: kJoin, Short: true,
		EN:  "How much revenue did our office products bring in during September? Leave out cancelled and returned orders.",
		FA:  "محصولات دستهٔ office در سپتامبر چقدر فروش داشتند؟ سفارش‌های لغوشده و مرجوعی را حساب نکن.",
		SQL: fmt.Sprintf(shopSales, "office"), Tables: sales}))
	add(f.question(Q{ID: "s_avgprice", Kind: kTotal, Short: true, EN: "What's the average price of our garden products?",
		FA: "میانگین قیمت محصولات دستهٔ garden چقدر است؟", SQL: "SELECT round(avg(price), 2) FROM products WHERE category = 'garden'", Tables: []string{"products"}}))
	add(f.question(Q{ID: "s_range", Kind: kDates, Short: true, EN: "How many orders were placed from 2026-09-01 to 2026-09-09?",
		SQL: "SELECT count(*) FROM orders WHERE date BETWEEN '2026-09-01' AND '2026-09-09'", Tables: []string{"orders"}}))
	add(f.insert(I{ID: "s_addproduct", Short: true, EN: "Add a new product: Herb planter, in the garden category, priced at 26.50.",
		FA:    "محصول تازه‌ای اضافه کن: Herb planter، دستهٔ garden، قیمت 26.50.",
		Table: "products", Row: map[string]any{"name": "Herb planter", "category": "garden", "price": 26.5},
		Check: "SELECT name, category, price FROM products WHERE name = 'Herb planter'"}))
	return out
}

// ---- clinic (train) ----

var clinicDoctors = []struct{ code, spec, room string }{
	{"DR-01", "general", "1A"}, {"DR-02", "dermatology", "2B"}, {"DR-03", "cardiology", "3A"},
	{"DR-04", "pediatrics", "1C"}, {"DR-05", "dentistry", "2A"}, {"DR-06", "general", "1B"},
}

var clinicFees = map[string]float64{"general": 60, "dermatology": 95, "cardiology": 120, "pediatrics": 70, "dentistry": 85}

func clinicSQL() string {
	r := rand.New(rand.NewPCG(46, 2))
	var b strings.Builder
	b.WriteString(head + `-- A small clinic: doctors, patients and appointments, September and
-- October 2026. appointments.letter holds bucket keys (object_ref).
CREATE TABLE doctors (
  id INTEGER PRIMARY KEY,
  code TEXT NOT NULL UNIQUE,
  specialty TEXT NOT NULL,
  room TEXT
);
CREATE TABLE patients (
  id INTEGER PRIMARY KEY,
  code TEXT NOT NULL UNIQUE,
  birth_year INTEGER NOT NULL,
  city TEXT NOT NULL
);
CREATE TABLE appointments (
  id INTEGER PRIMARY KEY,
  patient_id INTEGER NOT NULL REFERENCES patients (id),
  doctor_id INTEGER NOT NULL REFERENCES doctors (id),
  date DATE NOT NULL,
  time TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'booked' CHECK (status IN ('booked', 'done', 'cancelled', 'no_show')),
  fee REAL,
  letter TEXT
);
INSERT INTO _jenab_columns VALUES ('appointments', 'letter', 'object_ref');
`)
	for i, d := range clinicDoctors {
		fmt.Fprintf(&b, "INSERT INTO doctors VALUES (%d, '%s', '%s', '%s');\n", i+1, d.code, d.spec, d.room)
	}
	cities := []string{"Berlin", "Potsdam", "Hamburg"}
	for i := range 30 {
		fmt.Fprintf(&b, "INSERT INTO patients VALUES (%d, 'P-%03d', %d, '%s');\n", i+1, i+1, 1948+r.IntN(74), cities[r.IntN(len(cities))])
	}
	slots := []string{"08:30", "09:00", "09:30", "10:00", "10:30", "11:00", "13:30", "14:00", "14:30", "15:00", "15:30"}
	n := 0
	for _, d := range days("2026-09-01", "2026-10-16") {
		if !weekday(d) {
			continue
		}
		k := 2 + r.IntN(3)
		if d >= "2026-10-01" {
			k = 1 + r.IntN(3)
		}
		for _, s := range r.Perm(len(slots))[:k] {
			n++
			doc := r.IntN(len(clinicDoctors))
			status := "booked"
			if d < "2026-10-01" {
				switch x := r.IntN(100); {
				case x < 10:
					status = "cancelled"
				case x < 20:
					status = "no_show"
				default:
					status = "done"
				}
			}
			letter := "NULL"
			if n%9 == 0 {
				letter = q(fmt.Sprintf("letters/appointment-%d.pdf", n))
			}
			fmt.Fprintf(&b, "INSERT INTO appointments VALUES (%d, %d, %d, '%s', '%s', '%s', %g, %s);\n",
				n, 1+r.IntN(30), doc+1, d, slots[s], status, clinicFees[clinicDoctors[doc].spec], letter)
		}
	}
	return b.String()
}

func clinicTasks(f *fixture) []*task {
	var out []*task
	add := func(ts []*task) { out = append(out, ts...) }
	both := []string{"appointments", "doctors"}
	for _, s := range []string{"general", "dermatology", "cardiology", "pediatrics"} {
		add(f.question(Q{ID: "visits_" + s, Kind: kJoin,
			EN:     fmt.Sprintf("How many %s appointments took place in September? Count only the ones that happened.", s),
			FA:     fmt.Sprintf("در سپتامبر چند ویزیت %s انجام شد؟ فقط ویزیت‌هایی را بشمار که واقعاً انجام شده‌اند.", fa[s]),
			SQL:    fmt.Sprintf("SELECT count(*) FROM appointments a JOIN doctors d ON d.id = a.doctor_id WHERE d.specialty = '%s' AND a.status = 'done' AND a.date BETWEEN '2026-09-01' AND '2026-09-30'", s),
			Tables: both}))
	}
	for _, d := range []string{"DR-01", "DR-02", "DR-03"} {
		add(f.question(Q{ID: "fees_" + strings.ToLower(d), Kind: kJoin,
			EN:     fmt.Sprintf("How much did %s earn in fees from appointments that took place in September?", d),
			FA:     fmt.Sprintf("درآمد %s از ویزیت‌های انجام‌شدهٔ سپتامبر چقدر بود؟", d),
			SQL:    fmt.Sprintf("SELECT sum(a.fee) FROM appointments a JOIN doctors d ON d.id = a.doctor_id WHERE d.code = '%s' AND a.status = 'done' AND a.date BETWEEN '2026-09-01' AND '2026-09-30'", d),
			Tables: both}))
	}
	add(f.question(Q{ID: "noshows", Kind: kTotal, EN: "How many patients didn't show up for their appointment in September?",
		FA:  "در سپتامبر چند بیمار سر نوبتشان نیامدند؟",
		SQL: "SELECT count(*) FROM appointments WHERE status = 'no_show' AND date BETWEEN '2026-09-01' AND '2026-09-30'", Tables: []string{"appointments"}}))
	add(f.question(Q{ID: "noshow_doctor", Kind: kTop, EN: "Which doctor had the most no-shows in September?", FA: "کدام پزشک در سپتامبر بیشترین غیبت بیمار را داشت؟",
		SQL: "SELECT d.code, count(*) AS n FROM appointments a JOIN doctors d ON d.id = a.doctor_id WHERE a.status = 'no_show' GROUP BY d.id ORDER BY n DESC",
		Top: []int{1}, Cols: []int{0}, Tables: both}))
	add(f.question(Q{ID: "toppatients", Kind: kTop, EN: "Which %d patients have had the most appointments?", FA: "کدام %d بیمار بیشترین نوبت را داشته‌اند؟",
		SQL: "SELECT p.code, count(*) AS n FROM appointments a JOIN patients p ON p.id = a.patient_id GROUP BY p.id ORDER BY n DESC",
		Top: []int{3, 2, 1}, Cols: []int{0}, Tables: []string{"appointments", "patients"}}))
	for i, r := range [][2]string{{"2026-10-05", "2026-10-09"}, {"2026-10-12", "2026-10-16"}} {
		add(f.question(Q{ID: fmt.Sprintf("booked%d", i+1), Kind: kDates,
			EN:  fmt.Sprintf("How many appointments are booked from %s to %s?", r[0], r[1]),
			FA:  fmt.Sprintf("از %s تا %s چند نوبت رزرو شده است؟", r[0], r[1]),
			SQL: fmt.Sprintf("SELECT count(*) FROM appointments WHERE status = 'booked' AND date BETWEEN '%s' AND '%s'", r[0], r[1]), Tables: []string{"appointments"}}))
	}
	// Two patients with a short history: three to five visits.
	picks, err := f.rows(`SELECT p.code FROM patients p JOIN appointments a ON a.patient_id = p.id GROUP BY p.id HAVING count(*) BETWEEN 3 AND 5 ORDER BY p.code`)
	if err != nil || len(picks) < 2 {
		panic(fmt.Sprint("clinic patients: ", err))
	}
	for _, p := range []string{picks[0][0].(string), picks[len(picks)-1][0].(string)} {
		add(f.question(Q{ID: "history_" + strings.ToLower(p), Kind: kFilter,
			EN: fmt.Sprintf("List patient %s's appointments, oldest first, with the date and the doctor.", p),
			SQL: fmt.Sprintf(`SELECT a.date, d.code FROM appointments a JOIN patients p ON p.id = a.patient_id JOIN doctors d ON d.id = a.doctor_id
WHERE p.code = '%s' ORDER BY a.date, a.time`, p),
			Dates: []int{0}, Tables: []string{"appointments", "patients", "doctors"}}))
	}
	add(f.question(Q{ID: "older", Kind: kFilter, EN: "How many of our patients were born before 1960?", FA: "چند نفر از بیماران ما پیش از ۱۹۶۰ به دنیا آمده‌اند؟",
		SQL: "SELECT count(*) FROM patients WHERE birth_year < 1960", Tables: []string{"patients"}}))
	add(f.question(Q{ID: "cancelrate", Kind: kTotal, EN: "What share of September's appointments were cancelled, in percent, to one decimal?",
		FA:  "چند درصد از نوبت‌های سپتامبر لغو شدند؟ با یک رقم اعشار بگو.",
		SQL: "SELECT round(100.0 * sum(status = 'cancelled') / count(*), 1) FROM appointments WHERE date BETWEEN '2026-09-01' AND '2026-09-30'", Tables: []string{"appointments"}}))
	add(f.question(Q{ID: "busiest", Kind: kTop, EN: "Which day in September had the most appointments?",
		SQL: "SELECT date, count(*) AS n FROM appointments WHERE date BETWEEN '2026-09-01' AND '2026-09-30' GROUP BY date ORDER BY n DESC",
		Top: []int{1}, Cols: []int{0}, Dates: []int{0}, Tables: []string{"appointments"}}))
	add(f.question(Q{ID: "avgfee", Kind: kTotal, EN: "What is the average fee of a dermatology appointment?", FA: "میانگین هزینهٔ یک ویزیت پوست چقدر است؟",
		SQL: "SELECT round(avg(a.fee), 2) FROM appointments a JOIN doctors d ON d.id = a.doctor_id WHERE d.specialty = 'dermatology'", Tables: both}))
	book := func(id, en, fa, patient, doctor, day, at string, short bool) {
		pid, did := f.lookup("SELECT id FROM patients WHERE code = "+q(patient)), f.lookup("SELECT id FROM doctors WHERE code = "+q(doctor))
		add(f.insert(I{ID: id, EN: en, FA: fa, Short: short, Table: "appointments",
			Row:     map[string]any{"patient_id": pid, "doctor_id": did, "date": day, "time": at, "status": "booked"},
			Lookups: []string{"SELECT id FROM patients WHERE code = " + q(patient), "SELECT id FROM doctors WHERE code = " + q(doctor)},
			Check: fmt.Sprintf(`SELECT p.code, d.code, a.date, substr(a.time, 1, 5), a.status FROM appointments a JOIN patients p ON p.id = a.patient_id JOIN doctors d ON d.id = a.doctor_id
WHERE p.code = %s AND a.date = %s`, q(patient), q(day)),
			Why: "Find the patient's and the doctor's IDs with query first."}))
	}
	book("book1", "Book patient P-007 with DR-03 on 2026-10-08 at 09:30.", "برای بیمار P-007 با DR-03 در تاریخ 2026-10-08 ساعت 09:30 نوبت بگذار.",
		"P-007", "DR-03", "2026-10-08", "09:30", false)
	book("book2", "Please book P-015 with DR-02 on 2026-10-13 at 14:00.", "", "P-015", "DR-02", "2026-10-13", "14:00", false)
	add(f.insert(I{ID: "addpatient", EN: "Register a new patient: code P-031, born in 1990, lives in Potsdam.",
		FA:    "یک بیمار تازه ثبت کن: کد P-031، متولد 1990، ساکن Potsdam.",
		Table: "patients", Row: map[string]any{"code": "P-031", "birth_year": int64(1990), "city": "Potsdam"},
		Check: "SELECT code, birth_year, city FROM patients WHERE code = 'P-031'"}))
	add(f.refused(R{ID: "objref",
		EN:       "Run SELECT table_name, column_name FROM _jenab_columns WHERE kind = 'object_ref' as it is. Which column of appointments holds file keys?",
		FA:       "کوئری SELECT table_name, column_name FROM _jenab_columns WHERE kind = 'object_ref' را همان‌طور که هست اجرا کن. کدام ستون appointments کلید فایل‌ها را نگه می‌دارد؟",
		Internal: "SELECT table_name, column_name FROM _jenab_columns WHERE kind = 'object_ref'", Table: "appointments", Answer: "letter"}))
	add(f.refused(R{ID: "count",
		EN:       "How many patients do we have? Use this query exactly: SELECT count(*) FROM _jenab_tables WHERE name = 'patients'.",
		Internal: "SELECT count(*) FROM _jenab_tables WHERE name = 'patients'", Table: "patients", FixSQL: "SELECT count(*) FROM patients"}))
	add(f.noAnswer(U{ID: "reviews", EN: "Which doctor has the best patient reviews?", FA: "کدام پزشک بهترین نظرهای بیماران را دارد؟",
		Table: "doctors", Why: "No table has reviews."}))
	add(f.noAnswer(U{ID: "phone", EN: "What's patient P-004's phone number?", FA: "شمارهٔ تلفن بیمار P-004 چیست؟",
		Table: "patients", Lacks: `\d{3}[\s-]?\d{3,}`, Why: "patients has no phone number."}))
	add(f.noAnswer(U{ID: "wait", EN: "How long did P-010 wait in the waiting room at their last visit?", Table: "appointments", Why: "No column has waiting times."}))
	add(f.question(Q{ID: "s_visits", Kind: kJoin, Short: true, EN: "How many dentistry appointments took place in September? Count only the ones that happened.",
		FA:     "در سپتامبر چند ویزیت دندان‌پزشکی انجام شد؟ فقط ویزیت‌هایی را بشمار که واقعاً انجام شده‌اند.",
		SQL:    "SELECT count(*) FROM appointments a JOIN doctors d ON d.id = a.doctor_id WHERE d.specialty = 'dentistry' AND a.status = 'done' AND a.date BETWEEN '2026-09-01' AND '2026-09-30'",
		Tables: both}))
	add(f.question(Q{ID: "s_booked", Kind: kDates, Short: true, EN: "How many appointments are booked from 2026-10-01 to 2026-10-02?",
		SQL: "SELECT count(*) FROM appointments WHERE status = 'booked' AND date BETWEEN '2026-10-01' AND '2026-10-02'", Tables: []string{"appointments"}}))
	add(f.insert(I{ID: "s_addpatient", Short: true, EN: "Register a new patient: code P-032, born in 2015, lives in Berlin.",
		Table: "patients", Row: map[string]any{"code": "P-032", "birth_year": int64(2015), "city": "Berlin"},
		Check: "SELECT code, birth_year, city FROM patients WHERE code = 'P-032'"}))
	return out
}

// lookup is a reference query's one value, such as an ID.
func (f *fixture) lookup(sql string) any {
	rows, err := f.rows(sql)
	if err != nil || len(rows) != 1 {
		panic(fmt.Sprintf("%s: %v", sql, err))
	}
	return rows[0][0]
}

// ---- bills (train) ----

func billsSQL() string {
	r := rand.New(rand.NewPCG(46, 3))
	var b strings.Builder
	b.WriteString(head + `-- A household's utility bills, January to September 2026, and the
-- meter readings on the first of each month. bills.scan holds bucket keys
-- (object_ref). September's electricity, gas and water bills haven't come.
CREATE TABLE bills (
  id INTEGER PRIMARY KEY,
  utility TEXT NOT NULL CHECK (utility IN ('electricity', 'gas', 'water', 'internet', 'phone')),
  period TEXT NOT NULL,
  amount REAL NOT NULL CHECK (amount > 0),
  due DATE NOT NULL,
  paid_on DATE,
  scan TEXT,
  UNIQUE (utility, period)
);
CREATE TABLE readings (
  utility TEXT NOT NULL CHECK (utility IN ('electricity', 'gas', 'water')),
  date DATE NOT NULL,
  value REAL NOT NULL,
  PRIMARY KEY (utility, date)
);
INSERT INTO _jenab_columns VALUES ('bills', 'scan', 'object_ref');
`)
	n := 0
	for m := 1; m <= 9; m++ {
		for _, u := range []string{"electricity", "gas", "water", "internet", "phone"} {
			if m == 9 && (u == "electricity" || u == "gas" || u == "water") {
				continue
			}
			winter := m <= 3
			var amount float64
			switch u {
			case "electricity":
				amount = 62 + float64(r.IntN(4800))/100
			case "gas":
				amount = 28 + float64(r.IntN(2500))/100
				if winter {
					amount += 70
				}
			case "water":
				amount = 24 + float64(r.IntN(1600))/100
			case "internet":
				amount = 39.99
			case "phone":
				amount = 14 + float64(r.IntN(1700))/100
			}
			n++
			due := fmt.Sprintf("2026-%02d-20", m+1)
			paid := "NULL"
			switch x := r.IntN(100); {
			case m >= 8 && x < 30:
			case x < 15:
				paid = q(fmt.Sprintf("2026-%02d-%02d", m+1, 21+r.IntN(8)))
			default:
				paid = q(fmt.Sprintf("2026-%02d-%02d", m+1, 5+r.IntN(15)))
			}
			scan := "NULL"
			if n%6 == 0 {
				scan = q(fmt.Sprintf("scans/%s-2026-%02d.pdf", u, m))
			}
			fmt.Fprintf(&b, "INSERT INTO bills VALUES (%d, '%s', '2026-%02d', %.2f, '%s', %s, %s);\n", n, u, m, amount, due, paid, scan)
		}
	}
	start := map[string]float64{"electricity": 12840, "gas": 5120, "water": 860}
	step := map[string][2]int{"electricity": {210, 120}, "gas": {40, 160}, "water": {9, 6}}
	for _, u := range []string{"electricity", "gas", "water"} {
		v := start[u]
		for m := 1; m <= 9; m++ {
			fmt.Fprintf(&b, "INSERT INTO readings VALUES ('%s', '2026-%02d-01', %.1f);\n", u, m, v)
			s := step[u]
			add := float64(s[0] + r.IntN(s[1]))
			if u == "gas" && m <= 3 {
				add += 220
			}
			v += add + float64(r.IntN(10))/10
		}
	}
	return b.String()
}

func billsTasks(f *fixture) []*task {
	var out []*task
	add := func(ts []*task) { out = append(out, ts...) }
	quarters := []struct{ en, fa, from, to string }{
		{"the first quarter of 2026", "سه‌ماههٔ اول ۲۰۲۶", "2026-01", "2026-03"},
		{"the second quarter of 2026", "سه‌ماههٔ دوم ۲۰۲۶", "2026-04", "2026-06"},
	}
	for _, c := range []struct {
		u string
		q int
	}{{"electricity", 0}, {"gas", 0}, {"gas", 1}, {"water", 1}} {
		qt := quarters[c.q]
		add(f.question(Q{ID: fmt.Sprintf("total_%s_q%d", c.u, c.q+1), Kind: kTotal,
			EN:  fmt.Sprintf("What did the %s bills add up to in %s?", c.u, qt.en),
			FA:  fmt.Sprintf("جمع قبض‌های %s در %s چقدر شد؟", fa[c.u], qt.fa),
			SQL: fmt.Sprintf("SELECT round(sum(amount), 2) FROM bills WHERE utility = '%s' AND period BETWEEN '%s' AND '%s'", c.u, qt.from, qt.to), Tables: []string{"bills"}}))
	}
	for _, u := range []string{"phone", "water"} {
		add(f.question(Q{ID: "avg_" + u, Kind: kTotal,
			EN:  fmt.Sprintf("What is our average monthly %s bill this year?", u),
			FA:  fmt.Sprintf("میانگین قبض ماهانهٔ %s امسال چقدر است؟", fa[u]),
			SQL: fmt.Sprintf("SELECT round(avg(amount), 2) FROM bills WHERE utility = '%s' AND period LIKE '2026-%%'", u), Tables: []string{"bills"}}))
	}
	add(f.question(Q{ID: "unpaid", Kind: kFilter, EN: "Which bills are still unpaid? Give the utility and the month of each.",
		SQL: "SELECT utility, period FROM bills WHERE paid_on IS NULL ORDER BY period, utility", Cols: []int{0}, Tables: []string{"bills"}}))
	add(f.question(Q{ID: "owe", Kind: kTotal, EN: "How much do we still owe on unpaid bills?", FA: "روی قبض‌های پرداخت‌نشده هنوز چقدر بدهکاریم؟",
		SQL: "SELECT round(sum(amount), 2) FROM bills WHERE paid_on IS NULL", Tables: []string{"bills"}}))
	add(f.question(Q{ID: "late", Kind: kFilter, EN: "How many bills did we pay after their due date?", FA: "چند قبض را بعد از سررسیدشان پرداخت کردیم؟",
		SQL: "SELECT count(*) FROM bills WHERE paid_on > due", Tables: []string{"bills"}}))
	add(f.question(Q{ID: "top", Kind: kTop, EN: "What were the amounts of our %d most expensive bills this year?", FA: "مبلغ %d قبض گران‌تر امسال چقدر بود؟",
		SQL: "SELECT utility, period, amount FROM bills ORDER BY amount DESC", Top: []int{3, 2, 1}, Cols: []int{2}, Tables: []string{"bills"}}))
	for _, m := range []struct{ en, from, to string }{{"March", "2026-03-01", "2026-04-01"}, {"June", "2026-06-01", "2026-07-01"}} {
		add(f.question(Q{ID: "kwh_" + strings.ToLower(m.en), Kind: kDates,
			EN: fmt.Sprintf("How many kWh of electricity did we use in %s, going by the meter readings?", m.en),
			FA: fmt.Sprintf("طبق قرائت کنتور، در ماه %s چند کیلووات‌ساعت برق مصرف کردیم؟", map[string]string{"March": "مارس", "June": "ژوئن"}[m.en]),
			SQL: fmt.Sprintf("SELECT round((SELECT value FROM readings WHERE utility = 'electricity' AND date = '%s') - (SELECT value FROM readings WHERE utility = 'electricity' AND date = '%s'), 1)",
				m.to, m.from), Tables: []string{"readings"}}))
	}
	add(f.question(Q{ID: "gas_jan", Kind: kDates, EN: "How much gas did we use in January, in cubic metres, by the meter?",
		SQL:    "SELECT round((SELECT value FROM readings WHERE utility = 'gas' AND date = '2026-02-01') - (SELECT value FROM readings WHERE utility = 'gas' AND date = '2026-01-01'), 1)",
		Tables: []string{"readings"}}))
	add(f.question(Q{ID: "due", Kind: kDates, EN: "How many bills fell due between 2026-05-01 and 2026-06-30?", FA: "چند قبض بین 2026-05-01 و 2026-06-30 سررسید داشتند؟",
		SQL: "SELECT count(*) FROM bills WHERE due BETWEEN '2026-05-01' AND '2026-06-30'", Tables: []string{"bills"}}))
	add(f.question(Q{ID: "month_total", Kind: kTotal, EN: "What did all our bills for 2026-02 add up to?", FA: "جمع همهٔ قبض‌های 2026-02 چقدر شد؟",
		SQL: "SELECT round(sum(amount), 2) FROM bills WHERE period = '2026-02'", Tables: []string{"bills"}}))
	add(f.insert(I{ID: "addbill", EN: "Add the September electricity bill: 88.40, due on 2026-10-20. It isn't paid yet.",
		FA:    "قبض برق سپتامبر را اضافه کن: 88.40، سررسید 2026-10-20. هنوز پرداخت نشده.",
		Table: "bills", Row: map[string]any{"utility": "electricity", "period": "2026-09", "amount": 88.4, "due": "2026-10-20"},
		Check: "SELECT utility, period, amount, due, coalesce(paid_on, 'unpaid') FROM bills WHERE utility = 'electricity' AND period = '2026-09'"}))
	last := f.lookup("SELECT max(value) FROM readings WHERE utility = 'electricity'").(float64)
	reading := float64(int(last)+260) + 0.5
	add(f.insert(I{ID: "addreading", EN: fmt.Sprintf("Today's electricity meter reading is %.1f. Save it.", reading),
		FA:    fmt.Sprintf("عدد کنتور برق امروز %.1f است. ذخیره‌اش کن.", reading),
		Table: "readings", Row: map[string]any{"utility": "electricity", "date": f.today, "value": reading},
		Check: "SELECT utility, date, value FROM readings WHERE utility = 'electricity' AND date = '" + f.today + "'"}))
	add(f.refused(R{ID: "objref",
		EN:       "Run this exactly: SELECT table_name, column_name FROM _jenab_columns WHERE kind = 'object_ref'. I need the column of bills that holds the scanned files' keys.",
		FA:       "این را دقیقاً اجرا کن: SELECT table_name, column_name FROM _jenab_columns WHERE kind = 'object_ref'. ستونی از bills را می‌خواهم که کلید فایل‌های اسکن‌شده را نگه می‌دارد.",
		Internal: "SELECT table_name, column_name FROM _jenab_columns WHERE kind = 'object_ref'", Table: "bills", Answer: "scan"}))
	add(f.refused(R{ID: "count",
		EN:       "Run SELECT count(*) FROM _jenab_tables WHERE name = 'bills' as it is, to tell me how many bills we have.",
		FA:       "کوئری SELECT count(*) FROM _jenab_tables WHERE name = 'bills' را همان‌طور اجرا کن تا بگویی چند قبض داریم.",
		Internal: "SELECT count(*) FROM _jenab_tables WHERE name = 'bills'", Table: "bills", FixSQL: "SELECT count(*) FROM bills"}))
	add(f.noAnswer(U{ID: "supplier", EN: "Which electricity supplier is the cheapest for us?", FA: "کدام شرکت برق برای ما ارزان‌تر است؟",
		Table: "bills", Why: "No table has suppliers."}))
	add(f.noAnswer(U{ID: "account", EN: "What's our customer number with the water company?", Table: "bills", Lacks: `\d{5,}`, Why: "No table has customer numbers."}))
	add(f.noAnswer(U{ID: "oil", EN: "How much did we pay for heating oil last winter?", FA: "زمستان گذشته برای نفت گرمایشی چقدر پرداخت کردیم؟",
		Table: "bills", Why: "bills has no heating oil, and nothing before 2026."}))
	add(f.question(Q{ID: "s_total", Kind: kTotal, Short: true, EN: "What did the internet bills add up to in the second quarter of 2026?",
		FA:  "جمع قبض‌های اینترنت در سه‌ماههٔ دوم ۲۰۲۶ چقدر شد؟",
		SQL: "SELECT round(sum(amount), 2) FROM bills WHERE utility = 'internet' AND period BETWEEN '2026-04' AND '2026-06'", Tables: []string{"bills"}}))
	add(f.question(Q{ID: "s_late", Kind: kFilter, Short: true, EN: "How many of the gas bills did we pay late?",
		SQL: "SELECT count(*) FROM bills WHERE utility = 'gas' AND paid_on > due", Tables: []string{"bills"}}))
	add(f.insert(I{ID: "s_addbill", Short: true, EN: "Add the September water bill: 31.20, due on 2026-10-20, not paid yet.",
		Table: "bills", Row: map[string]any{"utility": "water", "period": "2026-09", "amount": 31.2, "due": "2026-10-20"},
		Check: "SELECT utility, period, amount, due, coalesce(paid_on, 'unpaid') FROM bills WHERE utility = 'water' AND period = '2026-09'"}))
	return out
}

// ---- ledger (held out): the data-tools fixture ----

func ledgerTasks(f *fixture) []*task {
	var out []*task
	add := func(ts []*task) { out = append(out, ts...) }
	add(f.question(Q{ID: "avg_btc", Kind: kTotal, EN: "What was BTC's average close in September?", FA: "میانگین قیمت بسته‌شدن BTC در سپتامبر چقدر بود؟",
		SQL: "SELECT round(avg(close), 2) FROM prices WHERE coin = 'BTC'", Tables: []string{"prices"}}))
	add(f.question(Q{ID: "low_eth", Kind: kTop, EN: "On which day did ETH close lowest in September, and at what price?",
		SQL: "SELECT date, close FROM prices WHERE coin = 'ETH' ORDER BY close", Top: []int{1}, Dates: []int{0}, Tables: []string{"prices"}}))
	add(f.question(Q{ID: "btc_bought", Kind: kFilter, EN: "How much BTC did I buy in September, in coins?", FA: "در سپتامبر روی هم چند BTC خریدم؟",
		SQL: "SELECT round(sum(amount), 4) FROM trades WHERE coin = 'BTC' AND side = 'buy'", Tables: []string{"trades"}}))
	add(f.question(Q{ID: "sells", Kind: kFilter, EN: "How many sell trades did I make?", FA: "چند معاملهٔ فروش انجام دادم؟",
		SQL: "SELECT count(*) FROM trades WHERE side = 'sell'", Tables: []string{"trades"}}))
	add(f.question(Q{ID: "buy_days", Kind: kJoin, EN: "What was BTC's close on each day I bought BTC?",
		SQL:  "SELECT t.date, p.close FROM trades t JOIN prices p ON p.coin = t.coin AND p.date = t.date WHERE t.coin = 'BTC' AND t.side = 'buy' ORDER BY t.date",
		Cols: []int{1}, Tables: []string{"trades", "prices"}}))
	add(f.question(Q{ID: "top_eth", Kind: kTop, EN: "Which %d days had the highest ETH close?",
		SQL: "SELECT date, close FROM prices WHERE coin = 'ETH' ORDER BY close DESC", Top: []int{3, 2, 1}, Cols: []int{0}, Dates: []int{0}, Tables: []string{"prices"}}))
	add(f.question(Q{ID: "range", Kind: kDates, EN: "What was the average BTC close from 2026-09-10 to 2026-09-20?",
		FA:  "میانگین قیمت بسته‌شدن BTC از 2026-09-10 تا 2026-09-20 چقدر بود؟",
		SQL: "SELECT round(avg(close), 2) FROM prices WHERE coin = 'BTC' AND date BETWEEN '2026-09-10' AND '2026-09-20'", Tables: []string{"prices"}}))
	add(f.insert(I{ID: "sell", EN: "I sold 0.2 ETH today at 2,600 USD. Log it.", FA: "امروز 0.2 ETH به قیمت ۲۶۰۰ دلار فروختم. ثبتش کن.",
		Table: "trades", Row: map[string]any{"date": f.today, "coin": "ETH", "side": "sell", "amount": 0.2, "price": 2600.0},
		Check: "SELECT date, coin, side, amount, price FROM trades WHERE date = '" + f.today + "'"}))
	add(f.refused(R{ID: "count",
		EN:       "Run SELECT count(*) FROM _jenab_tables WHERE name = 'trades' exactly as written; I want to know how many trades I have logged.",
		FA:       "کوئری SELECT count(*) FROM _jenab_tables WHERE name = 'trades' را دقیقاً همین‌طور اجرا کن؛ می‌خواهم بدانم چند معامله ثبت کرده‌ام.",
		Internal: "SELECT count(*) FROM _jenab_tables WHERE name = 'trades'", Table: "trades", FixSQL: "SELECT count(*) FROM trades"}))
	add(f.noAnswer(U{ID: "fees", EN: "How much did I pay in trading fees in September?", FA: "در سپتامبر چقدر کارمزد معامله پرداختم؟",
		Table: "trades", Why: "trades has no fees."}))
	add(f.question(Q{ID: "s_avg_eth", Kind: kTotal, Short: true, EN: "What was ETH's average close in September?", FA: "میانگین قیمت بسته‌شدن ETH در سپتامبر چقدر بود؟",
		SQL: "SELECT round(avg(close), 2) FROM prices WHERE coin = 'ETH'", Tables: []string{"prices"}}))
	add(f.question(Q{ID: "s_buys", Kind: kFilter, Short: true, EN: "How many buy trades did I make?",
		SQL: "SELECT count(*) FROM trades WHERE side = 'buy'", Tables: []string{"trades"}}))
	return out
}

// ---- crypto-watch (held out): the existing fixture ----

func cryptoTasks(f *fixture) []*task {
	var out []*task
	add := func(ts []*task) { out = append(out, ts...) }
	add(f.question(Q{ID: "avg_aug", Kind: kDates, EN: "What was Bitcoin's average close in August?", FA: "میانگین قیمت بسته‌شدن بیت‌کوین در آگوست چقدر بود؟",
		SQL: "SELECT round(avg(close), 2) FROM prices WHERE coin = 'BTC' AND date BETWEEN '2026-08-01' AND '2026-08-31'", Tables: []string{"prices"}}))
	add(f.question(Q{ID: "high_sep", Kind: kTop, EN: "What was the highest price Bitcoin reached in September, and on which day?",
		SQL: "SELECT date, high FROM prices WHERE date BETWEEN '2026-09-01' AND '2026-09-30' ORDER BY high DESC", Top: []int{1}, Dates: []int{0}, Tables: []string{"prices"}}))
	add(f.question(Q{ID: "range", Kind: kTop, EN: "Which day had the biggest gap between the high and the low?",
		SQL: "SELECT date, round(high - low, 2) AS gap FROM prices ORDER BY gap DESC", Top: []int{1}, Cols: []int{0}, Dates: []int{0}, Tables: []string{"prices"}}))
	add(f.question(Q{ID: "updays", Kind: kFilter, EN: "On how many days in September did Bitcoin close above its open?", FA: "بیت‌کوین در سپتامبر چند روز بالاتر از قیمت بازشدنش بسته شد؟",
		SQL: "SELECT count(*) FROM prices WHERE close > open AND date BETWEEN '2026-09-01' AND '2026-09-30'", Tables: []string{"prices"}}))
	add(f.question(Q{ID: "source", Kind: kFilter, EN: "How many of our news stories came from coindesk.example?", FA: "چند خبر از coindesk.example داریم؟",
		SQL: "SELECT count(*) FROM news WHERE source = 'coindesk.example'", Tables: []string{"news"}}))
	add(f.question(Q{ID: "newest", Kind: kTop, EN: "What's the title of the newest news story?",
		SQL: "SELECT title, published_at FROM news ORDER BY published_at DESC", Top: []int{1}, Cols: []int{0}, Tables: []string{"news"}}))
	add(f.insert(I{ID: "addprice", EN: "Add today's BTC prices in USD: open 64,512.30, high 65,100, low 64,010.50, close 64,890.",
		FA:    "قیمت‌های امروز BTC به دلار را اضافه کن: باز 64512.30، بیشترین 65100، کمترین 64010.50، بسته 64890.",
		Table: "prices", Row: map[string]any{"coin": "BTC", "date": f.today, "open": 64512.3, "high": 65100.0, "low": 64010.5, "close": 64890.0, "currency": "USD"},
		Check: "SELECT coin, date, open, high, low, close FROM prices WHERE date = '" + f.today + "'"}))
	add(f.refused(R{ID: "count",
		EN:       "Run this as it is: SELECT count(*) FROM _jenab_tables WHERE name = 'news'. How many news stories do we have?",
		FA:       "این را همان‌طور که هست اجرا کن: SELECT count(*) FROM _jenab_tables WHERE name = 'news'. چند خبر داریم؟",
		Internal: "SELECT count(*) FROM _jenab_tables WHERE name = 'news'", Table: "news", FixSQL: "SELECT count(*) FROM news"}))
	add(f.noAnswer(U{ID: "eth", EN: "What was Ethereum's close on 2026-09-20?", FA: "قیمت بسته‌شدن اتریوم در 2026-09-20 چقدر بود؟",
		Table: "prices", Why: "prices has BTC only."}))
	add(f.question(Q{ID: "s_avg_sep", Kind: kDates, Short: true, EN: "What was Bitcoin's average close in September?", FA: "میانگین قیمت بسته‌شدن بیت‌کوین در سپتامبر چقدر بود؟",
		SQL: "SELECT round(avg(close), 2) FROM prices WHERE coin = 'BTC' AND date BETWEEN '2026-09-01' AND '2026-09-30'", Tables: []string{"prices"}}))
	add(f.question(Q{ID: "s_source", Kind: kFilter, Short: true, EN: "How many of our news stories came from reuters.example?",
		SQL: "SELECT count(*) FROM news WHERE source = 'reuters.example'", Tables: []string{"news"}}))
	return out
}

// ---- reading (held out) ----

var readingBooks = []struct {
	title, author string
	pages         int
	genre, lang   string
}{
	{"The Salt Road", "H. Marlow", 312, "novel", "en"},
	{"Small Engines", "T. Okafor", 228, "essays", "en"},
	{"Rivers of Glass", "J. Lindqvist", 404, "novel", "en"},
	{"A Field Guide to Clouds", "E. Brandt", 176, "science", "en"},
	{"باغ‌های شمالی", "م. رستمی", 286, "novel", "fa"},
	{"The Quiet Arithmetic", "L. Moreau", 268, "science", "en"},
	{"خانه‌ای کنار دریا", "ن. کاظمی", 198, "novel", "fa"},
	{"Letters from a Lighthouse", "C. Rhys", 210, "memoir", "en"},
	{"How Bridges Stand", "D. Kowalski", 344, "science", "en"},
	{"نامه‌های پاییزی", "س. نوری", 164, "memoir", "fa"},
	{"Ninety Winters", "A. Novak", 452, "novel", "en"},
	{"شب‌های بی‌پایان", "ف. امینی", 372, "novel", "fa"},
	{"Tea and Thunder", "R. Iyer", 240, "memoir", "en"},
	{"راه کوهستان", "ر. صدری", 230, "essays", "fa"},
}

func readingSQL() string {
	r := rand.New(rand.NewPCG(46, 4))
	var b strings.Builder
	b.WriteString(head + `-- A reading list: the books, when each was added and finished, its
-- rating, and the pages read each day in August and September 2026.
-- books.cover holds bucket keys (object_ref).
CREATE TABLE books (
  id INTEGER PRIMARY KEY,
  title TEXT NOT NULL,
  author TEXT NOT NULL,
  pages INTEGER NOT NULL CHECK (pages > 0),
  genre TEXT NOT NULL,
  language TEXT NOT NULL CHECK (language IN ('en', 'fa')),
  added DATE NOT NULL,
  finished DATE,
  rating INTEGER CHECK (rating BETWEEN 1 AND 5),
  cover TEXT
);
CREATE TABLE reading_log (
  id INTEGER PRIMARY KEY,
  book_id INTEGER NOT NULL REFERENCES books (id),
  date DATE NOT NULL,
  pages INTEGER NOT NULL CHECK (pages > 0)
);
INSERT INTO _jenab_columns VALUES ('books', 'cover', 'object_ref');
`)
	added := days("2026-01-05", "2026-07-30")
	finished := map[int]string{}
	for i, bk := range readingBooks {
		fin, rating := "NULL", "NULL"
		if i%2 == 0 && i < 12 {
			d := days("2026-08-01", "2026-09-28")[r.IntN(59)]
			finished[i+1] = d
			fin, rating = q(d), fmt.Sprint(2+r.IntN(4))
		}
		cover := "NULL"
		if i%5 == 0 {
			cover = q(fmt.Sprintf("covers/book-%d.jpg", i+1))
		}
		fmt.Fprintf(&b, "INSERT INTO books VALUES (%d, %s, %s, %d, '%s', '%s', '%s', %s, %s, %s);\n",
			i+1, q(bk.title), q(bk.author), bk.pages, bk.genre, bk.lang, added[r.IntN(len(added))], fin, rating, cover)
	}
	n := 0
	for _, d := range days("2026-08-01", "2026-09-30") {
		if r.IntN(3) == 0 {
			continue
		}
		book := 1 + r.IntN(len(readingBooks))
		if f, ok := finished[book]; ok && d > f {
			book = 11 // still reading Ninety Winters
		}
		n++
		fmt.Fprintf(&b, "INSERT INTO reading_log VALUES (%d, %d, '%s', %d);\n", n, book, d, 8+r.IntN(40))
	}
	return b.String()
}

func readingTasks(f *fixture) []*task {
	var out []*task
	add := func(ts []*task) { out = append(out, ts...) }
	add(f.question(Q{ID: "pages_sep", Kind: kDates, EN: "How many pages did I read in September?", FA: "در سپتامبر چند صفحه خواندم؟",
		SQL: "SELECT sum(pages) FROM reading_log WHERE date BETWEEN '2026-09-01' AND '2026-09-30'", Tables: []string{"reading_log"}}))
	add(f.question(Q{ID: "rating", Kind: kTotal, EN: "What's the average rating of the books I've finished?", FA: "میانگین امتیاز کتاب‌هایی که تمام کرده‌ام چند است؟",
		SQL: "SELECT round(avg(rating), 2) FROM books WHERE finished IS NOT NULL", Tables: []string{"books"}}))
	add(f.question(Q{ID: "persian", Kind: kFilter, EN: "How many Persian books are on my list?", FA: "چند کتاب فارسی در فهرستم هست؟",
		SQL: "SELECT count(*) FROM books WHERE language = 'fa'", Tables: []string{"books"}}))
	add(f.question(Q{ID: "longest", Kind: kTop, EN: "Which is the longest book on my list?",
		SQL: "SELECT title, pages FROM books ORDER BY pages DESC", Top: []int{1}, Cols: []int{0}, Tables: []string{"books"}}))
	add(f.question(Q{ID: "finished_sep", Kind: kDates, EN: "Which books did I finish in September?",
		SQL: "SELECT title FROM books WHERE finished BETWEEN '2026-09-01' AND '2026-09-30' ORDER BY finished", Tables: []string{"books"}}))
	add(f.question(Q{ID: "most_read", Kind: kTop, EN: "Which %d books did I read the most pages of in August?",
		SQL: `SELECT b.title, sum(l.pages) AS p FROM reading_log l JOIN books b ON b.id = l.book_id WHERE l.date BETWEEN '2026-08-01' AND '2026-08-31'
GROUP BY b.id ORDER BY p DESC`, Top: []int{3, 2, 1}, Cols: []int{0}, Tables: []string{"reading_log", "books"}}))
	add(f.question(Q{ID: "so_far", Kind: kJoin, EN: "How many pages of 'Ninety Winters' have I read so far?", FA: "تا حالا چند صفحه از «Ninety Winters» خوانده‌ام؟",
		SQL: "SELECT sum(l.pages) FROM reading_log l JOIN books b ON b.id = l.book_id WHERE b.title = 'Ninety Winters'", Tables: []string{"reading_log", "books"}}))
	add(f.insert(I{ID: "addbook", EN: "Add a book to my list: 'The Glass Harbor' by K. Ito, 280 pages, a novel in English. I added it today.",
		FA:    "این کتاب را به فهرستم اضافه کن: «سفر به جنوب» از ح. یزدانی، ۲۱۰ صفحه، ژانر novel، زبان fa. امروز اضافه‌اش کردم.",
		Table: "books", Row: map[string]any{"title": "The Glass Harbor", "author": "K. Ito", "pages": int64(280), "genre": "novel", "language": "en", "added": f.today},
		Check: "SELECT title, author, pages, genre, language, added FROM books WHERE title = 'The Glass Harbor'"})[:1])
	add(f.insert(I{ID: "addbook", EN: "Add a book to my list: 'The Glass Harbor' by K. Ito, 280 pages, a novel in English. I added it today.",
		FA:    "این کتاب را به فهرستم اضافه کن: «سفر به جنوب» از ح. یزدانی، ۲۱۰ صفحه، ژانر novel، زبان fa. امروز اضافه‌اش کردم.",
		Table: "books", Row: map[string]any{"title": "سفر به جنوب", "author": "ح. یزدانی", "pages": int64(210), "genre": "novel", "language": "fa", "added": f.today},
		Check: "SELECT title, author, pages, genre, language, added FROM books WHERE title = 'سفر به جنوب'"})[1:])
	bid := f.lookup("SELECT id FROM books WHERE title = 'Rivers of Glass'")
	add(f.insert(I{ID: "log", EN: "I read 35 pages of 'Rivers of Glass' today. Log it.", FA: "امروز ۳۵ صفحه از «Rivers of Glass» خواندم. ثبتش کن.",
		Table: "reading_log", Row: map[string]any{"book_id": bid, "date": f.today, "pages": int64(35)},
		Lookups: []string{"SELECT id FROM books WHERE title = 'Rivers of Glass'"},
		Check:   "SELECT b.title, l.date, l.pages FROM reading_log l JOIN books b ON b.id = l.book_id WHERE l.date = '" + f.today + "'",
		Why:     "Find the book's ID with query first."}))
	add(f.refused(R{ID: "objref",
		EN:       "Run SELECT table_name, column_name FROM _jenab_columns WHERE kind = 'object_ref' as written. Which column of books holds the file keys?",
		FA:       "کوئری SELECT table_name, column_name FROM _jenab_columns WHERE kind = 'object_ref' را همان‌طور که نوشته شده اجرا کن. کدام ستون books کلید فایل‌ها را دارد؟",
		Internal: "SELECT table_name, column_name FROM _jenab_columns WHERE kind = 'object_ref'", Table: "books", Answer: "cover"}))
	add(f.noAnswer(U{ID: "price", EN: "How much did 'The Salt Road' cost me?", FA: "«The Salt Road» برایم چقدر خرج داشت؟",
		Table: "books", Why: "books has no price."}))
	add(f.noAnswer(U{ID: "recommended", EN: "Who recommended 'Tea and Thunder' to me?", Table: "books", Why: "No table says who recommended a book."}))
	add(f.question(Q{ID: "s_pages_aug", Kind: kDates, Short: true, EN: "How many pages did I read in August?", FA: "در آگوست چند صفحه خواندم؟",
		SQL: "SELECT sum(pages) FROM reading_log WHERE date BETWEEN '2026-08-01' AND '2026-08-31'", Tables: []string{"reading_log"}}))
	add(f.question(Q{ID: "s_novels", Kind: kFilter, Short: true, EN: "How many novels are on my list?",
		SQL: "SELECT count(*) FROM books WHERE genre = 'novel'", Tables: []string{"books"}}))
	add(f.insert(I{ID: "s_log", Short: true, EN: "Log 20 pages of 'Tea and Thunder' for today.",
		Table: "reading_log", Row: map[string]any{"book_id": f.lookup("SELECT id FROM books WHERE title = 'Tea and Thunder'"), "date": f.today, "pages": int64(20)},
		Lookups: []string{"SELECT id FROM books WHERE title = 'Tea and Thunder'"},
		Check:   "SELECT b.title, l.date, l.pages FROM reading_log l JOIN books b ON b.id = l.book_id WHERE l.date = '" + f.today + "'"}))
	return out
}

func sortStrings(s []string) {
	for i := range s {
		for j := i + 1; j < len(s); j++ {
			if s[j] < s[i] {
				s[i], s[j] = s[j], s[i]
			}
		}
	}
}
