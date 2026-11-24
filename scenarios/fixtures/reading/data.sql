-- SPIKE-046: a synthetic fixture, written by spikes/046-sql-lora/gen from a fixed seed.
-- A reading list: the books, when each was added and finished, its
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
INSERT INTO books VALUES (1, 'The Salt Road', 'H. Marlow', 312, 'novel', 'en', '2026-03-09', '2026-09-16', 5, 'covers/book-1.jpg');
INSERT INTO books VALUES (2, 'Small Engines', 'T. Okafor', 228, 'essays', 'en', '2026-06-06', NULL, NULL, NULL);
INSERT INTO books VALUES (3, 'Rivers of Glass', 'J. Lindqvist', 404, 'novel', 'en', '2026-03-15', '2026-08-24', 3, NULL);
INSERT INTO books VALUES (4, 'A Field Guide to Clouds', 'E. Brandt', 176, 'science', 'en', '2026-05-13', NULL, NULL, NULL);
INSERT INTO books VALUES (5, 'باغ‌های شمالی', 'م. رستمی', 286, 'novel', 'fa', '2026-04-09', '2026-08-16', 4, NULL);
INSERT INTO books VALUES (6, 'The Quiet Arithmetic', 'L. Moreau', 268, 'science', 'en', '2026-02-27', NULL, NULL, 'covers/book-6.jpg');
INSERT INTO books VALUES (7, 'خانه‌ای کنار دریا', 'ن. کاظمی', 198, 'novel', 'fa', '2026-06-24', '2026-09-17', 3, NULL);
INSERT INTO books VALUES (8, 'Letters from a Lighthouse', 'C. Rhys', 210, 'memoir', 'en', '2026-02-11', NULL, NULL, NULL);
INSERT INTO books VALUES (9, 'How Bridges Stand', 'D. Kowalski', 344, 'science', 'en', '2026-04-23', '2026-09-25', 3, NULL);
INSERT INTO books VALUES (10, 'نامه‌های پاییزی', 'س. نوری', 164, 'memoir', 'fa', '2026-07-27', NULL, NULL, NULL);
INSERT INTO books VALUES (11, 'Ninety Winters', 'A. Novak', 452, 'novel', 'en', '2026-04-10', '2026-09-09', 3, 'covers/book-11.jpg');
INSERT INTO books VALUES (12, 'شب‌های بی‌پایان', 'ف. امینی', 372, 'novel', 'fa', '2026-05-29', NULL, NULL, NULL);
INSERT INTO books VALUES (13, 'Tea and Thunder', 'R. Iyer', 240, 'memoir', 'en', '2026-04-27', NULL, NULL, NULL);
INSERT INTO books VALUES (14, 'راه کوهستان', 'ر. صدری', 230, 'essays', 'fa', '2026-02-08', NULL, NULL, NULL);
INSERT INTO reading_log VALUES (1, 2, '2026-08-01', 16);
INSERT INTO reading_log VALUES (2, 7, '2026-08-02', 26);
INSERT INTO reading_log VALUES (3, 9, '2026-08-04', 13);
INSERT INTO reading_log VALUES (4, 11, '2026-08-07', 18);
INSERT INTO reading_log VALUES (5, 9, '2026-08-09', 18);
INSERT INTO reading_log VALUES (6, 8, '2026-08-10', 15);
INSERT INTO reading_log VALUES (7, 11, '2026-08-11', 18);
INSERT INTO reading_log VALUES (8, 9, '2026-08-13', 21);
INSERT INTO reading_log VALUES (9, 1, '2026-08-15', 9);
INSERT INTO reading_log VALUES (10, 13, '2026-08-18', 38);
INSERT INTO reading_log VALUES (11, 2, '2026-08-19', 42);
INSERT INTO reading_log VALUES (12, 10, '2026-08-20', 43);
INSERT INTO reading_log VALUES (13, 12, '2026-08-22', 33);
INSERT INTO reading_log VALUES (14, 7, '2026-08-23', 17);
INSERT INTO reading_log VALUES (15, 13, '2026-08-25', 47);
INSERT INTO reading_log VALUES (16, 11, '2026-08-26', 11);
INSERT INTO reading_log VALUES (17, 7, '2026-08-27', 13);
INSERT INTO reading_log VALUES (18, 1, '2026-08-28', 45);
INSERT INTO reading_log VALUES (19, 7, '2026-08-30', 13);
INSERT INTO reading_log VALUES (20, 11, '2026-08-31', 21);
INSERT INTO reading_log VALUES (21, 8, '2026-09-02', 41);
INSERT INTO reading_log VALUES (22, 9, '2026-09-03', 13);
INSERT INTO reading_log VALUES (23, 6, '2026-09-05', 13);
INSERT INTO reading_log VALUES (24, 6, '2026-09-06', 22);
INSERT INTO reading_log VALUES (25, 6, '2026-09-07', 34);
INSERT INTO reading_log VALUES (26, 11, '2026-09-09', 22);
INSERT INTO reading_log VALUES (27, 6, '2026-09-10', 25);
INSERT INTO reading_log VALUES (28, 13, '2026-09-11', 41);
INSERT INTO reading_log VALUES (29, 6, '2026-09-13', 12);
INSERT INTO reading_log VALUES (30, 7, '2026-09-14', 33);
INSERT INTO reading_log VALUES (31, 12, '2026-09-16', 40);
INSERT INTO reading_log VALUES (32, 11, '2026-09-17', 39);
INSERT INTO reading_log VALUES (33, 14, '2026-09-18', 44);
INSERT INTO reading_log VALUES (34, 9, '2026-09-19', 35);
INSERT INTO reading_log VALUES (35, 12, '2026-09-20', 13);
INSERT INTO reading_log VALUES (36, 13, '2026-09-21', 39);
INSERT INTO reading_log VALUES (37, 13, '2026-09-22', 16);
INSERT INTO reading_log VALUES (38, 14, '2026-09-23', 18);
INSERT INTO reading_log VALUES (39, 12, '2026-09-25', 22);
INSERT INTO reading_log VALUES (40, 11, '2026-09-26', 22);
INSERT INTO reading_log VALUES (41, 14, '2026-09-27', 31);
INSERT INTO reading_log VALUES (42, 10, '2026-09-28', 14);
INSERT INTO reading_log VALUES (43, 11, '2026-09-30', 34);
