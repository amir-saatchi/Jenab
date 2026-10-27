CREATE TABLE "btc_news" (
  "url" TEXT,
  "date" TEXT,
  "title" TEXT,
  "_run_id" TEXT,
  PRIMARY KEY ("url")
);
CREATE TABLE "btc_prices" (
  "date" TEXT,
  "price" REAL,
  "_run_id" TEXT,
  PRIMARY KEY ("date")
);
CREATE TABLE "eth_news" (
  "url" TEXT,
  "date" TEXT,
  "title" TEXT,
  "_run_id" TEXT,
  PRIMARY KEY ("url")
);
CREATE TABLE "eth_prices" (
  "date" TEXT,
  "price" REAL,
  "_run_id" TEXT,
  PRIMARY KEY ("date")
);
CREATE TABLE "price_alerts" (
  "id" INTEGER PRIMARY KEY,
  "coin" TEXT,
  "date" TEXT,
  "drop_pct" REAL
);
CREATE TABLE "watch_keywords" (
  "id" INTEGER PRIMARY KEY,
  "keyword" TEXT NOT NULL,
  UNIQUE ("keyword")
);
INSERT INTO "btc_news" VALUES('https://news.example.com/b/1','2026-10-05','Seed story 1','run_1');
INSERT INTO "btc_news" VALUES('https://news.example.com/b/2','2026-10-04','Seed story 2','run_2');
INSERT INTO "btc_news" VALUES('https://news.example.com/b/3','2026-10-04','Seed story 3','run_2');
INSERT INTO "eth_news" VALUES('https://news.example.com/e/1','2026-10-05','Ether story 1','run_1');
INSERT INTO "eth_news" VALUES('https://news.example.com/e/2','2026-10-03','Ether story 2','run_3');
INSERT INTO "btc_prices" VALUES('2026-09-26',61000,'run_10');
INSERT INTO "btc_prices" VALUES('2026-09-27',60900,'run_9');
INSERT INTO "btc_prices" VALUES('2026-09-28',60800,'run_8');
INSERT INTO "btc_prices" VALUES('2026-09-29',60700,'run_7');
INSERT INTO "btc_prices" VALUES('2026-09-30',60600,'run_6');
INSERT INTO "btc_prices" VALUES('2026-10-01',60500,'run_5');
INSERT INTO "btc_prices" VALUES('2026-10-02',60400,'run_4');
INSERT INTO "btc_prices" VALUES('2026-10-03',60300,'run_3');
INSERT INTO "btc_prices" VALUES('2026-10-04',60200,'run_2');
INSERT INTO "btc_prices" VALUES('2026-10-05',60100,'run_1');
INSERT INTO "eth_prices" VALUES('2026-09-26',2410,'run_10');
INSERT INTO "eth_prices" VALUES('2026-09-27',2398,'run_9');
INSERT INTO "eth_prices" VALUES('2026-09-28',2405,'run_8');
INSERT INTO "eth_prices" VALUES('2026-09-29',2380,'run_7');
INSERT INTO "eth_prices" VALUES('2026-09-30',2371,'run_6');
INSERT INTO "eth_prices" VALUES('2026-10-01',2299,'run_5');
INSERT INTO "eth_prices" VALUES('2026-10-02',2310,'run_4');
INSERT INTO "eth_prices" VALUES('2026-10-03',2322,'run_3');
INSERT INTO "eth_prices" VALUES('2026-10-04',2301,'run_2');
INSERT INTO "eth_prices" VALUES('2026-10-05',2315,'run_1');
INSERT INTO "watch_keywords" VALUES(1,'ETF');
INSERT INTO "watch_keywords" VALUES(2,'regulation');
