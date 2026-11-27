-- application_id marks the file as this store's, so a foreign database is refused before it is opened.
PRAGMA application_id = 1363085617;

-- Every row is keyed by its UTC time in unix seconds, so a repeated time can never replace a stored row.
CREATE TABLE raw (
	ts INTEGER PRIMARY KEY,
	cpu_percent REAL,
	load_one REAL,
	memory_used INTEGER,
	swap_used INTEGER
);
CREATE TABLE raw_fs (
	ts INTEGER NOT NULL,
	mount TEXT NOT NULL,
	used INTEGER NOT NULL,
	total INTEGER NOT NULL,
	PRIMARY KEY (ts, mount)
) WITHOUT ROWID;

CREATE TABLE minute (
	ts INTEGER PRIMARY KEY,
	cpu_percent_avg REAL,
	cpu_percent_max REAL,
	load_one_avg REAL,
	load_one_max REAL,
	memory_used_avg REAL,
	memory_used_max INTEGER,
	swap_used_avg REAL,
	swap_used_max INTEGER
);
CREATE TABLE minute_fs (
	ts INTEGER NOT NULL,
	mount TEXT NOT NULL,
	used_avg REAL NOT NULL,
	used_max INTEGER NOT NULL,
	total INTEGER NOT NULL,
	PRIMARY KEY (ts, mount)
) WITHOUT ROWID;

CREATE TABLE hour (
	ts INTEGER PRIMARY KEY,
	cpu_percent_avg REAL,
	cpu_percent_max REAL,
	load_one_avg REAL,
	load_one_max REAL,
	memory_used_avg REAL,
	memory_used_max INTEGER,
	swap_used_avg REAL,
	swap_used_max INTEGER
);
CREATE TABLE hour_fs (
	ts INTEGER NOT NULL,
	mount TEXT NOT NULL,
	used_avg REAL NOT NULL,
	used_max INTEGER NOT NULL,
	total INTEGER NOT NULL,
	PRIMARY KEY (ts, mount)
) WITHOUT ROWID;
