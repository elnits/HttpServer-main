package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// TableStat статистика по таблице
type TableStat struct {
	Name        string `json:"name"`
	RowCount    int64  `json:"row_count"`
	SizeBytes   int64  `json:"size_bytes"`
	SizeMB      float64 `json:"size_mb"`
}

// DatabaseAnalytics полная аналитика базы данных
type DatabaseAnalytics struct {
	FilePath      string      `json:"file_path"`
	DatabaseType  string      `json:"database_type"`
	TotalSize     int64       `json:"total_size"`
	TotalSizeMB   float64     `json:"total_size_mb"`
	TableCount    int         `json:"table_count"`
	TotalRows     int64       `json:"total_rows"`
	TableStats    []TableStat `json:"table_stats"`
	TopTables     []TableStat `json:"top_tables"`
	AnalyzedAt    time.Time   `json:"analyzed_at"`
}

// HistoryEntry запись истории изменений
type HistoryEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Size      int64     `json:"size"`
	SizeMB    float64   `json:"size_mb"`
	RowCount  int64     `json:"row_count"`
}

// DetectDatabaseType определяет тип базы данных по наличию таблиц
func DetectDatabaseType(dbPath string) (string, error) {
	// Проверяем существование файла
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return "unknown", fmt.Errorf("database file does not exist: %s", dbPath)
	}

	// Открываем базу данных
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return "unknown", fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	// Проверяем наличие таблиц
	hasService := false
	hasUploads := false
	hasBenchmarks := false

	// Проверяем наличие таблицы clients (сервисная БД)
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='clients'").Scan(&count)
	if err == nil && count > 0 {
		hasService = true
	}

	// Проверяем наличие таблицы uploads (БД с выгрузками)
	err = db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='uploads'").Scan(&count)
	if err == nil && count > 0 {
		hasUploads = true
	}

	// Проверяем наличие таблицы client_benchmarks (БД с эталонами)
	err = db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='client_benchmarks'").Scan(&count)
	if err == nil && count > 0 {
		hasBenchmarks = true
	}

	// Определяем тип
	typeCount := 0
	if hasService {
		typeCount++
	}
	if hasUploads {
		typeCount++
	}
	if hasBenchmarks {
		typeCount++
	}

	if typeCount == 0 {
		return "unknown", nil
	}
	if typeCount > 1 {
		return "combined", nil
	}
	if hasService {
		return "service", nil
	}
	if hasUploads {
		return "uploads", nil
	}
	if hasBenchmarks {
		return "benchmarks", nil
	}

	return "unknown", nil
}

// GetTableStats получает статистику по всем таблицам базы данных
func GetTableStats(dbPath string) ([]TableStat, error) {
	// Открываем базу данных
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	// Получаем список всех таблиц
	rows, err := db.Query(`
		SELECT name FROM sqlite_master 
		WHERE type='table' AND name NOT LIKE 'sqlite_%'
		ORDER BY name
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to get tables: %w", err)
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			return nil, fmt.Errorf("failed to scan table name: %w", err)
		}
		tables = append(tables, tableName)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating tables: %w", err)
	}

	// Получаем статистику по каждой таблице
	var stats []TableStat
	for _, tableName := range tables {
		// Количество строк
		var rowCount int64
		err := db.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s", tableName)).Scan(&rowCount)
		if err != nil {
			// Пропускаем таблицы, к которым нет доступа
			continue
		}

		// Размер таблицы (приблизительно)
		// Используем простую оценку: количество строк * средний размер строки
		// Для разных типов таблиц используем разные оценки
		var sizeBytes int64
		avgRowSize := int64(200) // Базовая оценка: 200 байт на строку
		
		// Для больших таблиц увеличиваем оценку
		if rowCount > 100000 {
			avgRowSize = 500
		} else if rowCount > 10000 {
			avgRowSize = 300
		}
		
		sizeBytes = rowCount * avgRowSize

		stats = append(stats, TableStat{
			Name:      tableName,
			RowCount:  rowCount,
			SizeBytes: sizeBytes,
			SizeMB:    float64(sizeBytes) / (1024 * 1024),
		})
	}

	return stats, nil
}

// GetDatabaseAnalytics получает полную аналитику базы данных
func GetDatabaseAnalytics(dbPath string) (*DatabaseAnalytics, error) {
	// Определяем тип базы
	dbType, err := DetectDatabaseType(dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to detect database type: %w", err)
	}

	// Получаем информацию о файле
	fileInfo, err := os.Stat(dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get file info: %w", err)
	}
	totalSize := fileInfo.Size()

	// Получаем статистику по таблицам
	tableStats, err := GetTableStats(dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get table stats: %w", err)
	}

	// Вычисляем общее количество строк
	var totalRows int64
	for _, stat := range tableStats {
		totalRows += stat.RowCount
	}

	// Сортируем таблицы по размеру для топ-листа
	topTables := make([]TableStat, len(tableStats))
	copy(topTables, tableStats)
	// Сортируем по размеру (от большего к меньшему)
	for i := 0; i < len(topTables)-1; i++ {
		for j := i + 1; j < len(topTables); j++ {
			if topTables[i].SizeBytes < topTables[j].SizeBytes {
				topTables[i], topTables[j] = topTables[j], topTables[i]
			}
		}
	}
	// Берем топ-10
	if len(topTables) > 10 {
		topTables = topTables[:10]
	}

	return &DatabaseAnalytics{
		FilePath:     dbPath,
		DatabaseType: dbType,
		TotalSize:    totalSize,
		TotalSizeMB:  float64(totalSize) / (1024 * 1024),
		TableCount:   len(tableStats),
		TotalRows:    totalRows,
		TableStats:   tableStats,
		TopTables:    topTables,
		AnalyzedAt:   time.Now(),
	}, nil
}

// GetDatabaseHistory получает историю изменений базы данных из метаданных
func GetDatabaseHistory(serviceDB *ServiceDB, dbPath string) ([]HistoryEntry, error) {
	metadata, err := serviceDB.GetDatabaseMetadata(dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get database metadata: %w", err)
	}

	if metadata == nil || metadata.MetadataJSON == "" {
		return []HistoryEntry{}, nil
	}

	// Парсим JSON с историей
	var historyData struct {
		History []HistoryEntry `json:"history"`
	}
	if err := json.Unmarshal([]byte(metadata.MetadataJSON), &historyData); err != nil {
		return nil, fmt.Errorf("failed to parse history JSON: %w", err)
	}

	// Фильтруем историю за последние 30 дней
	thirtyDaysAgo := time.Now().AddDate(0, 0, -30)
	var filteredHistory []HistoryEntry
	for _, entry := range historyData.History {
		if entry.Timestamp.After(thirtyDaysAgo) {
			filteredHistory = append(filteredHistory, entry)
		}
	}

	return filteredHistory, nil
}

// UpdateDatabaseHistory обновляет историю изменений базы данных
func UpdateDatabaseHistory(serviceDB *ServiceDB, dbPath string, size int64, rowCount int64) error {
	metadata, err := serviceDB.GetDatabaseMetadata(dbPath)
	if err != nil {
		return fmt.Errorf("failed to get database metadata: %w", err)
	}

	var historyData struct {
		History []HistoryEntry `json:"history"`
	}

	// Если метаданные существуют, парсим существующую историю
	if metadata != nil && metadata.MetadataJSON != "" {
		if err := json.Unmarshal([]byte(metadata.MetadataJSON), &historyData); err != nil {
			// Если не удалось распарсить, начинаем с пустой истории
			historyData.History = []HistoryEntry{}
		}
	}

	// Добавляем новую запись
	newEntry := HistoryEntry{
		Timestamp: time.Now(),
		Size:      size,
		SizeMB:    float64(size) / (1024 * 1024),
		RowCount:  rowCount,
	}
	historyData.History = append(historyData.History, newEntry)

	// Ограничиваем историю последними 100 записями
	if len(historyData.History) > 100 {
		historyData.History = historyData.History[len(historyData.History)-100:]
	}

	// Сериализуем обратно в JSON
	historyJSON, err := json.Marshal(historyData)
	if err != nil {
		return fmt.Errorf("failed to marshal history: %w", err)
	}

	// Определяем тип базы
	dbType, err := DetectDatabaseType(dbPath)
	if err != nil {
		dbType = "unknown"
	}

	// Обновляем метаданные
	description := fmt.Sprintf("База данных типа %s", dbType)
	if err := serviceDB.UpsertDatabaseMetadata(dbPath, dbType, description, string(historyJSON)); err != nil {
		return fmt.Errorf("failed to update metadata: %w", err)
	}

	return nil
}

// GetDatabaseName возвращает читаемое имя базы данных из пути
func GetDatabaseName(dbPath string) string {
	return filepath.Base(dbPath)
}

