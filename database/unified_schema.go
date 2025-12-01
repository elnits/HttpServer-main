package database

import (
	"database/sql"
	"fmt"
	"strings"
)

// InitUnifiedSchema создает базовые таблицы для единой базы данных справочников
func InitUnifiedSchema(db *sql.DB) error {
	schema := `
	-- Таблица выгрузок (общая для всех)
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
		config_version TEXT,
		iteration_number INTEGER DEFAULT 1,
		iteration_label TEXT,
		programmer_name TEXT,
		upload_purpose TEXT,
		parent_upload_id INTEGER
	);

	-- Таблица констант (общая для всех выгрузок)
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

	-- Таблица маппинга справочников на имена таблиц
	CREATE TABLE IF NOT EXISTS catalog_mappings (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		catalog_name TEXT NOT NULL UNIQUE,
		table_name TEXT NOT NULL UNIQUE,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	-- Индексы для оптимизации запросов
	CREATE INDEX IF NOT EXISTS idx_uploads_uuid ON uploads(upload_uuid);
	CREATE INDEX IF NOT EXISTS idx_uploads_config ON uploads(config_name);
	CREATE INDEX IF NOT EXISTS idx_constants_upload_id ON constants(upload_id);
	CREATE INDEX IF NOT EXISTS idx_catalog_mappings_name ON catalog_mappings(catalog_name);
	CREATE INDEX IF NOT EXISTS idx_catalog_mappings_table ON catalog_mappings(table_name);
	`

	_, err := db.Exec(schema)
	if err != nil {
		return fmt.Errorf("failed to initialize unified schema: %w", err)
	}

	return nil
}

// CreateCatalogTable динамически создает таблицу для конкретного справочника
func CreateCatalogTable(db *sql.DB, tableName string) error {
	// Валидация имени таблицы (только буквы, цифры и подчеркивания)
	if !isValidTableName(tableName) {
		return fmt.Errorf("invalid table name: %s", tableName)
	}

	// Формируем SQL для создания таблицы
	schema := fmt.Sprintf(`
	CREATE TABLE IF NOT EXISTS %s (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		upload_id INTEGER NOT NULL,
		reference TEXT NOT NULL,
		code TEXT,
		name TEXT,
		attributes_xml TEXT,
		table_parts_xml TEXT,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY(upload_id) REFERENCES uploads(id) ON DELETE CASCADE
	);

	-- Индексы для оптимизации запросов
	CREATE INDEX IF NOT EXISTS idx_%s_upload_id ON %s(upload_id);
	CREATE INDEX IF NOT EXISTS idx_%s_reference ON %s(reference);
	CREATE INDEX IF NOT EXISTS idx_%s_code ON %s(code);
	CREATE INDEX IF NOT EXISTS idx_%s_name ON %s(name);
	`, tableName, tableName, tableName, tableName, tableName, tableName, tableName, tableName, tableName)

	_, err := db.Exec(schema)
	if err != nil {
		return fmt.Errorf("failed to create catalog table %s: %w", tableName, err)
	}

	return nil
}

// isValidTableName проверяет что имя таблицы содержит только допустимые символы
func isValidTableName(name string) bool {
	if name == "" || len(name) > 100 {
		return false
	}

	// Проверяем что имя содержит только буквы, цифры и подчеркивания
	for _, ch := range name {
		if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || 
			(ch >= '0' && ch <= '9') || ch == '_') {
			return false
		}
	}

	// Имя не должно начинаться с цифры
	if name[0] >= '0' && name[0] <= '9' {
		return false
	}

	// Имя не должно быть SQL ключевым словом
	sqlKeywords := []string{
		"select", "insert", "update", "delete", "drop", "create", "alter",
		"table", "index", "view", "trigger", "database", "schema",
	}
	nameLower := strings.ToLower(name)
	for _, keyword := range sqlKeywords {
		if nameLower == keyword {
			return false
		}
	}

	return true
}

// TableExists проверяет существует ли таблица в БД
func TableExists(db *sql.DB, tableName string) (bool, error) {
	query := `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`
	
	var count int
	err := db.QueryRow(query, tableName).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check if table exists: %w", err)
	}

	return count > 0, nil
}

// GetAllCatalogTables возвращает список всех таблиц справочников из маппинга
func GetAllCatalogTables(db *sql.DB) (map[string]string, error) {
	query := `SELECT catalog_name, table_name FROM catalog_mappings ORDER BY catalog_name`
	
	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to get catalog mappings: %w", err)
	}
	defer rows.Close()

	mappings := make(map[string]string)
	for rows.Next() {
		var catalogName, tableName string
		if err := rows.Scan(&catalogName, &tableName); err != nil {
			return nil, fmt.Errorf("failed to scan catalog mapping: %w", err)
		}
		mappings[catalogName] = tableName
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating catalog mappings: %w", err)
	}

	return mappings, nil
}


