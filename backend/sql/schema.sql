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
  id                  INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id             INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  food_id             INTEGER REFERENCES foods(id) ON DELETE SET NULL,
  date                TEXT NOT NULL,
  food_name           TEXT NOT NULL,
  food_unit           TEXT NOT NULL,
  calories_per_unit   REAL NOT NULL,
  quantity            REAL NOT NULL,
  calories            REAL NOT NULL,
  source_recipe_id    INTEGER,
  source_recipe_name  TEXT
);

CREATE TABLE IF NOT EXISTS recipes (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  name       TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT (date('now'))
);

CREATE TABLE IF NOT EXISTS recipe_ingredients (
  id        INTEGER PRIMARY KEY AUTOINCREMENT,
  recipe_id INTEGER NOT NULL REFERENCES recipes(id) ON DELETE CASCADE,
  food_id   INTEGER NOT NULL REFERENCES foods(id)   ON DELETE RESTRICT,
  quantity  REAL    NOT NULL CHECK(quantity > 0)
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

CREATE INDEX IF NOT EXISTS idx_log_user_date            ON log_entries(user_id, date);
CREATE INDEX IF NOT EXISTS idx_log_source_recipe        ON log_entries(source_recipe_id);
CREATE INDEX IF NOT EXISTS idx_metrics_user_date        ON daily_metrics(user_id, date);
CREATE INDEX IF NOT EXISTS idx_recipe_ingredients_recipe ON recipe_ingredients(recipe_id);
