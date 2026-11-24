-- SPIKE-046: a synthetic fixture, written by spikes/046-sql-lora/gen from a fixed seed.
-- A household's utility bills, January to September 2026, and the
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
INSERT INTO bills VALUES (1, 'electricity', '2026-01', 93.94, '2026-02-20', '2026-02-06', NULL);
INSERT INTO bills VALUES (2, 'gas', '2026-01', 121.12, '2026-02-20', '2026-02-23', NULL);
INSERT INTO bills VALUES (3, 'water', '2026-01', 24.49, '2026-02-20', '2026-02-17', NULL);
INSERT INTO bills VALUES (4, 'internet', '2026-01', 39.99, '2026-02-20', '2026-02-14', NULL);
INSERT INTO bills VALUES (5, 'phone', '2026-01', 18.20, '2026-02-20', '2026-02-25', NULL);
INSERT INTO bills VALUES (6, 'electricity', '2026-02', 87.12, '2026-03-20', '2026-03-23', 'scans/electricity-2026-02.pdf');
INSERT INTO bills VALUES (7, 'gas', '2026-02', 117.96, '2026-03-20', '2026-03-26', NULL);
INSERT INTO bills VALUES (8, 'water', '2026-02', 25.48, '2026-03-20', '2026-03-18', NULL);
INSERT INTO bills VALUES (9, 'internet', '2026-02', 39.99, '2026-03-20', '2026-03-14', NULL);
INSERT INTO bills VALUES (10, 'phone', '2026-02', 26.37, '2026-03-20', '2026-03-16', NULL);
INSERT INTO bills VALUES (11, 'electricity', '2026-03', 95.25, '2026-04-20', '2026-04-14', NULL);
INSERT INTO bills VALUES (12, 'gas', '2026-03', 108.19, '2026-04-20', '2026-04-12', 'scans/gas-2026-03.pdf');
INSERT INTO bills VALUES (13, 'water', '2026-03', 39.59, '2026-04-20', '2026-04-22', NULL);
INSERT INTO bills VALUES (14, 'internet', '2026-03', 39.99, '2026-04-20', '2026-04-18', NULL);
INSERT INTO bills VALUES (15, 'phone', '2026-03', 30.34, '2026-04-20', '2026-04-07', NULL);
INSERT INTO bills VALUES (16, 'electricity', '2026-04', 103.82, '2026-05-20', '2026-05-15', NULL);
INSERT INTO bills VALUES (17, 'gas', '2026-04', 49.37, '2026-05-20', '2026-05-17', NULL);
INSERT INTO bills VALUES (18, 'water', '2026-04', 24.61, '2026-05-20', '2026-05-19', 'scans/water-2026-04.pdf');
INSERT INTO bills VALUES (19, 'internet', '2026-04', 39.99, '2026-05-20', '2026-05-09', NULL);
INSERT INTO bills VALUES (20, 'phone', '2026-04', 21.19, '2026-05-20', '2026-05-05', NULL);
INSERT INTO bills VALUES (21, 'electricity', '2026-05', 68.16, '2026-06-20', '2026-06-06', NULL);
INSERT INTO bills VALUES (22, 'gas', '2026-05', 42.07, '2026-06-20', '2026-06-13', NULL);
INSERT INTO bills VALUES (23, 'water', '2026-05', 35.45, '2026-06-20', '2026-06-14', NULL);
INSERT INTO bills VALUES (24, 'internet', '2026-05', 39.99, '2026-06-20', '2026-06-19', 'scans/internet-2026-05.pdf');
INSERT INTO bills VALUES (25, 'phone', '2026-05', 19.21, '2026-06-20', '2026-06-12', NULL);
INSERT INTO bills VALUES (26, 'electricity', '2026-06', 108.16, '2026-07-20', '2026-07-08', NULL);
INSERT INTO bills VALUES (27, 'gas', '2026-06', 51.74, '2026-07-20', '2026-07-18', NULL);
INSERT INTO bills VALUES (28, 'water', '2026-06', 38.58, '2026-07-20', '2026-07-06', NULL);
INSERT INTO bills VALUES (29, 'internet', '2026-06', 39.99, '2026-07-20', '2026-07-18', NULL);
INSERT INTO bills VALUES (30, 'phone', '2026-06', 22.34, '2026-07-20', '2026-07-08', 'scans/phone-2026-06.pdf');
INSERT INTO bills VALUES (31, 'electricity', '2026-07', 73.97, '2026-08-20', '2026-08-19', NULL);
INSERT INTO bills VALUES (32, 'gas', '2026-07', 38.07, '2026-08-20', '2026-08-17', NULL);
INSERT INTO bills VALUES (33, 'water', '2026-07', 28.96, '2026-08-20', '2026-08-16', NULL);
INSERT INTO bills VALUES (34, 'internet', '2026-07', 39.99, '2026-08-20', '2026-08-18', NULL);
INSERT INTO bills VALUES (35, 'phone', '2026-07', 14.02, '2026-08-20', '2026-08-07', NULL);
INSERT INTO bills VALUES (36, 'electricity', '2026-08', 95.39, '2026-09-20', '2026-09-18', 'scans/electricity-2026-08.pdf');
INSERT INTO bills VALUES (37, 'gas', '2026-08', 34.99, '2026-09-20', '2026-09-06', NULL);
INSERT INTO bills VALUES (38, 'water', '2026-08', 39.32, '2026-09-20', NULL, NULL);
INSERT INTO bills VALUES (39, 'internet', '2026-08', 39.99, '2026-09-20', '2026-09-13', NULL);
INSERT INTO bills VALUES (40, 'phone', '2026-08', 16.70, '2026-09-20', '2026-09-05', NULL);
INSERT INTO bills VALUES (41, 'internet', '2026-09', 39.99, '2026-10-20', NULL, NULL);
INSERT INTO bills VALUES (42, 'phone', '2026-09', 15.22, '2026-10-20', '2026-10-13', 'scans/phone-2026-09.pdf');
INSERT INTO readings VALUES ('electricity', '2026-01-01', 12840.0);
INSERT INTO readings VALUES ('electricity', '2026-02-01', 13158.5);
INSERT INTO readings VALUES ('electricity', '2026-03-01', 13389.6);
INSERT INTO readings VALUES ('electricity', '2026-04-01', 13629.8);
INSERT INTO readings VALUES ('electricity', '2026-05-01', 13898.1);
INSERT INTO readings VALUES ('electricity', '2026-06-01', 14177.2);
INSERT INTO readings VALUES ('electricity', '2026-07-01', 14489.3);
INSERT INTO readings VALUES ('electricity', '2026-08-01', 14782.8);
INSERT INTO readings VALUES ('electricity', '2026-09-01', 15021.5);
INSERT INTO readings VALUES ('gas', '2026-01-01', 5120.0);
INSERT INTO readings VALUES ('gas', '2026-02-01', 5422.4);
INSERT INTO readings VALUES ('gas', '2026-03-01', 5807.8);
INSERT INTO readings VALUES ('gas', '2026-04-01', 6193.1);
INSERT INTO readings VALUES ('gas', '2026-05-01', 6238.6);
INSERT INTO readings VALUES ('gas', '2026-06-01', 6384.1);
INSERT INTO readings VALUES ('gas', '2026-07-01', 6544.2);
INSERT INTO readings VALUES ('gas', '2026-08-01', 6732.1);
INSERT INTO readings VALUES ('gas', '2026-09-01', 6806.1);
INSERT INTO readings VALUES ('water', '2026-01-01', 860.0);
INSERT INTO readings VALUES ('water', '2026-02-01', 874.6);
INSERT INTO readings VALUES ('water', '2026-03-01', 885.7);
INSERT INTO readings VALUES ('water', '2026-04-01', 895.7);
INSERT INTO readings VALUES ('water', '2026-05-01', 909.1);
INSERT INTO readings VALUES ('water', '2026-06-01', 918.6);
INSERT INTO readings VALUES ('water', '2026-07-01', 930.5);
INSERT INTO readings VALUES ('water', '2026-08-01', 945.0);
INSERT INTO readings VALUES ('water', '2026-09-01', 959.1);
