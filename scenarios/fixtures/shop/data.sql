-- SPIKE-046: a synthetic fixture, written by spikes/046-sql-lora/gen from a fixed seed.
-- A small online shop: products, customers, orders and their items,
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
INSERT INTO products VALUES (1, 'Pocket notebook', 'office', 4.5, 'photos/product-1.jpg');
INSERT INTO products VALUES (2, 'Gel pens (10)', 'office', 7.9, NULL);
INSERT INTO products VALUES (3, 'Desk organizer', 'office', 24, NULL);
INSERT INTO products VALUES (4, 'Chef knife', 'kitchen', 39.9, NULL);
INSERT INTO products VALUES (5, 'Cast iron pan', 'kitchen', 54, 'photos/product-5.jpg');
INSERT INTO products VALUES (6, 'Tea kettle', 'kitchen', 32.5, NULL);
INSERT INTO products VALUES (7, 'Pruning shears', 'garden', 18.75, NULL);
INSERT INTO products VALUES (8, 'Watering can', 'garden', 15, NULL);
INSERT INTO products VALUES (9, 'Seed starter kit', 'garden', 22.4, 'photos/product-9.jpg');
INSERT INTO products VALUES (10, 'Wooden puzzle', 'toys', 12.9, NULL);
INSERT INTO products VALUES (11, 'Kite', 'toys', 19.5, NULL);
INSERT INTO products VALUES (12, 'Card game', 'toys', 9.99, NULL);
INSERT INTO products VALUES (13, 'Cookbook: Soups', 'books', 21, 'photos/product-13.jpg');
INSERT INTO products VALUES (14, 'Field guide to birds', 'books', 27.5, NULL);
INSERT INTO customers VALUES (1, 'C-001', 'Berlin', '2025-11-07');
INSERT INTO customers VALUES (2, 'C-002', 'Hamburg', '2026-05-19');
INSERT INTO customers VALUES (3, 'C-003', 'Cologne', '2025-05-10');
INSERT INTO customers VALUES (4, 'C-004', 'Hamburg', '2026-01-29');
INSERT INTO customers VALUES (5, 'C-005', 'Berlin', '2026-06-10');
INSERT INTO customers VALUES (6, 'C-006', 'Leipzig', '2026-07-25');
INSERT INTO customers VALUES (7, 'C-007', 'Hamburg', '2025-11-08');
INSERT INTO customers VALUES (8, 'C-008', 'Munich', '2026-01-01');
INSERT INTO customers VALUES (9, 'C-009', 'Cologne', '2026-03-10');
INSERT INTO customers VALUES (10, 'C-010', 'Cologne', '2025-04-26');
INSERT INTO customers VALUES (11, 'C-011', 'Munich', '2025-05-24');
INSERT INTO customers VALUES (12, 'C-012', 'Cologne', '2025-12-02');
INSERT INTO orders VALUES (1, 3, '2026-08-01', 'shipped');
INSERT INTO order_items VALUES (1, 9, 3, 22.4);
INSERT INTO orders VALUES (2, 10, '2026-08-05', 'shipped');
INSERT INTO order_items VALUES (2, 6, 3, 32.5);
INSERT INTO orders VALUES (3, 3, '2026-08-05', 'shipped');
INSERT INTO order_items VALUES (3, 6, 2, 32.5);
INSERT INTO order_items VALUES (3, 12, 3, 9.99);
INSERT INTO orders VALUES (4, 6, '2026-08-10', 'shipped');
INSERT INTO order_items VALUES (4, 12, 3, 9.99);
INSERT INTO orders VALUES (5, 11, '2026-08-11', 'shipped');
INSERT INTO order_items VALUES (5, 5, 1, 54);
INSERT INTO order_items VALUES (5, 13, 3, 21);
INSERT INTO order_items VALUES (5, 8, 2, 15);
INSERT INTO orders VALUES (6, 8, '2026-08-12', 'returned');
INSERT INTO order_items VALUES (6, 10, 1, 12.9);
INSERT INTO order_items VALUES (6, 13, 1, 21);
INSERT INTO orders VALUES (7, 8, '2026-08-14', 'shipped');
INSERT INTO order_items VALUES (7, 3, 2, 24);
INSERT INTO order_items VALUES (7, 11, 2, 19.5);
INSERT INTO orders VALUES (8, 9, '2026-08-16', 'shipped');
INSERT INTO order_items VALUES (8, 7, 2, 18.75);
INSERT INTO orders VALUES (9, 4, '2026-08-16', 'shipped');
INSERT INTO order_items VALUES (9, 7, 2, 18.75);
INSERT INTO order_items VALUES (9, 3, 3, 24);
INSERT INTO order_items VALUES (9, 5, 1, 54);
INSERT INTO orders VALUES (10, 8, '2026-08-17', 'shipped');
INSERT INTO order_items VALUES (10, 1, 2, 4.5);
INSERT INTO order_items VALUES (10, 3, 2, 24);
INSERT INTO order_items VALUES (10, 10, 2, 12.9);
INSERT INTO orders VALUES (11, 6, '2026-08-17', 'shipped');
INSERT INTO order_items VALUES (11, 14, 1, 27.5);
INSERT INTO order_items VALUES (11, 2, 2, 7.9);
INSERT INTO orders VALUES (12, 6, '2026-08-18', 'returned');
INSERT INTO order_items VALUES (12, 8, 2, 15);
INSERT INTO order_items VALUES (12, 12, 3, 9.99);
INSERT INTO orders VALUES (13, 1, '2026-08-19', 'shipped');
INSERT INTO order_items VALUES (13, 4, 2, 39.9);
INSERT INTO order_items VALUES (13, 3, 1, 24);
INSERT INTO order_items VALUES (13, 5, 2, 54);
INSERT INTO orders VALUES (14, 1, '2026-08-20', 'shipped');
INSERT INTO order_items VALUES (14, 10, 3, 12.9);
INSERT INTO orders VALUES (15, 9, '2026-08-21', 'shipped');
INSERT INTO order_items VALUES (15, 6, 1, 32.5);
INSERT INTO orders VALUES (16, 6, '2026-08-22', 'shipped');
INSERT INTO order_items VALUES (16, 7, 2, 18.75);
INSERT INTO orders VALUES (17, 4, '2026-08-24', 'shipped');
INSERT INTO order_items VALUES (17, 9, 3, 22.4);
INSERT INTO order_items VALUES (17, 10, 1, 12.9);
INSERT INTO order_items VALUES (17, 1, 3, 4.5);
INSERT INTO orders VALUES (18, 12, '2026-08-25', 'shipped');
INSERT INTO order_items VALUES (18, 11, 1, 19.5);
INSERT INTO order_items VALUES (18, 1, 3, 4.5);
INSERT INTO orders VALUES (19, 3, '2026-08-28', 'shipped');
INSERT INTO order_items VALUES (19, 1, 3, 4.5);
INSERT INTO orders VALUES (20, 8, '2026-08-29', 'shipped');
INSERT INTO order_items VALUES (20, 9, 2, 22.4);
INSERT INTO order_items VALUES (20, 11, 2, 19.5);
INSERT INTO orders VALUES (21, 12, '2026-08-30', 'shipped');
INSERT INTO order_items VALUES (21, 5, 3, 54);
INSERT INTO order_items VALUES (21, 3, 2, 24);
INSERT INTO orders VALUES (22, 4, '2026-08-31', 'shipped');
INSERT INTO order_items VALUES (22, 3, 2, 24);
INSERT INTO orders VALUES (23, 4, '2026-09-03', 'shipped');
INSERT INTO order_items VALUES (23, 7, 2, 18.75);
INSERT INTO orders VALUES (24, 6, '2026-09-04', 'returned');
INSERT INTO order_items VALUES (24, 1, 1, 4.5);
INSERT INTO order_items VALUES (24, 8, 1, 15);
INSERT INTO orders VALUES (25, 8, '2026-09-04', 'shipped');
INSERT INTO order_items VALUES (25, 9, 2, 22.4);
INSERT INTO order_items VALUES (25, 2, 2, 7.9);
INSERT INTO order_items VALUES (25, 14, 1, 27.5);
INSERT INTO orders VALUES (26, 1, '2026-09-05', 'cancelled');
INSERT INTO order_items VALUES (26, 13, 1, 21);
INSERT INTO orders VALUES (27, 5, '2026-09-05', 'shipped');
INSERT INTO order_items VALUES (27, 13, 2, 21);
INSERT INTO order_items VALUES (27, 3, 3, 24);
INSERT INTO order_items VALUES (27, 14, 3, 27.5);
INSERT INTO orders VALUES (28, 10, '2026-09-08', 'shipped');
INSERT INTO order_items VALUES (28, 4, 1, 39.9);
INSERT INTO orders VALUES (29, 3, '2026-09-08', 'shipped');
INSERT INTO order_items VALUES (29, 6, 2, 32.5);
INSERT INTO order_items VALUES (29, 1, 2, 4.5);
INSERT INTO order_items VALUES (29, 8, 2, 15);
INSERT INTO orders VALUES (30, 10, '2026-09-08', 'shipped');
INSERT INTO order_items VALUES (30, 10, 1, 12.9);
INSERT INTO order_items VALUES (30, 8, 3, 15);
INSERT INTO orders VALUES (31, 12, '2026-09-09', 'shipped');
INSERT INTO order_items VALUES (31, 14, 3, 27.5);
INSERT INTO order_items VALUES (31, 7, 3, 18.75);
INSERT INTO orders VALUES (32, 4, '2026-09-11', 'cancelled');
INSERT INTO order_items VALUES (32, 4, 2, 39.9);
INSERT INTO order_items VALUES (32, 7, 1, 18.75);
INSERT INTO orders VALUES (33, 3, '2026-09-11', 'cancelled');
INSERT INTO order_items VALUES (33, 5, 3, 54);
INSERT INTO orders VALUES (34, 1, '2026-09-12', 'cancelled');
INSERT INTO order_items VALUES (34, 12, 1, 9.99);
INSERT INTO order_items VALUES (34, 10, 2, 12.9);
INSERT INTO order_items VALUES (34, 13, 1, 21);
INSERT INTO orders VALUES (35, 8, '2026-09-14', 'shipped');
INSERT INTO order_items VALUES (35, 12, 1, 9.99);
INSERT INTO orders VALUES (36, 5, '2026-09-15', 'shipped');
INSERT INTO order_items VALUES (36, 2, 1, 7.9);
INSERT INTO orders VALUES (37, 4, '2026-09-15', 'shipped');
INSERT INTO order_items VALUES (37, 5, 2, 54);
INSERT INTO orders VALUES (38, 4, '2026-09-18', 'returned');
INSERT INTO order_items VALUES (38, 2, 3, 7.9);
INSERT INTO order_items VALUES (38, 6, 1, 32.5);
INSERT INTO order_items VALUES (38, 4, 1, 39.9);
INSERT INTO orders VALUES (39, 12, '2026-09-21', 'shipped');
INSERT INTO order_items VALUES (39, 4, 1, 39.9);
INSERT INTO order_items VALUES (39, 11, 3, 19.5);
INSERT INTO order_items VALUES (39, 7, 2, 18.75);
INSERT INTO orders VALUES (40, 2, '2026-09-21', 'returned');
INSERT INTO order_items VALUES (40, 13, 3, 21);
INSERT INTO orders VALUES (41, 3, '2026-09-22', 'shipped');
INSERT INTO order_items VALUES (41, 8, 3, 15);
INSERT INTO order_items VALUES (41, 2, 1, 7.9);
INSERT INTO orders VALUES (42, 4, '2026-09-23', 'shipped');
INSERT INTO order_items VALUES (42, 5, 2, 54);
INSERT INTO order_items VALUES (42, 14, 2, 27.5);
INSERT INTO order_items VALUES (42, 8, 2, 15);
INSERT INTO orders VALUES (43, 1, '2026-09-24', 'paid');
INSERT INTO order_items VALUES (43, 3, 2, 24);
INSERT INTO order_items VALUES (43, 9, 2, 22.4);
INSERT INTO order_items VALUES (43, 2, 2, 7.9);
INSERT INTO orders VALUES (44, 11, '2026-09-26', 'paid');
INSERT INTO order_items VALUES (44, 9, 1, 22.4);
INSERT INTO orders VALUES (45, 3, '2026-09-26', 'paid');
INSERT INTO order_items VALUES (45, 1, 3, 4.5);
INSERT INTO order_items VALUES (45, 14, 1, 27.5);
INSERT INTO order_items VALUES (45, 3, 1, 24);
INSERT INTO orders VALUES (46, 1, '2026-09-28', 'paid');
INSERT INTO order_items VALUES (46, 2, 2, 7.9);
INSERT INTO orders VALUES (47, 7, '2026-09-30', 'shipped');
INSERT INTO order_items VALUES (47, 8, 1, 15);
INSERT INTO orders VALUES (48, 11, '2026-09-30', 'shipped');
INSERT INTO order_items VALUES (48, 12, 2, 9.99);
