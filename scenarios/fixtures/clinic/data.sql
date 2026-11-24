-- SPIKE-046: a synthetic fixture, written by spikes/046-sql-lora/gen from a fixed seed.
-- A small clinic: doctors, patients and appointments, September and
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
INSERT INTO doctors VALUES (1, 'DR-01', 'general', '1A');
INSERT INTO doctors VALUES (2, 'DR-02', 'dermatology', '2B');
INSERT INTO doctors VALUES (3, 'DR-03', 'cardiology', '3A');
INSERT INTO doctors VALUES (4, 'DR-04', 'pediatrics', '1C');
INSERT INTO doctors VALUES (5, 'DR-05', 'dentistry', '2A');
INSERT INTO doctors VALUES (6, 'DR-06', 'general', '1B');
INSERT INTO patients VALUES (1, 'P-001', 1987, 'Berlin');
INSERT INTO patients VALUES (2, 'P-002', 1986, 'Potsdam');
INSERT INTO patients VALUES (3, 'P-003', 2019, 'Hamburg');
INSERT INTO patients VALUES (4, 'P-004', 1970, 'Potsdam');
INSERT INTO patients VALUES (5, 'P-005', 1975, 'Hamburg');
INSERT INTO patients VALUES (6, 'P-006', 1972, 'Potsdam');
INSERT INTO patients VALUES (7, 'P-007', 1960, 'Berlin');
INSERT INTO patients VALUES (8, 'P-008', 1963, 'Berlin');
INSERT INTO patients VALUES (9, 'P-009', 1989, 'Berlin');
INSERT INTO patients VALUES (10, 'P-010', 2002, 'Hamburg');
INSERT INTO patients VALUES (11, 'P-011', 1989, 'Berlin');
INSERT INTO patients VALUES (12, 'P-012', 1987, 'Potsdam');
INSERT INTO patients VALUES (13, 'P-013', 2006, 'Berlin');
INSERT INTO patients VALUES (14, 'P-014', 2007, 'Potsdam');
INSERT INTO patients VALUES (15, 'P-015', 1992, 'Berlin');
INSERT INTO patients VALUES (16, 'P-016', 1983, 'Potsdam');
INSERT INTO patients VALUES (17, 'P-017', 1983, 'Berlin');
INSERT INTO patients VALUES (18, 'P-018', 1999, 'Hamburg');
INSERT INTO patients VALUES (19, 'P-019', 2015, 'Berlin');
INSERT INTO patients VALUES (20, 'P-020', 1991, 'Hamburg');
INSERT INTO patients VALUES (21, 'P-021', 1953, 'Hamburg');
INSERT INTO patients VALUES (22, 'P-022', 1970, 'Potsdam');
INSERT INTO patients VALUES (23, 'P-023', 1952, 'Hamburg');
INSERT INTO patients VALUES (24, 'P-024', 2000, 'Berlin');
INSERT INTO patients VALUES (25, 'P-025', 1967, 'Potsdam');
INSERT INTO patients VALUES (26, 'P-026', 1956, 'Potsdam');
INSERT INTO patients VALUES (27, 'P-027', 1972, 'Potsdam');
INSERT INTO patients VALUES (28, 'P-028', 1974, 'Berlin');
INSERT INTO patients VALUES (29, 'P-029', 1973, 'Potsdam');
INSERT INTO patients VALUES (30, 'P-030', 1980, 'Berlin');
INSERT INTO appointments VALUES (1, 11, 3, '2026-09-01', '13:30', 'cancelled', 120, NULL);
INSERT INTO appointments VALUES (2, 5, 5, '2026-09-01', '08:30', 'done', 85, NULL);
INSERT INTO appointments VALUES (3, 4, 4, '2026-09-02', '10:00', 'done', 70, NULL);
INSERT INTO appointments VALUES (4, 29, 1, '2026-09-02', '14:00', 'done', 60, NULL);
INSERT INTO appointments VALUES (5, 3, 3, '2026-09-02', '09:30', 'done', 120, NULL);
INSERT INTO appointments VALUES (6, 6, 6, '2026-09-02', '13:30', 'cancelled', 60, NULL);
INSERT INTO appointments VALUES (7, 17, 6, '2026-09-03', '10:30', 'done', 60, NULL);
INSERT INTO appointments VALUES (8, 30, 4, '2026-09-03', '14:30', 'done', 70, NULL);
INSERT INTO appointments VALUES (9, 3, 4, '2026-09-03', '08:30', 'no_show', 70, 'letters/appointment-9.pdf');
INSERT INTO appointments VALUES (10, 13, 1, '2026-09-04', '09:00', 'no_show', 60, NULL);
INSERT INTO appointments VALUES (11, 5, 1, '2026-09-04', '10:00', 'no_show', 60, NULL);
INSERT INTO appointments VALUES (12, 10, 2, '2026-09-04', '11:00', 'done', 95, NULL);
INSERT INTO appointments VALUES (13, 24, 5, '2026-09-07', '10:30', 'no_show', 85, NULL);
INSERT INTO appointments VALUES (14, 5, 3, '2026-09-07', '14:30', 'done', 120, NULL);
INSERT INTO appointments VALUES (15, 5, 4, '2026-09-07', '09:00', 'done', 70, NULL);
INSERT INTO appointments VALUES (16, 5, 6, '2026-09-08', '14:30', 'cancelled', 60, NULL);
INSERT INTO appointments VALUES (17, 10, 4, '2026-09-08', '08:30', 'done', 70, NULL);
INSERT INTO appointments VALUES (18, 11, 5, '2026-09-09', '15:00', 'cancelled', 85, 'letters/appointment-18.pdf');
INSERT INTO appointments VALUES (19, 10, 3, '2026-09-09', '10:30', 'done', 120, NULL);
INSERT INTO appointments VALUES (20, 4, 5, '2026-09-09', '14:30', 'no_show', 85, NULL);
INSERT INTO appointments VALUES (21, 3, 1, '2026-09-10', '10:30', 'done', 60, NULL);
INSERT INTO appointments VALUES (22, 13, 5, '2026-09-10', '09:30', 'done', 85, NULL);
INSERT INTO appointments VALUES (23, 9, 3, '2026-09-10', '09:00', 'done', 120, NULL);
INSERT INTO appointments VALUES (24, 17, 2, '2026-09-11', '15:30', 'cancelled', 95, NULL);
INSERT INTO appointments VALUES (25, 22, 5, '2026-09-11', '13:30', 'done', 85, NULL);
INSERT INTO appointments VALUES (26, 15, 2, '2026-09-14', '14:00', 'done', 95, NULL);
INSERT INTO appointments VALUES (27, 14, 1, '2026-09-14', '10:00', 'done', 60, 'letters/appointment-27.pdf');
INSERT INTO appointments VALUES (28, 25, 3, '2026-09-15', '13:30', 'done', 120, NULL);
INSERT INTO appointments VALUES (29, 20, 2, '2026-09-15', '09:00', 'done', 95, NULL);
INSERT INTO appointments VALUES (30, 8, 5, '2026-09-15', '14:30', 'done', 85, NULL);
INSERT INTO appointments VALUES (31, 18, 2, '2026-09-15', '09:30', 'no_show', 95, NULL);
INSERT INTO appointments VALUES (32, 17, 5, '2026-09-16', '11:00', 'cancelled', 85, NULL);
INSERT INTO appointments VALUES (33, 14, 2, '2026-09-16', '15:30', 'done', 95, NULL);
INSERT INTO appointments VALUES (34, 1, 1, '2026-09-17', '15:30', 'done', 60, NULL);
INSERT INTO appointments VALUES (35, 17, 6, '2026-09-17', '11:00', 'done', 60, NULL);
INSERT INTO appointments VALUES (36, 1, 6, '2026-09-17', '10:00', 'done', 60, 'letters/appointment-36.pdf');
INSERT INTO appointments VALUES (37, 10, 3, '2026-09-17', '13:30', 'no_show', 120, NULL);
INSERT INTO appointments VALUES (38, 19, 2, '2026-09-18', '14:00', 'done', 95, NULL);
INSERT INTO appointments VALUES (39, 30, 5, '2026-09-18', '09:00', 'cancelled', 85, NULL);
INSERT INTO appointments VALUES (40, 26, 6, '2026-09-21', '08:30', 'done', 60, NULL);
INSERT INTO appointments VALUES (41, 28, 2, '2026-09-21', '14:00', 'done', 95, NULL);
INSERT INTO appointments VALUES (42, 10, 2, '2026-09-21', '11:00', 'cancelled', 95, NULL);
INSERT INTO appointments VALUES (43, 26, 1, '2026-09-22', '09:00', 'done', 60, NULL);
INSERT INTO appointments VALUES (44, 13, 2, '2026-09-22', '14:00', 'done', 95, NULL);
INSERT INTO appointments VALUES (45, 8, 5, '2026-09-22', '15:30', 'done', 85, 'letters/appointment-45.pdf');
INSERT INTO appointments VALUES (46, 22, 5, '2026-09-22', '10:30', 'done', 85, NULL);
INSERT INTO appointments VALUES (47, 10, 4, '2026-09-23', '09:00', 'cancelled', 70, NULL);
INSERT INTO appointments VALUES (48, 14, 1, '2026-09-23', '08:30', 'cancelled', 60, NULL);
INSERT INTO appointments VALUES (49, 20, 6, '2026-09-24', '13:30', 'done', 60, NULL);
INSERT INTO appointments VALUES (50, 29, 3, '2026-09-24', '09:30', 'done', 120, NULL);
INSERT INTO appointments VALUES (51, 25, 5, '2026-09-24', '10:30', 'done', 85, NULL);
INSERT INTO appointments VALUES (52, 28, 3, '2026-09-25', '15:00', 'done', 120, NULL);
INSERT INTO appointments VALUES (53, 19, 5, '2026-09-25', '13:30', 'done', 85, NULL);
INSERT INTO appointments VALUES (54, 17, 6, '2026-09-25', '11:00', 'done', 60, 'letters/appointment-54.pdf');
INSERT INTO appointments VALUES (55, 23, 3, '2026-09-25', '15:30', 'done', 120, NULL);
INSERT INTO appointments VALUES (56, 2, 1, '2026-09-28', '09:30', 'no_show', 60, NULL);
INSERT INTO appointments VALUES (57, 14, 2, '2026-09-28', '14:00', 'done', 95, NULL);
INSERT INTO appointments VALUES (58, 26, 5, '2026-09-29', '14:30', 'done', 85, NULL);
INSERT INTO appointments VALUES (59, 17, 4, '2026-09-29', '13:30', 'done', 70, NULL);
INSERT INTO appointments VALUES (60, 11, 5, '2026-09-29', '11:00', 'done', 85, NULL);
INSERT INTO appointments VALUES (61, 28, 4, '2026-09-30', '15:00', 'done', 70, NULL);
INSERT INTO appointments VALUES (62, 13, 1, '2026-09-30', '10:30', 'cancelled', 60, NULL);
INSERT INTO appointments VALUES (63, 20, 5, '2026-09-30', '10:00', 'done', 85, 'letters/appointment-63.pdf');
INSERT INTO appointments VALUES (64, 4, 5, '2026-09-30', '15:30', 'done', 85, NULL);
INSERT INTO appointments VALUES (65, 25, 3, '2026-10-01', '10:30', 'booked', 120, NULL);
INSERT INTO appointments VALUES (66, 6, 2, '2026-10-02', '09:00', 'booked', 95, NULL);
INSERT INTO appointments VALUES (67, 17, 5, '2026-10-02', '10:00', 'booked', 85, NULL);
INSERT INTO appointments VALUES (68, 9, 3, '2026-10-05', '09:00', 'booked', 120, NULL);
INSERT INTO appointments VALUES (69, 27, 1, '2026-10-06', '15:00', 'booked', 60, NULL);
INSERT INTO appointments VALUES (70, 18, 6, '2026-10-07', '11:00', 'booked', 60, NULL);
INSERT INTO appointments VALUES (71, 3, 3, '2026-10-07', '15:00', 'booked', 120, NULL);
INSERT INTO appointments VALUES (72, 3, 6, '2026-10-08', '09:30', 'booked', 60, 'letters/appointment-72.pdf');
INSERT INTO appointments VALUES (73, 28, 2, '2026-10-08', '14:30', 'booked', 95, NULL);
INSERT INTO appointments VALUES (74, 12, 4, '2026-10-08', '11:00', 'booked', 70, NULL);
INSERT INTO appointments VALUES (75, 9, 1, '2026-10-09', '10:00', 'booked', 60, NULL);
INSERT INTO appointments VALUES (76, 25, 3, '2026-10-12', '11:00', 'booked', 120, NULL);
INSERT INTO appointments VALUES (77, 26, 4, '2026-10-12', '09:30', 'booked', 70, NULL);
INSERT INTO appointments VALUES (78, 5, 4, '2026-10-12', '13:30', 'booked', 70, NULL);
INSERT INTO appointments VALUES (79, 16, 3, '2026-10-13', '11:00', 'booked', 120, NULL);
INSERT INTO appointments VALUES (80, 22, 3, '2026-10-13', '10:00', 'booked', 120, NULL);
INSERT INTO appointments VALUES (81, 1, 1, '2026-10-13', '14:30', 'booked', 60, 'letters/appointment-81.pdf');
INSERT INTO appointments VALUES (82, 25, 5, '2026-10-14', '13:30', 'booked', 85, NULL);
INSERT INTO appointments VALUES (83, 14, 5, '2026-10-15', '10:30', 'booked', 85, NULL);
INSERT INTO appointments VALUES (84, 23, 4, '2026-10-16', '13:30', 'booked', 70, NULL);
