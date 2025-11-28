package database

import (
	"encoding/json"
	"fmt"
	"strings"
)

// UpdateNomenclatureItemClassification обновляет классификацию элемента номенклатуры
func (db *DB) UpdateNomenclatureItemClassification(itemID int, categoryOriginal []string, categoryLevels map[string]string, classificationStrategy string, confidence float64) error {
	// Сериализуем оригинальный путь
	originalJSON, err := json.Marshal(categoryOriginal)
	if err != nil {
		return fmt.Errorf("failed to marshal original path: %w", err)
	}

	// Извлекаем уровни
	categoryLevel1 := ""
	categoryLevel2 := ""
	categoryLevel3 := ""
	categoryLevel4 := ""
	categoryLevel5 := ""

	if level1, exists := categoryLevels["level1"]; exists {
		categoryLevel1 = level1
	}
	if level2, exists := categoryLevels["level2"]; exists {
		categoryLevel2 = level2
	}
	if level3, exists := categoryLevels["level3"]; exists {
		categoryLevel3 = level3
	}
	if level4, exists := categoryLevels["level4"]; exists {
		categoryLevel4 = level4
	}
	if level5, exists := categoryLevels["level5"]; exists {
		categoryLevel5 = level5
	}

	// Проверяем наличие полей, если нет - добавляем
	if err := db.ensureNomenclatureCategoryColumns(); err != nil {
		return fmt.Errorf("failed to ensure category columns: %w", err)
	}

	query := `
		UPDATE nomenclature_items 
		SET category_original = ?,
		    category_level1 = ?,
		    category_level2 = ?,
		    category_level3 = ?,
		    category_level4 = ?,
		    category_level5 = ?,
		    classification_strategy = ?,
		    classification_confidence = ?
		WHERE id = ?
	`

	_, err = db.conn.Exec(query,
		string(originalJSON),
		categoryLevel1,
		categoryLevel2,
		categoryLevel3,
		categoryLevel4,
		categoryLevel5,
		classificationStrategy,
		confidence,
		itemID,
	)

	if err != nil {
		return fmt.Errorf("failed to update nomenclature classification: %w", err)
	}

	return nil
}

// HasNomenclatureClassification проверяет, есть ли уже классификация у элемента номенклатуры
func (db *DB) HasNomenclatureClassification(itemID int) (bool, error) {
	// Проверяем наличие полей
	if err := db.ensureNomenclatureCategoryColumns(); err != nil {
		return false, fmt.Errorf("failed to ensure category columns: %w", err)
	}

	query := `
		SELECT COUNT(*) 
		FROM nomenclature_items 
		WHERE id = ? AND category_level1 IS NOT NULL AND category_level1 != ''
	`

	var count int
	err := db.conn.QueryRow(query, itemID).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check classification: %w", err)
	}

	return count > 0, nil
}

// ensureNomenclatureCategoryColumns добавляет поля категорий в таблицу nomenclature_items если их нет
func (db *DB) ensureNomenclatureCategoryColumns() error {
	migrations := []string{
		`ALTER TABLE nomenclature_items ADD COLUMN category_original TEXT`,
		`ALTER TABLE nomenclature_items ADD COLUMN category_level1 TEXT`,
		`ALTER TABLE nomenclature_items ADD COLUMN category_level2 TEXT`,
		`ALTER TABLE nomenclature_items ADD COLUMN category_level3 TEXT`,
		`ALTER TABLE nomenclature_items ADD COLUMN category_level4 TEXT`,
		`ALTER TABLE nomenclature_items ADD COLUMN category_level5 TEXT`,
		`ALTER TABLE nomenclature_items ADD COLUMN classification_strategy TEXT`,
		`ALTER TABLE nomenclature_items ADD COLUMN classification_confidence REAL`,
	}

	for _, migration := range migrations {
		_, err := db.conn.Exec(migration)
		if err != nil {
			errStr := strings.ToLower(err.Error())
			if !strings.Contains(errStr, "duplicate column") && !strings.Contains(errStr, "already exists") {
				// Игнорируем ошибки если колонки уже существуют
				return fmt.Errorf("migration failed: %s, error: %w", migration, err)
			}
		}
	}

	return nil
}

// GetNomenclatureItemsForClassification получает список номенклатур для классификации
func (db *DB) GetNomenclatureItemsForClassification(limit, offset int) ([]struct {
	ID       int
	Ref      string
	Code     string
	Name     string
}, error) {
	query := `
		SELECT DISTINCT id, nomenclature_reference, nomenclature_code, nomenclature_name
		FROM nomenclature_items
		WHERE nomenclature_name IS NOT NULL AND nomenclature_name != ''
		ORDER BY id
		LIMIT ? OFFSET ?
	`

	rows, err := db.conn.Query(query, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to query nomenclature items: %w", err)
	}
	defer rows.Close()

	var items []struct {
		ID       int
		Ref      string
		Code     string
		Name     string
	}

	for rows.Next() {
		var item struct {
			ID       int
			Ref      string
			Code     string
			Name     string
		}
		if err := rows.Scan(&item.ID, &item.Ref, &item.Code, &item.Name); err != nil {
			return nil, fmt.Errorf("failed to scan nomenclature item: %w", err)
		}
		items = append(items, item)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating nomenclature items: %w", err)
	}

	return items, nil
}

// GetNomenclatureItemsCount получает общее количество номенклатур
func (db *DB) GetNomenclatureItemsCount() (int, error) {
	query := `
		SELECT COUNT(DISTINCT id)
		FROM nomenclature_items
		WHERE nomenclature_name IS NOT NULL AND nomenclature_name != ''
	`

	var count int
	err := db.conn.QueryRow(query).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count nomenclature items: %w", err)
	}

	return count, nil
}

