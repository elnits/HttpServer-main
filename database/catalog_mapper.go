package database

import (
	"database/sql"
	"fmt"
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// Таблица транслитерации для русских символов
var translitMap = map[rune]string{
	'а': "a", 'б': "b", 'в': "v", 'г': "g", 'д': "d", 'е': "e", 'ё': "yo",
	'ж': "zh", 'з': "z", 'и': "i", 'й': "y", 'к': "k", 'л': "l", 'м': "m",
	'н': "n", 'о': "o", 'п': "p", 'р': "r", 'с': "s", 'т': "t", 'у': "u",
	'ф': "f", 'х': "h", 'ц': "ts", 'ч': "ch", 'ш': "sh", 'щ': "shch",
	'ъ': "", 'ы': "y", 'ь': "", 'э': "e", 'ю': "yu", 'я': "ya",
	'А': "A", 'Б': "B", 'В': "V", 'Г': "G", 'Д': "D", 'Е': "E", 'Ё': "Yo",
	'Ж': "Zh", 'З': "Z", 'И': "I", 'Й': "Y", 'К': "K", 'Л': "L", 'М': "M",
	'Н': "N", 'О': "O", 'П': "P", 'Р': "R", 'С': "S", 'Т': "T", 'У': "U",
	'Ф': "F", 'Х': "H", 'Ц': "Ts", 'Ч': "Ch", 'Ш': "Sh", 'Щ': "Shch",
	'Ъ': "", 'Ы': "Y", 'Ь': "", 'Э': "E", 'Ю': "Yu", 'Я': "Ya",
}

// NormalizeCatalogName преобразует название справочника в имя таблицы
// Например: "Номенклатура" -> "nomenclature_items"
//           "ФизическиеЛица" -> "physical_persons_items"
func NormalizeCatalogName(catalogName string) string {
	if catalogName == "" {
		return "unknown_items"
	}

	// 1. Убираем пробелы в начале и конце
	catalogName = strings.TrimSpace(catalogName)

	// 2. Транслитерация
	result := strings.Builder{}
	for _, ch := range catalogName {
		if translitStr, ok := translitMap[ch]; ok {
			result.WriteString(translitStr)
		} else if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') {
			// Латинские буквы и цифры оставляем как есть
			result.WriteRune(ch)
		} else if ch == ' ' || ch == '-' || ch == '_' {
			// Пробелы, дефисы и подчеркивания заменяем на подчеркивание
			result.WriteRune('_')
		}
		// Остальные символы пропускаем
	}

	tableName := result.String()

	// 3. Приводим к нижнему регистру
	tableName = strings.ToLower(tableName)

	// 4. Удаляем множественные подчеркивания
	for strings.Contains(tableName, "__") {
		tableName = strings.ReplaceAll(tableName, "__", "_")
	}

	// 5. Удаляем подчеркивания в начале и конце
	tableName = strings.Trim(tableName, "_")

	// 6. Если имя пустое, используем дефолтное
	if tableName == "" {
		tableName = "unknown"
	}

	// 7. Добавляем суффикс _items
	tableName += "_items"

	// 8. Если имя начинается с цифры, добавляем префикс
	if len(tableName) > 0 && tableName[0] >= '0' && tableName[0] <= '9' {
		tableName = "cat_" + tableName
	}

	// 9. Ограничиваем длину (SQLite позволяет до 1024 символов, но разумно ограничить)
	if len(tableName) > 64 {
		tableName = tableName[:64]
	}

	return tableName
}

// GetOrCreateCatalogTable получает имя таблицы для справочника или создает новую
func GetOrCreateCatalogTable(db *sql.DB, catalogName string) (string, error) {
	// Нормализуем имя справочника
	tableName := NormalizeCatalogName(catalogName)

	// Проверяем существует ли маппинг
	existingTableName, err := GetCatalogTableName(db, catalogName)
	if err == nil && existingTableName != "" {
		// Маппинг найден, возвращаем существующую таблицу
		return existingTableName, nil
	}

	// Маппинга нет, создаем новую таблицу
	if err := CreateCatalogTable(db, tableName); err != nil {
		return "", fmt.Errorf("failed to create catalog table: %w", err)
	}

	// Сохраняем маппинг
	if err := SaveCatalogMapping(db, catalogName, tableName); err != nil {
		return "", fmt.Errorf("failed to save catalog mapping: %w", err)
	}

	return tableName, nil
}

// GetCatalogTableName получает имя таблицы для справочника из маппинга
func GetCatalogTableName(db *sql.DB, catalogName string) (string, error) {
	query := `SELECT table_name FROM catalog_mappings WHERE catalog_name = ?`
	
	var tableName string
	err := db.QueryRow(query, catalogName).Scan(&tableName)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("catalog mapping not found for: %s", catalogName)
	}
	if err != nil {
		return "", fmt.Errorf("failed to get catalog table name: %w", err)
	}

	return tableName, nil
}

// SaveCatalogMapping сохраняет маппинг справочника на таблицу
func SaveCatalogMapping(db *sql.DB, catalogName, tableName string) error {
	query := `INSERT OR IGNORE INTO catalog_mappings (catalog_name, table_name) VALUES (?, ?)`
	
	_, err := db.Exec(query, catalogName, tableName)
	if err != nil {
		return fmt.Errorf("failed to save catalog mapping: %w", err)
	}

	return nil
}

// RemoveAccents удаляет диакритические знаки из строки
func RemoveAccents(s string) string {
	t := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	result, _, _ := transform.String(t, s)
	return result
}

// GetCatalogNameFromTable получает оригинальное имя справочника по имени таблицы
func GetCatalogNameFromTable(db *sql.DB, tableName string) (string, error) {
	query := `SELECT catalog_name FROM catalog_mappings WHERE table_name = ?`
	
	var catalogName string
	err := db.QueryRow(query, tableName).Scan(&catalogName)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("catalog name not found for table: %s", tableName)
	}
	if err != nil {
		return "", fmt.Errorf("failed to get catalog name: %w", err)
	}

	return catalogName, nil
}

// GetAllMappings возвращает все маппинги справочников
func GetAllMappings(db *sql.DB) (map[string]string, error) {
	return GetAllCatalogTables(db)
}


