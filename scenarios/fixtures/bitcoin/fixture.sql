CREATE TABLE "btc_news" (
  "url" TEXT,
  "date" TEXT,
  "title" TEXT,
  "reason" TEXT,
  "_run_id" TEXT,
  "_fetched_at" TEXT,
  PRIMARY KEY ("url")
);
CREATE TABLE "btc_prices" (
  "date" TEXT,
  "price" REAL,
  "_run_id" TEXT,
  "_fetched_at" TEXT,
  PRIMARY KEY ("date")
);
CREATE TABLE "watch_keywords" (
  "id" INTEGER PRIMARY KEY,
  "keyword" TEXT NOT NULL,
  UNIQUE ("keyword")
);
INSERT INTO "btc_news" VALUES('https://news.example.com/seed/1','2026-10-05','Seed story 1','seed reason','run_seed_1','2026-10-05T08:00:09Z');
INSERT INTO "btc_news" VALUES('https://news.example.com/seed/2','2026-10-04','Seed story 2','seed reason','run_seed_2','2026-10-04T08:00:09Z');
INSERT INTO "btc_news" VALUES('https://news.example.com/seed/3','2026-10-03','Seed story 3','seed reason','run_seed_3','2026-10-03T08:00:09Z');
INSERT INTO "btc_news" VALUES('https://news.example.com/seed/4','2026-10-02','Seed story 4','seed reason','run_seed_4','2026-10-02T08:00:09Z');
INSERT INTO "btc_prices" VALUES('2026-09-26',61000,'run_seed_10','2026-09-26T08:00:05Z');
INSERT INTO "btc_prices" VALUES('2026-09-27',60900,'run_seed_9','2026-09-27T08:00:05Z');
INSERT INTO "btc_prices" VALUES('2026-09-28',60800,'run_seed_8','2026-09-28T08:00:05Z');
INSERT INTO "btc_prices" VALUES('2026-09-29',60700,'run_seed_7','2026-09-29T08:00:05Z');
INSERT INTO "btc_prices" VALUES('2026-09-30',60600,'run_seed_6','2026-09-30T08:00:05Z');
INSERT INTO "btc_prices" VALUES('2026-10-01',60500,'run_seed_5','2026-10-01T08:00:05Z');
INSERT INTO "btc_prices" VALUES('2026-10-02',60400,'run_seed_4','2026-10-02T08:00:05Z');
INSERT INTO "btc_prices" VALUES('2026-10-03',60300,'run_seed_3','2026-10-03T08:00:05Z');
INSERT INTO "btc_prices" VALUES('2026-10-04',60200,'run_seed_2','2026-10-04T08:00:05Z');
INSERT INTO "btc_prices" VALUES('2026-10-05',60100,'run_seed_1','2026-10-05T08:00:05Z');
INSERT INTO "watch_keywords" VALUES(1,'ETF');
INSERT INTO "watch_keywords" VALUES(2,'halving');
INSERT INTO "watch_keywords" VALUES(3,'regulation');
