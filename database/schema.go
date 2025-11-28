package database

import (
	"database/sql"
	"fmt"
	"strings"
)

// InitSchema создает все необходимые таблицы в SQLite базе данных
func InitSchema(db *sql.DB) error {
	schema := `
	-- Таблица выгрузок
	CREATE TABLE IF NOT EXISTS uploads (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		upload_uuid TEXT UNIQUE NOT NULL,
		started_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		completed_at TIMESTAMP,
		status TEXT DEFAULT 'in_progress',
		version_1c TEXT,
		config_name TEXT,
		total_constants INTEGER DEFAULT 0,
		total_catalogs INTEGER DEFAULT 0,
		total_items INTEGER DEFAULT 0,
		database_id INTEGER,
		client_id INTEGER,
		project_id INTEGER,
		computer_name TEXT,
		user_name TEXT,
		config_version TEXT
	);

	-- Таблица констант
	CREATE TABLE IF NOT EXISTS constants (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		upload_id INTEGER NOT NULL,
		name TEXT NOT NULL,
		synonym TEXT,
		type TEXT,
		value TEXT,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY(upload_id) REFERENCES uploads(id) ON DELETE CASCADE
	);

	-- Таблица справочников (метаданные)
	CREATE TABLE IF NOT EXISTS catalogs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		upload_id INTEGER NOT NULL,
		name TEXT NOT NULL,
		synonym TEXT,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY(upload_id) REFERENCES uploads(id) ON DELETE CASCADE
	);

	-- Таблица элементов справочников
	CREATE TABLE IF NOT EXISTS catalog_items (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		catalog_id INTEGER NOT NULL,
		reference TEXT NOT NULL,
		code TEXT,
		name TEXT,
		attributes_xml TEXT, -- XML с реквизитами
		table_parts_xml TEXT, -- XML с табличными частями
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY(catalog_id) REFERENCES catalogs(id) ON DELETE CASCADE
	);

	-- Таблица номенклатуры с характеристиками
	CREATE TABLE IF NOT EXISTS nomenclature_items (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		upload_id INTEGER NOT NULL,
		nomenclature_reference TEXT NOT NULL,
		nomenclature_code TEXT,
		nomenclature_name TEXT,
		characteristic_reference TEXT,
		characteristic_name TEXT,
		attributes_xml TEXT,
		table_parts_xml TEXT,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY(upload_id) REFERENCES uploads(id) ON DELETE CASCADE
	);

	-- Индексы для оптимизации запросов (без индексов на database_id, client_id, project_id - они будут созданы после миграции)
	CREATE INDEX IF NOT EXISTS idx_uploads_uuid ON uploads(upload_uuid);
	CREATE INDEX IF NOT EXISTS idx_constants_upload_id ON constants(upload_id);
	CREATE INDEX IF NOT EXISTS idx_catalogs_upload_id ON catalogs(upload_id);
	CREATE INDEX IF NOT EXISTS idx_catalog_items_catalog_id ON catalog_items(catalog_id);
	CREATE INDEX IF NOT EXISTS idx_catalog_items_reference ON catalog_items(reference);
	CREATE INDEX IF NOT EXISTS idx_nomenclature_items_upload_id ON nomenclature_items(upload_id);
	CREATE INDEX IF NOT EXISTS idx_nomenclature_items_reference ON nomenclature_items(nomenclature_reference);
	`

	_, err := db.Exec(schema)
	if err != nil {
		return fmt.Errorf("failed to create schema: %w", err)
	}

	// Выполняем миграцию для расширения таблицы uploads (ПЕРЕД созданием индексов на эти поля)
	if err := MigrateUploadsTable(db); err != nil {
		return fmt.Errorf("failed to migrate uploads table: %w", err)
	}

	// Создаем индексы на мигрированные поля после миграции
	indexesAfterMigration := []string{
		`CREATE INDEX IF NOT EXISTS idx_uploads_database_id ON uploads(database_id)`,
		`CREATE INDEX IF NOT EXISTS idx_uploads_client_id ON uploads(client_id)`,
		`CREATE INDEX IF NOT EXISTS idx_uploads_project_id ON uploads(project_id)`,
	}
	for _, indexSQL := range indexesAfterMigration {
		_, err = db.Exec(indexSQL)
		if err != nil {
			// Игнорируем ошибки создания индекса, если он уже существует
			errStr := strings.ToLower(err.Error())
			if !strings.Contains(errStr, "duplicate index") && !strings.Contains(errStr, "already exists") {
				return fmt.Errorf("failed to create index after migration: %w", err)
			}
		}
	}

	// Выполняем миграцию для полей нормализации
	if err := MigrateNomenclatureFields(db); err != nil {
		return fmt.Errorf("failed to migrate nomenclature fields: %w", err)
	}

	// Создаем дополнительные индексы для оптимизации запросов качества данных
	nomenclatureIndexes := []string{
		`CREATE INDEX IF NOT EXISTS idx_nomenclature_items_code ON nomenclature_items(nomenclature_code)`,
		`CREATE INDEX IF NOT EXISTS idx_nomenclature_items_name ON nomenclature_items(nomenclature_name)`,
		`CREATE INDEX IF NOT EXISTS idx_nomenclature_items_upload_code ON nomenclature_items(upload_id, nomenclature_code)`,
		`CREATE INDEX IF NOT EXISTS idx_nomenclature_items_upload_name ON nomenclature_items(upload_id, nomenclature_name)`,
	}
	for _, indexSQL := range nomenclatureIndexes {
		_, err = db.Exec(indexSQL)
		if err != nil {
			// Игнорируем ошибки создания индекса, если он уже существует
			errStr := strings.ToLower(err.Error())
			if !strings.Contains(errStr, "duplicate index") && !strings.Contains(errStr, "already exists") {
				return fmt.Errorf("failed to create nomenclature index: %w", err)
			}
		}
	}

	// Создаем таблицу normalized_data
	if err := CreateNormalizedDataTable(db); err != nil {
		return fmt.Errorf("failed to create normalized_data table: %w", err)
	}

	// Добавляем AI поля в normalized_data
	if err := MigrateNormalizedDataAIFields(db); err != nil {
		return fmt.Errorf("failed to migrate AI fields: %w", err)
	}

	// Создаем таблицу для атрибутов нормализованных товаров
	if err := CreateNormalizedItemAttributesTable(db); err != nil {
		return fmt.Errorf("failed to create normalized_item_attributes table: %w", err)
	}

	// Добавляем КПВЭД поля в normalized_data
	if err := MigrateNormalizedDataKpvedFields(db); err != nil {
		return fmt.Errorf("failed to migrate KPVED fields: %w", err)
	}

	// Добавляем поля качества и валидации в normalized_data
	if err := MigrateNormalizedDataQualityFields(db); err != nil {
		return fmt.Errorf("failed to migrate quality fields: %w", err)
	}

	// Создаем таблицы системы качества (DQAS)
	if err := CreateQualityAssessmentsTables(db); err != nil {
		return fmt.Errorf("failed to create quality assessment tables: %w", err)
	}

	// Создаем таблицы для анализа качества данных
	if err := CreateDataQualityTables(db); err != nil {
		return fmt.Errorf("failed to create data quality tables: %w", err)
	}

	// Создаем таблицы для срезов данных
	// CreateSnapshotTables должна быть определена в другом месте или закомментирована
	// if err := CreateSnapshotTables(db); err != nil {
	// 	return fmt.Errorf("failed to create snapshot tables: %w", err)
	// }

	// Создаем таблицу для нормализованных данных срезов
	if err := CreateSnapshotNormalizedDataTable(db); err != nil {
		return fmt.Errorf("failed to create snapshot normalized data table: %w", err)
	}

	// Создаем таблицы для многостадийного версионирования
	if err := CreateNormalizationVersionsTables(db); err != nil {
		return fmt.Errorf("failed to create normalization versions tables: %w", err)
	}

	// Создаем таблицы для системы классификации
	if err := CreateClassificationTables(db); err != nil {
		return fmt.Errorf("failed to create classification tables: %w", err)
	}

	return nil
}

// MigrateUploadsTable добавляет новые поля в таблицу uploads для связи с базами данных
func MigrateUploadsTable(db *sql.DB) error {
	migrations := []string{
		`ALTER TABLE uploads ADD COLUMN database_id INTEGER`,
		`ALTER TABLE uploads ADD COLUMN client_id INTEGER`,
		`ALTER TABLE uploads ADD COLUMN project_id INTEGER`,
		`ALTER TABLE uploads ADD COLUMN computer_name TEXT`,
		`ALTER TABLE uploads ADD COLUMN user_name TEXT`,
		`ALTER TABLE uploads ADD COLUMN config_version TEXT`,
		// Поля для итераций
		`ALTER TABLE uploads ADD COLUMN iteration_number INTEGER DEFAULT 1`,
		`ALTER TABLE uploads ADD COLUMN iteration_label VARCHAR(100)`,
		`ALTER TABLE uploads ADD COLUMN programmer_name VARCHAR(255)`,
		`ALTER TABLE uploads ADD COLUMN upload_purpose TEXT`,
		`ALTER TABLE uploads ADD COLUMN parent_upload_id INTEGER`,
	}

	for _, migration := range migrations {
		// Игнорируем ошибки, если поле уже существует
		_, err := db.Exec(migration)
		if err != nil {
			errStr := strings.ToLower(err.Error())
			// Проверяем, что это не ошибка "duplicate column name" или "already exists"
			if !strings.Contains(errStr, "duplicate column") &&
				!strings.Contains(errStr, "already exists") {
				return fmt.Errorf("migration failed: %s, error: %w", migration, err)
			}
		}
	}

	return nil
}

// MigrateNomenclatureFields добавляет поля для нормализации номенклатуры в таблицу catalog_items
func MigrateNomenclatureFields(db *sql.DB) error {
	migrations := []string{
		// Переименование существующих полей, если они есть (для совместимости со старыми БД)
		`ALTER TABLE catalog_items ADD COLUMN normalized_name TEXT`,
		`ALTER TABLE catalog_items ADD COLUMN kpved_code TEXT`,
		`ALTER TABLE catalog_items ADD COLUMN kpved_name TEXT`,
		`ALTER TABLE catalog_items ADD COLUMN processing_status TEXT DEFAULT 'pending'`,
		`ALTER TABLE catalog_items ADD COLUMN processed_at TIMESTAMP`,
		`ALTER TABLE catalog_items ADD COLUMN error_message TEXT`,
		`ALTER TABLE catalog_items ADD COLUMN ai_response_raw TEXT`,
		`ALTER TABLE catalog_items ADD COLUMN processing_attempts INTEGER DEFAULT 0`,
		`ALTER TABLE catalog_items ADD COLUMN last_processed_at TIMESTAMP`,
		// Индексы для оптимизации
		`CREATE INDEX IF NOT EXISTS idx_catalog_items_status ON catalog_items(processing_status)`,
		`CREATE INDEX IF NOT EXISTS idx_catalog_items_processed ON catalog_items(processed_at)`,
	}

	for _, migration := range migrations {
		// Игнорируем ошибки, если поле уже существует (SQLite не поддерживает IF NOT EXISTS для ALTER TABLE ADD COLUMN)
		_, err := db.Exec(migration)
		if err != nil {
			errStr := strings.ToLower(err.Error())
			// Проверяем, что это не ошибка "duplicate column name" или "already exists"
			if !strings.Contains(errStr, "duplicate column") &&
				!strings.Contains(errStr, "already exists") &&
				!strings.Contains(errStr, "duplicate index") {
				return fmt.Errorf("migration failed: %s, error: %w", migration, err)
			}
		}
	}

	return nil
}

// CreateNormalizedDataTable создает таблицу normalized_data для хранения нормализованных данных
func CreateNormalizedDataTable(db *sql.DB) error {
	// Проверяем существование таблицы
	var exists bool
	err := db.QueryRow(`
		SELECT EXISTS (
			SELECT 1 FROM sqlite_master 
			WHERE type='table' AND name='normalized_data'
		)
	`).Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to check table existence: %w", err)
	}

	if exists {
		// Таблица уже существует, пропускаем создание
		return nil
	}

	// Создаем таблицу
	createTable := `
		CREATE TABLE normalized_data (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			source_reference TEXT,
			source_name TEXT,
			code TEXT UNIQUE,
			normalized_name TEXT,
			normalized_reference TEXT,
			category TEXT,
			merged_count INTEGER DEFAULT 1,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`

	_, err = db.Exec(createTable)
	if err != nil {
		return fmt.Errorf("failed to create normalized_data table: %w", err)
	}

	// Создаем базовые индексы (только для колонок, которые существуют при создании таблицы)
	indexes := []string{
		// Одиночные индексы
		`CREATE INDEX IF NOT EXISTS idx_normalized_name ON normalized_data(normalized_name)`,
		`CREATE INDEX IF NOT EXISTS idx_normalized_reference ON normalized_data(normalized_reference)`,
		`CREATE INDEX IF NOT EXISTS idx_category ON normalized_data(category)`,

		// Композитные индексы для повышения производительности
		// Используются для быстрой группировки и поиска дубликатов
		`CREATE INDEX IF NOT EXISTS idx_name_category ON normalized_data(normalized_name, category)`,

		// Для проверки дубликатов по коду (используется в filterDuplicatesFromBatch)
		`CREATE INDEX IF NOT EXISTS idx_code ON normalized_data(code)`,
	}

	for _, indexSQL := range indexes {
		_, err = db.Exec(indexSQL)
		if err != nil {
			return fmt.Errorf("failed to create index: %w", err)
		}
	}

	return nil
}

// CreateNormalizedItemAttributesTable создает таблицу для хранения извлеченных атрибутов нормализованных товаров
func CreateNormalizedItemAttributesTable(db *sql.DB) error {
	// Проверяем существование таблицы
	var exists bool
	err := db.QueryRow(`
		SELECT EXISTS (
			SELECT 1 FROM sqlite_master 
			WHERE type='table' AND name='normalized_item_attributes'
		)
	`).Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to check table existence: %w", err)
	}

	if exists {
		// Таблица уже существует, пропускаем создание
		return nil
	}

	// Создаем таблицу
	createTable := `
		CREATE TABLE normalized_item_attributes (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			normalized_item_id INTEGER NOT NULL,
			attribute_type TEXT NOT NULL,
			attribute_name TEXT,
			attribute_value TEXT NOT NULL,
			unit TEXT,
			original_text TEXT,
			confidence REAL DEFAULT 1.0,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY(normalized_item_id) REFERENCES normalized_data(id) ON DELETE CASCADE
		)
	`

	_, err = db.Exec(createTable)
	if err != nil {
		return fmt.Errorf("failed to create normalized_item_attributes table: %w", err)
	}

	// Создаем индексы
	indexes := []string{
		`CREATE INDEX IF NOT EXISTS idx_attributes_item_id ON normalized_item_attributes(normalized_item_id)`,
		`CREATE INDEX IF NOT EXISTS idx_attributes_type ON normalized_item_attributes(attribute_type)`,
		`CREATE INDEX IF NOT EXISTS idx_attributes_name ON normalized_item_attributes(attribute_name)`,
		`CREATE INDEX IF NOT EXISTS idx_attributes_item_type ON normalized_item_attributes(normalized_item_id, attribute_type)`,
	}

	for _, indexSQL := range indexes {
		_, err = db.Exec(indexSQL)
		if err != nil {
			return fmt.Errorf("failed to create index: %w", err)
		}
	}

	return nil
}

// MigrateNormalizedDataAIFields добавляет AI поля в таблицу normalized_data
func MigrateNormalizedDataAIFields(db *sql.DB) error {
	migrations := []string{
		`ALTER TABLE normalized_data ADD COLUMN ai_confidence REAL DEFAULT 0.0`,
		`ALTER TABLE normalized_data ADD COLUMN ai_reasoning TEXT`,
		`ALTER TABLE normalized_data ADD COLUMN processing_level TEXT DEFAULT 'basic'`,
		`CREATE INDEX IF NOT EXISTS idx_normalized_processing_level ON normalized_data(processing_level)`,
		`CREATE INDEX IF NOT EXISTS idx_normalized_ai_confidence ON normalized_data(ai_confidence)`,
		// Индексы, использующие processing_level и ai_confidence (создаются после добавления колонок)
		`CREATE INDEX IF NOT EXISTS idx_category_level ON normalized_data(category, processing_level)`,
		`CREATE INDEX IF NOT EXISTS idx_level_confidence ON normalized_data(processing_level, ai_confidence)`,
	}

	for _, migration := range migrations {
		// Игнорируем ошибки, если поле уже существует
		_, err := db.Exec(migration)
		if err != nil {
			errStr := strings.ToLower(err.Error())
			// Проверяем, что это не ошибка "duplicate column name" или "already exists"
			if !strings.Contains(errStr, "duplicate column") &&
				!strings.Contains(errStr, "already exists") &&
				!strings.Contains(errStr, "duplicate index") {
				return fmt.Errorf("migration failed: %s, error: %w", migration, err)
			}
		}
	}

	return nil
}

// InitServiceSchema создает все необходимые таблицы в сервисной SQLite базе данных
func InitServiceSchema(db *sql.DB) error {
	schema := `
	-- Таблица клиентов
	CREATE TABLE IF NOT EXISTS clients (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL UNIQUE,
		legal_name TEXT,
		description TEXT,
		contact_email TEXT,
		contact_phone TEXT,
		tax_id TEXT,
		status TEXT DEFAULT 'active',
		created_by TEXT,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	-- Таблица проектов/справочников клиента
	CREATE TABLE IF NOT EXISTS client_projects (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		client_id INTEGER NOT NULL,
		name TEXT NOT NULL,
		project_type TEXT NOT NULL,
		description TEXT,
		source_system TEXT,
		status TEXT DEFAULT 'active',
		target_quality_score REAL DEFAULT 0.9,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY(client_id) REFERENCES clients(id) ON DELETE CASCADE
	);

	-- Таблица эталонных записей клиента
	CREATE TABLE IF NOT EXISTS client_benchmarks (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		client_project_id INTEGER NOT NULL,
		original_name TEXT NOT NULL,
		normalized_name TEXT NOT NULL,
		category TEXT NOT NULL,
		subcategory TEXT,
		attributes TEXT,
		quality_score REAL DEFAULT 0.0,
		is_approved BOOLEAN DEFAULT FALSE,
		approved_by TEXT,
		approved_at TIMESTAMP,
		source_database TEXT,
		usage_count INTEGER DEFAULT 0,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY(client_project_id) REFERENCES client_projects(id) ON DELETE CASCADE
	);

	-- Таблица сессий нормализации для клиентов
	CREATE TABLE IF NOT EXISTS client_normalization_sessions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		client_project_id INTEGER NOT NULL,
		session_type TEXT NOT NULL,
		status TEXT DEFAULT 'pending',
		source_database_path TEXT,
		processed_count INTEGER DEFAULT 0,
		benchmark_created_count INTEGER DEFAULT 0,
		quality_metrics TEXT,
		started_at TIMESTAMP,
		completed_at TIMESTAMP,
		created_by TEXT,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY(client_project_id) REFERENCES client_projects(id) ON DELETE CASCADE
	);

	-- Таблица баз данных проекта
	CREATE TABLE IF NOT EXISTS project_databases (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		client_project_id INTEGER NOT NULL,
		name TEXT NOT NULL,
		file_path TEXT NOT NULL,
		description TEXT,
		is_active BOOLEAN DEFAULT TRUE,
		file_size INTEGER,
		last_used_at TIMESTAMP,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY(client_project_id) REFERENCES client_projects(id) ON DELETE CASCADE
	);

	-- Таблица конфигурации нормализации
	CREATE TABLE IF NOT EXISTS normalization_config (
		id INTEGER PRIMARY KEY CHECK (id = 1), -- Только одна конфигурация
		database_path TEXT NOT NULL,
		source_table TEXT NOT NULL,
		reference_column TEXT NOT NULL,
		code_column TEXT NOT NULL,
		name_column TEXT NOT NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	-- Вставляем дефолтную конфигурацию если таблица пустая
	INSERT OR IGNORE INTO normalization_config (id, database_path, source_table, reference_column, code_column, name_column)
	VALUES (1, '', 'catalog_items', 'reference', 'code', 'name');

	-- Таблица метаданных баз данных
	CREATE TABLE IF NOT EXISTS database_metadata (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		file_path TEXT NOT NULL UNIQUE,
		database_type TEXT NOT NULL,
		description TEXT,
		first_seen_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		last_analyzed_at TIMESTAMP,
		metadata_json TEXT
	);

	-- Индексы для оптимизации запросов
	CREATE INDEX IF NOT EXISTS idx_client_projects_client_id ON client_projects(client_id);
	CREATE INDEX IF NOT EXISTS idx_client_benchmarks_project_id ON client_benchmarks(client_project_id);
	CREATE INDEX IF NOT EXISTS idx_client_benchmarks_normalized_name ON client_benchmarks(normalized_name);
	CREATE INDEX IF NOT EXISTS idx_client_benchmarks_approved ON client_benchmarks(is_approved);
	CREATE INDEX IF NOT EXISTS idx_client_sessions_project_id ON client_normalization_sessions(client_project_id);
	CREATE INDEX IF NOT EXISTS idx_clients_status ON clients(status);
	CREATE INDEX IF NOT EXISTS idx_project_databases_project_id ON project_databases(client_project_id);
	CREATE INDEX IF NOT EXISTS idx_project_databases_active ON project_databases(is_active);
	CREATE INDEX IF NOT EXISTS idx_database_metadata_file_path ON database_metadata(file_path);
	CREATE INDEX IF NOT EXISTS idx_database_metadata_type ON database_metadata(database_type);

	-- Таблица конфигурации воркеров и провайдеров AI
	CREATE TABLE IF NOT EXISTS worker_config (
		id INTEGER PRIMARY KEY CHECK (id = 1), -- Только одна конфигурация
		config_json TEXT NOT NULL, -- JSON с полной конфигурацией (включая API ключи)
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	-- Индекс для оптимизации
	CREATE INDEX IF NOT EXISTS idx_worker_config_updated_at ON worker_config(updated_at);

	-- Таблица классификатора КПВЭД
	CREATE TABLE IF NOT EXISTS kpved_classifier (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		code TEXT NOT NULL UNIQUE,
		name TEXT NOT NULL,
		parent_code TEXT,
		level INTEGER,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	-- Индексы для таблицы kpved_classifier
	CREATE INDEX IF NOT EXISTS idx_kpved_code ON kpved_classifier(code);
	CREATE INDEX IF NOT EXISTS idx_kpved_parent ON kpved_classifier(parent_code);
	CREATE INDEX IF NOT EXISTS idx_kpved_level ON kpved_classifier(level);
	`

	_, err := db.Exec(schema)
	if err != nil {
		return fmt.Errorf("failed to create service schema: %w", err)
	}

	return nil
}

// CreateKpvedClassifierTable создает таблицу для хранения иерархии классификатора КПВЭД
func CreateKpvedClassifierTable(db *sql.DB) error {
	// Проверяем существование таблицы
	var exists bool
	err := db.QueryRow(`
		SELECT EXISTS (
			SELECT 1 FROM sqlite_master
			WHERE type='table' AND name='kpved_classifier'
		)
	`).Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to check kpved_classifier table existence: %w", err)
	}

	if exists {
		// Таблица уже существует, пропускаем создание
		return nil
	}

	// Создаем таблицу
	createTable := `
		CREATE TABLE kpved_classifier (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			code TEXT NOT NULL UNIQUE,
			name TEXT NOT NULL,
			parent_code TEXT,
			level INTEGER,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`

	_, err = db.Exec(createTable)
	if err != nil {
		return fmt.Errorf("failed to create kpved_classifier table: %w", err)
	}

	// Создаем индексы
	indexes := []string{
		`CREATE INDEX IF NOT EXISTS idx_kpved_code ON kpved_classifier(code)`,
		`CREATE INDEX IF NOT EXISTS idx_kpved_parent ON kpved_classifier(parent_code)`,
		`CREATE INDEX IF NOT EXISTS idx_kpved_level ON kpved_classifier(level)`,
	}

	for _, indexSQL := range indexes {
		_, err = db.Exec(indexSQL)
		if err != nil {
			return fmt.Errorf("failed to create index: %w", err)
		}
	}

	return nil
}

// MigrateNormalizedDataKpvedFields добавляет КПВЭД поля в таблицу normalized_data
func MigrateNormalizedDataKpvedFields(db *sql.DB) error {
	migrations := []string{
		`ALTER TABLE normalized_data ADD COLUMN kpved_code TEXT`,
		`ALTER TABLE normalized_data ADD COLUMN kpved_name TEXT`,
		`ALTER TABLE normalized_data ADD COLUMN kpved_confidence REAL DEFAULT 0.0`,
		`CREATE INDEX IF NOT EXISTS idx_normalized_kpved_code ON normalized_data(kpved_code)`,
		`CREATE INDEX IF NOT EXISTS idx_normalized_kpved_name ON normalized_data(kpved_name)`,
		// Индексы, использующие kpved_code и kpved_name (создаются после добавления колонок)
		`CREATE INDEX IF NOT EXISTS idx_kpved ON normalized_data(kpved_code, kpved_name)`,
		`CREATE INDEX IF NOT EXISTS idx_normalized_name_category_kpved ON normalized_data(normalized_name, category, kpved_code)`,
		`CREATE INDEX IF NOT EXISTS idx_name_category_update ON normalized_data(normalized_name, category, kpved_code, kpved_name)`,
	}

	for _, migration := range migrations {
		// Игнорируем ошибки, если поле уже существует
		_, err := db.Exec(migration)
		if err != nil {
			errStr := strings.ToLower(err.Error())
			// Проверяем, что это не ошибка "duplicate column name" или "already exists"
			if !strings.Contains(errStr, "duplicate column") &&
				!strings.Contains(errStr, "already exists") &&
				!strings.Contains(errStr, "duplicate index") {
				return fmt.Errorf("migration failed: %s, error: %w", migration, err)
			}
		}
	}

	return nil
}

// MigrateNormalizedDataQualityFields добавляет поля качества и валидации в таблицу normalized_data
func MigrateNormalizedDataQualityFields(db *sql.DB) error {
	migrations := []string{
		`ALTER TABLE normalized_data ADD COLUMN quality_score REAL DEFAULT 0.0`,
		`ALTER TABLE normalized_data ADD COLUMN validation_status TEXT DEFAULT ''`,
		`ALTER TABLE normalized_data ADD COLUMN validation_reason TEXT`,
		`CREATE INDEX IF NOT EXISTS idx_normalized_quality_score ON normalized_data(quality_score)`,
		`CREATE INDEX IF NOT EXISTS idx_normalized_validation_status ON normalized_data(validation_status)`,
	}

	for _, migration := range migrations {
		// Игнорируем ошибки, если поле уже существует
		_, err := db.Exec(migration)
		if err != nil {
			errStr := strings.ToLower(err.Error())
			// Проверяем, что это не ошибка "duplicate column name" или "already exists"
			if !strings.Contains(errStr, "duplicate column") &&
				!strings.Contains(errStr, "already exists") &&
				!strings.Contains(errStr, "duplicate index") {
				return fmt.Errorf("migration failed: %s, error: %w", migration, err)
			}
		}
	}

	return nil
}

// CreateQualityAssessmentsTables создает таблицы для системы оценки качества данных (DQAS)
func CreateQualityAssessmentsTables(db *sql.DB) error {
	// Таблица для хранения оценок качества
	qualityAssessmentsTable := `
		CREATE TABLE IF NOT EXISTS quality_assessments (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			normalized_item_id INTEGER NOT NULL,
			assessment_date TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			overall_score REAL NOT NULL,
			category_confidence REAL,
			name_clarity REAL,
			consistency REAL,
			completeness REAL,
			standardization REAL,
			kpved_accuracy REAL,
			duplicate_score REAL,
			data_enrichment REAL,
			is_benchmark BOOLEAN DEFAULT FALSE,
			issues_json TEXT,  -- JSON array с найденными проблемами
			FOREIGN KEY(normalized_item_id) REFERENCES normalized_data(id) ON DELETE CASCADE
		)
	`

	// Таблица для нарушений правил качества
	qualityViolationsTable := `
		CREATE TABLE IF NOT EXISTS quality_violations (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			normalized_item_id INTEGER NOT NULL,
			rule_name TEXT NOT NULL,
			category TEXT NOT NULL,  -- completeness, accuracy, consistency, uniqueness, format
			severity TEXT NOT NULL,  -- info, warning, error, critical
			description TEXT NOT NULL,
			field TEXT,
			current_value TEXT,
			recommendation TEXT,
			detected_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			resolved_at TIMESTAMP,
			resolved_by TEXT,
			FOREIGN KEY(normalized_item_id) REFERENCES normalized_data(id) ON DELETE CASCADE
		)
	`

	// Таблица для предложений по улучшению
	qualitySuggestionsTable := `
		CREATE TABLE IF NOT EXISTS quality_suggestions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			normalized_item_id INTEGER NOT NULL,
			suggestion_type TEXT NOT NULL,  -- set_value, correct_format, reprocess, merge, review
			priority TEXT NOT NULL,  -- low, medium, high, critical
			field TEXT NOT NULL,
			current_value TEXT,
			suggested_value TEXT,
			confidence REAL NOT NULL,
			reasoning TEXT,
			auto_applyable BOOLEAN DEFAULT FALSE,
			applied BOOLEAN DEFAULT FALSE,
			applied_at TIMESTAMP,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY(normalized_item_id) REFERENCES normalized_data(id) ON DELETE CASCADE
		)
	`

	// Таблица для групп дубликатов
	duplicateGroupsTable := `
		CREATE TABLE IF NOT EXISTS duplicate_groups (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			group_hash TEXT NOT NULL UNIQUE,  -- Хэш для идентификации группы
			duplicate_type TEXT NOT NULL,  -- exact, semantic, phonetic, mixed
			similarity_score REAL NOT NULL,
			item_ids_json TEXT NOT NULL,  -- JSON array с ID записей в группе
			suggested_master_id INTEGER,  -- Предлагаемый master record
			confidence REAL NOT NULL,
			reason TEXT,
			merged BOOLEAN DEFAULT FALSE,
			merged_at TIMESTAMP,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`

	// Создаем таблицы
	tables := []string{
		qualityAssessmentsTable,
		qualityViolationsTable,
		qualitySuggestionsTable,
		duplicateGroupsTable,
	}

	for _, tableSQL := range tables {
		_, err := db.Exec(tableSQL)
		if err != nil {
			return fmt.Errorf("failed to create quality table: %w", err)
		}
	}

	// Создаем индексы для оптимизации запросов
	indexes := []string{
		// Индексы для quality_assessments
		`CREATE INDEX IF NOT EXISTS idx_qa_item_id ON quality_assessments(normalized_item_id)`,
		`CREATE INDEX IF NOT EXISTS idx_qa_overall_score ON quality_assessments(overall_score)`,
		`CREATE INDEX IF NOT EXISTS idx_qa_is_benchmark ON quality_assessments(is_benchmark)`,
		`CREATE INDEX IF NOT EXISTS idx_qa_date ON quality_assessments(assessment_date)`,

		// Индексы для quality_violations
		`CREATE INDEX IF NOT EXISTS idx_qv_item_id ON quality_violations(normalized_item_id)`,
		`CREATE INDEX IF NOT EXISTS idx_qv_severity ON quality_violations(severity)`,
		`CREATE INDEX IF NOT EXISTS idx_qv_category ON quality_violations(category)`,
		`CREATE INDEX IF NOT EXISTS idx_qv_resolved ON quality_violations(resolved_at)`,

		// Индексы для quality_suggestions
		`CREATE INDEX IF NOT EXISTS idx_qs_item_id ON quality_suggestions(normalized_item_id)`,
		`CREATE INDEX IF NOT EXISTS idx_qs_priority ON quality_suggestions(priority)`,
		`CREATE INDEX IF NOT EXISTS idx_qs_applied ON quality_suggestions(applied)`,
		`CREATE INDEX IF NOT EXISTS idx_qs_auto_applyable ON quality_suggestions(auto_applyable)`,

		// Индексы для duplicate_groups
		`CREATE INDEX IF NOT EXISTS idx_dg_hash ON duplicate_groups(group_hash)`,
		`CREATE INDEX IF NOT EXISTS idx_dg_type ON duplicate_groups(duplicate_type)`,
		`CREATE INDEX IF NOT EXISTS idx_dg_merged ON duplicate_groups(merged)`,
		`CREATE INDEX IF NOT EXISTS idx_dg_similarity ON duplicate_groups(similarity_score)`,
	}

	for _, indexSQL := range indexes {
		_, err := db.Exec(indexSQL)
		if err != nil {
			return fmt.Errorf("failed to create quality index: %w", err)
		}
	}

	return nil
}

// CreateDataQualityTables создает таблицы для анализа качества данных из выгрузок 1С
func CreateDataQualityTables(db *sql.DB) error {
	// Таблица метрик качества
	dataQualityMetricsTable := `
		CREATE TABLE IF NOT EXISTS data_quality_metrics (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			upload_id INTEGER NOT NULL,
			database_id INTEGER NOT NULL,
			metric_category TEXT NOT NULL,
			metric_name TEXT NOT NULL,
			metric_value REAL NOT NULL,
			threshold_value REAL,
			status TEXT CHECK(status IN ('PASS', 'WARNING', 'FAIL')),
			measured_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			details TEXT,
			FOREIGN KEY(upload_id) REFERENCES uploads(id) ON DELETE CASCADE
		)
	`

	// Таблица проблем качества
	dataQualityIssuesTable := `
		CREATE TABLE IF NOT EXISTS data_quality_issues (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			upload_id INTEGER NOT NULL,
			database_id INTEGER NOT NULL,
			entity_type TEXT NOT NULL,
			entity_reference TEXT,
			issue_type TEXT NOT NULL,
			issue_severity TEXT CHECK(issue_severity IN ('CRITICAL', 'HIGH', 'MEDIUM', 'LOW')),
			field_name TEXT,
			expected_value TEXT,
			actual_value TEXT,
			description TEXT,
			detected_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			resolved_at TIMESTAMP,
			status TEXT DEFAULT 'OPEN' CHECK(status IN ('OPEN', 'IN_PROGRESS', 'RESOLVED')),
			FOREIGN KEY(upload_id) REFERENCES uploads(id) ON DELETE CASCADE
		)
	`

	// Таблица трендов качества
	qualityTrendsTable := `
		CREATE TABLE IF NOT EXISTS quality_trends (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			database_id INTEGER NOT NULL,
			measurement_date DATE NOT NULL,
			overall_score REAL NOT NULL,
			completeness_score REAL,
			consistency_score REAL,
			uniqueness_score REAL,
			validity_score REAL,
			records_analyzed INTEGER,
			issues_count INTEGER,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(database_id, measurement_date)
		)
	`

	// Таблица конфигурации проверок качества
	qualityChecksConfigTable := `
		CREATE TABLE IF NOT EXISTS quality_checks_config (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			database_id INTEGER NOT NULL,
			entity_type TEXT NOT NULL,
			check_name TEXT NOT NULL,
			check_type TEXT NOT NULL,
			parameters TEXT,
			is_active BOOLEAN DEFAULT TRUE,
			severity TEXT DEFAULT 'MEDIUM',
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`

	// Таблица истории метрик производительности
	performanceMetricsHistoryTable := `
		CREATE TABLE IF NOT EXISTS performance_metrics_history (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			metric_type TEXT NOT NULL,  -- 'ai', 'cache', 'batch', 'quality', 'circuit_breaker', 'checkpoint'
			metric_data TEXT NOT NULL,  -- JSON со всеми метриками
			uptime_seconds INTEGER,
			throughput REAL,
			ai_success_rate REAL,
			cache_hit_rate REAL,
			batch_queue_size INTEGER,
			circuit_breaker_state TEXT,
			checkpoint_progress REAL
		)
	`

	// Создаем таблицы
	tables := []string{
		dataQualityMetricsTable,
		dataQualityIssuesTable,
		qualityTrendsTable,
		qualityChecksConfigTable,
		performanceMetricsHistoryTable,
	}

	for _, tableSQL := range tables {
		_, err := db.Exec(tableSQL)
		if err != nil {
			return fmt.Errorf("failed to create data quality table: %w", err)
		}
	}

	// Добавляем поле quality_score в таблицу uploads
	migration := `ALTER TABLE uploads ADD COLUMN quality_score REAL`
	_, err := db.Exec(migration)
	if err != nil {
		errStr := strings.ToLower(err.Error())
		if !strings.Contains(errStr, "duplicate column") &&
			!strings.Contains(errStr, "already exists") {
			return fmt.Errorf("failed to add quality_score column: %w", err)
		}
	}

	// Создаем индексы для оптимизации запросов
	indexes := []string{
		// Индексы для data_quality_metrics
		`CREATE INDEX IF NOT EXISTS idx_dqm_upload_id ON data_quality_metrics(upload_id)`,
		`CREATE INDEX IF NOT EXISTS idx_dqm_database_id ON data_quality_metrics(database_id)`,
		`CREATE INDEX IF NOT EXISTS idx_dqm_category ON data_quality_metrics(metric_category)`,
		`CREATE INDEX IF NOT EXISTS idx_dqm_name ON data_quality_metrics(metric_name)`,
		`CREATE INDEX IF NOT EXISTS idx_dqm_measured_at ON data_quality_metrics(measured_at)`,

		// Индексы для data_quality_issues
		`CREATE INDEX IF NOT EXISTS idx_dqi_upload_id ON data_quality_issues(upload_id)`,
		`CREATE INDEX IF NOT EXISTS idx_dqi_database_id ON data_quality_issues(database_id)`,
		`CREATE INDEX IF NOT EXISTS idx_dqi_entity_type ON data_quality_issues(entity_type)`,
		`CREATE INDEX IF NOT EXISTS idx_dqi_issue_type ON data_quality_issues(issue_type)`,
		`CREATE INDEX IF NOT EXISTS idx_dqi_severity ON data_quality_issues(issue_severity)`,
		`CREATE INDEX IF NOT EXISTS idx_dqi_status ON data_quality_issues(status)`,
		`CREATE INDEX IF NOT EXISTS idx_dqi_detected_at ON data_quality_issues(detected_at)`,

		// Индексы для quality_trends
		`CREATE INDEX IF NOT EXISTS idx_qt_database_id ON quality_trends(database_id)`,
		`CREATE INDEX IF NOT EXISTS idx_qt_measurement_date ON quality_trends(measurement_date)`,

		// Индексы для quality_checks_config
		`CREATE INDEX IF NOT EXISTS idx_qcc_database_id ON quality_checks_config(database_id)`,
		`CREATE INDEX IF NOT EXISTS idx_qcc_entity_type ON quality_checks_config(entity_type)`,
		`CREATE INDEX IF NOT EXISTS idx_qcc_active ON quality_checks_config(is_active)`,

		// Индексы для performance_metrics_history
		`CREATE INDEX IF NOT EXISTS idx_pmh_timestamp ON performance_metrics_history(timestamp)`,
		`CREATE INDEX IF NOT EXISTS idx_pmh_metric_type ON performance_metrics_history(metric_type)`,
		`CREATE INDEX IF NOT EXISTS idx_pmh_timestamp_type ON performance_metrics_history(timestamp, metric_type)`,
	}

	for _, indexSQL := range indexes {
		_, err := db.Exec(indexSQL)
		if err != nil {
			return fmt.Errorf("failed to create data quality index: %w", err)
		}
	}

	return nil
}

// CreateSnapshotNormalizedDataTable создает таблицу для результатов нормализации срезов
func CreateSnapshotNormalizedDataTable(db *sql.DB) error {
	// Проверяем существование таблицы
	var exists bool
	err := db.QueryRow(`
		SELECT EXISTS (
			SELECT 1 FROM sqlite_master 
			WHERE type='table' AND name='snapshot_normalized_data'
		)
	`).Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to check table existence: %w", err)
	}

	if exists {
		// Таблица уже существует, пропускаем создание
		return nil
	}

	// Создаем таблицу
	createTable := `
		CREATE TABLE IF NOT EXISTS snapshot_normalized_data (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			snapshot_id INTEGER NOT NULL,
			upload_id INTEGER NOT NULL,
			source_reference TEXT NOT NULL,
			source_name TEXT,
			code TEXT,
			normalized_name TEXT NOT NULL,
			normalized_reference TEXT NOT NULL,
			category TEXT,
			merged_count INTEGER DEFAULT 1,
			source_database_id INTEGER,
			source_iteration_number INTEGER,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY(snapshot_id) REFERENCES data_snapshots(id) ON DELETE CASCADE,
			FOREIGN KEY(upload_id) REFERENCES uploads(id) ON DELETE CASCADE
		)
	`

	_, err = db.Exec(createTable)
	if err != nil {
		return fmt.Errorf("failed to create snapshot_normalized_data table: %w", err)
	}

	// Создаем индексы
	indexes := []string{
		`CREATE INDEX IF NOT EXISTS idx_snapshot_normalized_snapshot_id ON snapshot_normalized_data(snapshot_id)`,
		`CREATE INDEX IF NOT EXISTS idx_snapshot_normalized_upload_id ON snapshot_normalized_data(upload_id)`,
		`CREATE INDEX IF NOT EXISTS idx_snapshot_normalized_reference ON snapshot_normalized_data(normalized_reference)`,
		`CREATE INDEX IF NOT EXISTS idx_snapshot_normalized_name ON snapshot_normalized_data(normalized_name)`,
	}

	for _, indexSQL := range indexes {
		_, err := db.Exec(indexSQL)
		if err != nil {
			// Игнорируем ошибки создания индекса, если он уже существует
			errStr := strings.ToLower(err.Error())
			if !strings.Contains(errStr, "duplicate index") && !strings.Contains(errStr, "already exists") {
				return fmt.Errorf("failed to create snapshot normalized data index: %w", err)
			}
		}
	}

	return nil
}
