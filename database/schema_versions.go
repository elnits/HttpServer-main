package database

import (
	"database/sql"
	"fmt"
	"strings"
)

// CreateNormalizationVersionsTables создает таблицы для многостадийного версионирования нормализации
func CreateNormalizationVersionsTables(db *sql.DB) error {
	// Таблица сессий нормализации
	sessionsTable := `
		CREATE TABLE IF NOT EXISTS normalization_sessions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			catalog_item_id INTEGER NOT NULL,
			original_name TEXT NOT NULL,
			current_name TEXT NOT NULL,
			stages_count INTEGER DEFAULT 0,
			status TEXT DEFAULT 'in_progress',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY(catalog_item_id) REFERENCES catalog_items(id) ON DELETE CASCADE
		)
	`

	// Таблица стадий нормализации
	stagesTable := `
		CREATE TABLE IF NOT EXISTS normalization_stages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id INTEGER NOT NULL,
			stage_type TEXT NOT NULL,
			stage_name TEXT NOT NULL,
			input_name TEXT NOT NULL,
			output_name TEXT NOT NULL,
			applied_patterns TEXT,
			ai_context TEXT,
			category_original TEXT,
			category_folded TEXT,
			classification_strategy TEXT,
			confidence REAL DEFAULT 0.0,
			status TEXT DEFAULT 'applied',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY(session_id) REFERENCES normalization_sessions(id) ON DELETE CASCADE
		)
	`

	// Создаем таблицы
	tables := []string{
		sessionsTable,
		stagesTable,
	}

	for _, tableSQL := range tables {
		_, err := db.Exec(tableSQL)
		if err != nil {
			return fmt.Errorf("failed to create normalization versions table: %w", err)
		}
	}

	// Создаем индексы
	indexes := []string{
		`CREATE INDEX IF NOT EXISTS idx_norm_sessions_item_id ON normalization_sessions(catalog_item_id)`,
		`CREATE INDEX IF NOT EXISTS idx_norm_sessions_status ON normalization_sessions(status)`,
		`CREATE INDEX IF NOT EXISTS idx_norm_stages_session_id ON normalization_stages(session_id)`,
		`CREATE INDEX IF NOT EXISTS idx_norm_stages_type ON normalization_stages(stage_type)`,
		`CREATE INDEX IF NOT EXISTS idx_norm_stages_status ON normalization_stages(status)`,
	}

	for _, indexSQL := range indexes {
		_, err := db.Exec(indexSQL)
		if err != nil {
			return fmt.Errorf("failed to create normalization versions index: %w", err)
		}
	}

	return nil
}

// CreateClassificationTables создает таблицы для системы классификации
func CreateClassificationTables(db *sql.DB) error {
	// Таблица классификаторов категорий
	classifiersTable := `
		CREATE TABLE IF NOT EXISTS category_classifiers (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			description TEXT,
			max_depth INTEGER DEFAULT 6,
			tree_structure TEXT NOT NULL,
			client_id INTEGER,
			project_id INTEGER,
			is_active BOOLEAN DEFAULT TRUE,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`

	// Таблица стратегий свертки
	strategiesTable := `
		CREATE TABLE IF NOT EXISTS folding_strategies (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			description TEXT,
			strategy_config TEXT NOT NULL,
			client_id INTEGER,
			is_default BOOLEAN DEFAULT FALSE,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`

	// Создаем таблицы
	tables := []string{
		classifiersTable,
		strategiesTable,
	}

	for _, tableSQL := range tables {
		_, err := db.Exec(tableSQL)
		if err != nil {
			return fmt.Errorf("failed to create classification table: %w", err)
		}
	}

	// Создаем индексы
	indexes := []string{
		`CREATE INDEX IF NOT EXISTS idx_classifiers_active ON category_classifiers(is_active)`,
		`CREATE INDEX IF NOT EXISTS idx_classifiers_client_id ON category_classifiers(client_id)`,
		`CREATE INDEX IF NOT EXISTS idx_classifiers_project_id ON category_classifiers(project_id)`,
		`CREATE INDEX IF NOT EXISTS idx_strategies_client_id ON folding_strategies(client_id)`,
		`CREATE INDEX IF NOT EXISTS idx_strategies_default ON folding_strategies(is_default)`,
	}

	for _, indexSQL := range indexes {
		_, err := db.Exec(indexSQL)
		if err != nil {
			return fmt.Errorf("failed to create classification index: %w", err)
		}
	}

	// Добавляем поля client_id и project_id в category_classifiers если их нет
	classifierMigrations := []string{
		`ALTER TABLE category_classifiers ADD COLUMN client_id INTEGER`,
		`ALTER TABLE category_classifiers ADD COLUMN project_id INTEGER`,
	}

	for _, migration := range classifierMigrations {
		_, err := db.Exec(migration)
		if err != nil {
			errStr := strings.ToLower(err.Error())
			if !strings.Contains(errStr, "duplicate column") && !strings.Contains(errStr, "already exists") {
				// Игнорируем ошибки если колонки уже существуют
			}
		}
	}

	// Расширяем таблицу catalog_items полями категорий
	migrations := []string{
		`ALTER TABLE catalog_items ADD COLUMN category_original TEXT`,
		`ALTER TABLE catalog_items ADD COLUMN category_level1 TEXT`,
		`ALTER TABLE catalog_items ADD COLUMN category_level2 TEXT`,
		`ALTER TABLE catalog_items ADD COLUMN category_level3 TEXT`,
		`ALTER TABLE catalog_items ADD COLUMN category_level4 TEXT`,
		`ALTER TABLE catalog_items ADD COLUMN category_level5 TEXT`,
		`ALTER TABLE catalog_items ADD COLUMN classification_confidence REAL DEFAULT 0.0`,
		`ALTER TABLE catalog_items ADD COLUMN classification_strategy TEXT`,
	}

	for _, migration := range migrations {
		_, err := db.Exec(migration)
		if err != nil {
			errStr := strings.ToLower(err.Error())
			if !strings.Contains(errStr, "duplicate column") &&
				!strings.Contains(errStr, "already exists") {
				return fmt.Errorf("failed to migrate catalog_items for classification: %w", err)
			}
		}
	}

	// Создаем индексы для категорий в catalog_items
	categoryIndexes := []string{
		`CREATE INDEX IF NOT EXISTS idx_catalog_items_category_level1 ON catalog_items(category_level1)`,
		`CREATE INDEX IF NOT EXISTS idx_catalog_items_category_level2 ON catalog_items(category_level2)`,
		`CREATE INDEX IF NOT EXISTS idx_catalog_items_classification_strategy ON catalog_items(classification_strategy)`,
	}

	for _, indexSQL := range categoryIndexes {
		_, err := db.Exec(indexSQL)
		if err != nil {
			return fmt.Errorf("failed to create catalog_items category index: %w", err)
		}
	}

	return nil
}

