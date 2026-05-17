CREATE TABLE IF NOT EXISTS users (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  name       TEXT NOT NULL,
  avatar     TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT (date('now'))
);

CREATE TABLE IF NOT EXISTS foods (
  id                INTEGER PRIMARY KEY AUTOINCREMENT,
  name              TEXT NOT NULL,
  unit              TEXT NOT NULL DEFAULT 'g',
  calories_per_unit REAL NOT NULL,
  created_at        TEXT NOT NULL DEFAULT (date('now'))
);

CREATE TABLE IF NOT EXISTS log_entries (
  id                INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id           INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  food_id           INTEGER REFERENCES foods(id) ON DELETE SET NULL,
  date              TEXT NOT NULL,
  food_name         TEXT NOT NULL,
  food_unit         TEXT NOT NULL,
  calories_per_unit REAL NOT NULL,
  quantity          REAL NOT NULL,
  calories          REAL NOT NULL
);

CREATE TABLE IF NOT EXISTS daily_metrics (
  id              INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id         INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  date            TEXT NOT NULL,
  weight          REAL,
  steps           INTEGER,
  target_calories INTEGER,
  UNIQUE(user_id, date)
);

CREATE INDEX IF NOT EXISTS idx_log_user_date     ON log_entries(user_id, date);
CREATE INDEX IF NOT EXISTS idx_metrics_user_date ON daily_metrics(user_id, date);
