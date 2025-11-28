package database

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// ============================================================================
// Classification Data Structures
// ============================================================================

// CategoryClassifier представляет классификатор категорий
type CategoryClassifier struct {
	ID            int       `json:"id"`
	Name          string    `json:"name"`
	Description   string    `json:"description"`
	MaxDepth      int       `json:"max_depth"`
	TreeStructure string    `json:"tree_structure"` // JSON с деревом категорий
	ClientID      *int      `json:"client_id,omitempty"`
	ProjectID     *int      `json:"project_id,omitempty"`
	IsActive      bool      `json:"is_active"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// FoldingStrategy представляет стратегию свертки категорий
type FoldingStrategy struct {
	ID             int       `json:"id"`
	Name           string    `json:"name"`
	Description    string    `json:"description"`
	StrategyConfig string    `json:"strategy_config"` // JSON конфигурация стратегии
	ClientID       *int      `json:"client_id,omitempty"`
	IsDefault      bool      `json:"is_default"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// CreateCategoryClassifier создает новый классификатор категорий
func (db *DB) CreateCategoryClassifier(classifier *CategoryClassifier) (*CategoryClassifier, error) {
	treeJSON := ""
	if classifier.TreeStructure != "" {
		treeJSON = classifier.TreeStructure
	}

	query := `
		INSERT INTO category_classifiers (name, description, max_depth, tree_structure, client_id, project_id, is_active)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`

	result, err := db.conn.Exec(query, classifier.Name, classifier.Description, classifier.MaxDepth, treeJSON, classifier.ClientID, classifier.ProjectID, classifier.IsActive)
	if err != nil {
		return nil, fmt.Errorf("failed to create category classifier: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("failed to get classifier ID: %w", err)
	}

	return db.GetCategoryClassifier(int(id))
}

// GetCategoryClassifier получает классификатор по ID
func (db *DB) GetCategoryClassifier(id int) (*CategoryClassifier, error) {
	query := `
		SELECT id, name, description, max_depth, tree_structure, client_id, project_id, is_active, created_at, updated_at
		FROM category_classifiers WHERE id = ?
	`

	classifier := &CategoryClassifier{}
	var clientID, projectID sql.NullInt64
	err := db.conn.QueryRow(query, id).Scan(
		&classifier.ID,
		&classifier.Name,
		&classifier.Description,
		&classifier.MaxDepth,
		&classifier.TreeStructure,
		&clientID,
		&projectID,
		&classifier.IsActive,
		&classifier.CreatedAt,
		&classifier.UpdatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get category classifier: %w", err)
	}

	if clientID.Valid {
		id := int(clientID.Int64)
		classifier.ClientID = &id
	}
	if projectID.Valid {
		id := int(projectID.Int64)
		classifier.ProjectID = &id
	}

	return classifier, nil
}

// GetActiveCategoryClassifiers получает все активные классификаторы
func (db *DB) GetActiveCategoryClassifiers() ([]*CategoryClassifier, error) {
	return db.GetCategoryClassifiersByFilter(nil, nil, true)
}

// GetCategoryClassifiersByFilter получает классификаторы с фильтрацией по клиенту и проекту
func (db *DB) GetCategoryClassifiersByFilter(clientID *int, projectID *int, activeOnly bool) ([]*CategoryClassifier, error) {
	query := `
		SELECT id, name, description, max_depth, tree_structure, client_id, project_id, is_active, created_at, updated_at
		FROM category_classifiers WHERE 1=1
	`
	args := []interface{}{}

	if activeOnly {
		query += " AND is_active = TRUE"
	}

	if clientID != nil {
		query += " AND (client_id = ? OR client_id IS NULL)"
		args = append(args, *clientID)
	}

	if projectID != nil {
		query += " AND (project_id = ? OR project_id IS NULL)"
		args = append(args, *projectID)
	}

	query += " ORDER BY created_at DESC"

	rows, err := db.conn.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to get active category classifiers: %w", err)
	}
	defer rows.Close()

	var classifiers []*CategoryClassifier
	for rows.Next() {
		classifier := &CategoryClassifier{}
		var clientID, projectID sql.NullInt64
		err := rows.Scan(
			&classifier.ID,
			&classifier.Name,
			&classifier.Description,
			&classifier.MaxDepth,
			&classifier.TreeStructure,
			&clientID,
			&projectID,
			&classifier.IsActive,
			&classifier.CreatedAt,
			&classifier.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan category classifier: %w", err)
		}

		if clientID.Valid {
			id := int(clientID.Int64)
			classifier.ClientID = &id
		}
		if projectID.Valid {
			id := int(projectID.Int64)
			classifier.ProjectID = &id
		}

		classifiers = append(classifiers, classifier)
	}

	return classifiers, nil
}

// UpdateCategoryClassifier обновляет классификатор категорий
func (db *DB) UpdateCategoryClassifier(classifier *CategoryClassifier) error {
	query := `
		UPDATE category_classifiers 
		SET name = ?, description = ?, max_depth = ?, tree_structure = ?, client_id = ?, project_id = ?, is_active = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`

	_, err := db.conn.Exec(query,
		classifier.Name,
		classifier.Description,
		classifier.MaxDepth,
		classifier.TreeStructure,
		classifier.ClientID,
		classifier.ProjectID,
		classifier.IsActive,
		classifier.ID,
	)

	if err != nil {
		return fmt.Errorf("failed to update category classifier: %w", err)
	}

	return nil
}

// CreateFoldingStrategy создает новую стратегию свертки
func (db *DB) CreateFoldingStrategy(strategy *FoldingStrategy) (*FoldingStrategy, error) {
	query := `
		INSERT INTO folding_strategies (name, description, strategy_config, client_id, is_default)
		VALUES (?, ?, ?, ?, ?)
	`

	result, err := db.conn.Exec(query, strategy.Name, strategy.Description, strategy.StrategyConfig, strategy.ClientID, strategy.IsDefault)
	if err != nil {
		return nil, fmt.Errorf("failed to create folding strategy: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("failed to get strategy ID: %w", err)
	}

	return db.GetFoldingStrategy(int(id))
}

// GetFoldingStrategy получает стратегию свертки по ID
func (db *DB) GetFoldingStrategy(id int) (*FoldingStrategy, error) {
	query := `
		SELECT id, name, description, strategy_config, client_id, is_default, created_at, updated_at
		FROM folding_strategies WHERE id = ?
	`

	strategy := &FoldingStrategy{}
	var clientID sql.NullInt64

	err := db.conn.QueryRow(query, id).Scan(
		&strategy.ID,
		&strategy.Name,
		&strategy.Description,
		&strategy.StrategyConfig,
		&clientID,
		&strategy.IsDefault,
		&strategy.CreatedAt,
		&strategy.UpdatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get folding strategy: %w", err)
	}

	if clientID.Valid {
		id := int(clientID.Int64)
		strategy.ClientID = &id
	}

	return strategy, nil
}

// GetFoldingStrategiesByClient получает стратегии свертки для клиента
func (db *DB) GetFoldingStrategiesByClient(clientID int) ([]*FoldingStrategy, error) {
	query := `
		SELECT id, name, description, strategy_config, client_id, is_default, created_at, updated_at
		FROM folding_strategies WHERE client_id = ?
		ORDER BY is_default DESC, created_at DESC
	`

	rows, err := db.conn.Query(query, clientID)
	if err != nil {
		return nil, fmt.Errorf("failed to get folding strategies by client: %w", err)
	}
	defer rows.Close()

	var strategies []*FoldingStrategy
	for rows.Next() {
		strategy := &FoldingStrategy{}
		var clientID sql.NullInt64

		err := rows.Scan(
			&strategy.ID,
			&strategy.Name,
			&strategy.Description,
			&strategy.StrategyConfig,
			&clientID,
			&strategy.IsDefault,
			&strategy.CreatedAt,
			&strategy.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan folding strategy: %w", err)
		}

		if clientID.Valid {
			id := int(clientID.Int64)
			strategy.ClientID = &id
		}

		strategies = append(strategies, strategy)
	}

	return strategies, nil
}

// GetDefaultFoldingStrategy получает стратегию свертки по умолчанию для клиента
func (db *DB) GetDefaultFoldingStrategy(clientID int) (*FoldingStrategy, error) {
	query := `
		SELECT id, name, description, strategy_config, client_id, is_default, created_at, updated_at
		FROM folding_strategies 
		WHERE client_id = ? AND is_default = TRUE
		LIMIT 1
	`

	strategy := &FoldingStrategy{}
	var clientIDNull sql.NullInt64

	err := db.conn.QueryRow(query, clientID).Scan(
		&strategy.ID,
		&strategy.Name,
		&strategy.Description,
		&strategy.StrategyConfig,
		&clientIDNull,
		&strategy.IsDefault,
		&strategy.CreatedAt,
		&strategy.UpdatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get default folding strategy: %w", err)
	}

	if clientIDNull.Valid {
		id := int(clientIDNull.Int64)
		strategy.ClientID = &id
	}

	return strategy, nil
}

// UpdateFoldingStrategy обновляет стратегию свертки
func (db *DB) UpdateFoldingStrategy(strategy *FoldingStrategy) error {
	query := `
		UPDATE folding_strategies 
		SET name = ?, description = ?, strategy_config = ?, client_id = ?, is_default = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`

	_, err := db.conn.Exec(query,
		strategy.Name,
		strategy.Description,
		strategy.StrategyConfig,
		strategy.ClientID,
		strategy.IsDefault,
		strategy.ID,
	)

	if err != nil {
		return fmt.Errorf("failed to update folding strategy: %w", err)
	}

	return nil
}

// ============================================================================
// Catalog Item Classification Methods
// ============================================================================

// UpdateCatalogItemClassification обновляет классификацию элемента справочника
func (db *DB) UpdateCatalogItemClassification(catalogItemID int, categoryOriginal string, categoryLevels map[string]string, classificationStrategy string, confidence float64) error {
	// Извлекаем уровни категорий
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

	query := `
		UPDATE catalog_items 
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

	_, err := db.conn.Exec(query,
		categoryOriginal,
		categoryLevel1,
		categoryLevel2,
		categoryLevel3,
		categoryLevel4,
		categoryLevel5,
		classificationStrategy,
		confidence,
		catalogItemID,
	)

	if err != nil {
		return fmt.Errorf("failed to update catalog item classification: %w", err)
	}

	return nil
}

// GetCatalogItemClassification получает классификацию элемента справочника
func (db *DB) GetCatalogItemClassification(catalogItemID int) (map[string]interface{}, error) {
	query := `
		SELECT category_original, category_level1, category_level2, category_level3, category_level4, category_level5, classification_strategy, classification_confidence
		FROM catalog_items WHERE id = ?
	`

	var categoryOriginal, strategy sql.NullString
	var confidence sql.NullFloat64
	var categoryLevel1, categoryLevel2, categoryLevel3, categoryLevel4, categoryLevel5 sql.NullString

	err := db.conn.QueryRow(query, catalogItemID).Scan(
		&categoryOriginal,
		&categoryLevel1,
		&categoryLevel2,
		&categoryLevel3,
		&categoryLevel4,
		&categoryLevel5,
		&strategy,
		&confidence,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get catalog item classification: %w", err)
	}

	result := make(map[string]interface{})

	if categoryOriginal.Valid {
		result["category_original"] = categoryOriginal.String
	}

	categoryLevels := make(map[string]string)
	if categoryLevel1.Valid {
		categoryLevels["level1"] = categoryLevel1.String
	}
	if categoryLevel2.Valid {
		categoryLevels["level2"] = categoryLevel2.String
	}
	if categoryLevel3.Valid {
		categoryLevels["level3"] = categoryLevel3.String
	}
	if categoryLevel4.Valid {
		categoryLevels["level4"] = categoryLevel4.String
	}
	if categoryLevel5.Valid {
		categoryLevels["level5"] = categoryLevel5.String
	}

	result["category_levels"] = categoryLevels

	if strategy.Valid {
		result["classification_strategy"] = strategy.String
	}

	if confidence.Valid {
		result["classification_confidence"] = confidence.Float64
	}

	return result, nil
}

// GetCatalogItemsByCategory получает элементы справочника по категории
func (db *DB) GetCatalogItemsByCategory(level1, level2 string, limit, offset int) ([]*CatalogItem, int, error) {
	query := `
		SELECT ci.id, ci.catalog_id, c.name as catalog_name, ci.reference, ci.code, ci.name,
		       ci.category_original, ci.category_level1, ci.category_level2, ci.category_level3, 
		       ci.category_level4, ci.category_level5, ci.classification_strategy, ci.classification_confidence,
		       ci.created_at
		FROM catalog_items ci
		INNER JOIN catalogs c ON ci.catalog_id = c.id
		WHERE ci.category_level1 = ?
	`

	args := []interface{}{level1}

	if level2 != "" {
		query += " AND ci.category_level2 = ?"
		args = append(args, level2)
	}

	// Получаем общее количество
	countQuery := strings.Replace(query, "SELECT ci.id, ci.catalog_id, c.name as catalog_name, ci.reference, ci.code, ci.name,\n\t\t       ci.category_original, ci.category_level1, ci.category_level2, ci.category_level3, \n\t\t       ci.category_level4, ci.category_level5, ci.classification_strategy, ci.classification_confidence,\n\t\t       ci.created_at", "SELECT COUNT(*)", 1)

	var totalCount int
	err := db.conn.QueryRow(countQuery, args...).Scan(&totalCount)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get total count: %w", err)
	}

	// Добавляем сортировку и пагинацию
	query += " ORDER BY ci.category_level2, ci.category_level3, ci.name"

	if limit > 0 {
		query += " LIMIT ? OFFSET ?"
		args = append(args, limit, offset)
	}

	rows, err := db.conn.Query(query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get catalog items by category: %w", err)
	}
	defer rows.Close()

	var items []*CatalogItem
	for rows.Next() {
		item := &CatalogItem{}
		var categoryOriginal, categoryLevel1, categoryLevel2, categoryLevel3, categoryLevel4, categoryLevel5, classificationStrategy sql.NullString
		var classificationConfidence sql.NullFloat64
		err := rows.Scan(
			&item.ID, &item.CatalogID, &item.CatalogName, &item.Reference, &item.Code, &item.Name,
			&categoryOriginal, &categoryLevel1, &categoryLevel2, &categoryLevel3, &categoryLevel4, &categoryLevel5,
			&classificationStrategy, &classificationConfidence, &item.CreatedAt,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan catalog item: %w", err)
		}
		items = append(items, item)
	}

	return items, totalCount, nil
}

// GetClassificationStats получает статистику по классификации
func (db *DB) GetClassificationStats() (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// Общее количество элементов с классификацией
	var totalClassified int
	err := db.conn.QueryRow("SELECT COUNT(*) FROM catalog_items WHERE classification_strategy IS NOT NULL").Scan(&totalClassified)
	if err != nil {
		return nil, fmt.Errorf("failed to get total classified count: %w", err)
	}
	stats["total_classified"] = totalClassified

	// Статистика по уровням категорий
	levelStats := make(map[string]int)

	for level := 1; level <= 5; level++ {
		var count int
		err := db.conn.QueryRow(fmt.Sprintf("SELECT COUNT(DISTINCT category_level%d) FROM catalog_items WHERE category_level%d IS NOT NULL AND category_level%d != ''", level, level, level)).Scan(&count)
		if err != nil {
			return nil, fmt.Errorf("failed to get level %d stats: %w", level, err)
		}
		levelStats[fmt.Sprintf("level%d", level)] = count
	}
	stats["categories_by_level"] = levelStats

	// Статистика по стратегиям классификации
	strategyRows, err := db.conn.Query(`
		SELECT classification_strategy, COUNT(*) as count
		FROM catalog_items 
		WHERE classification_strategy IS NOT NULL
		GROUP BY classification_strategy
		ORDER BY count DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to get strategy stats: %w", err)
	}
	defer strategyRows.Close()

	strategyStats := make(map[string]int)
	for strategyRows.Next() {
		var strategy string
		var count int
		if err := strategyRows.Scan(&strategy, &count); err != nil {
			return nil, fmt.Errorf("failed to scan strategy stats: %w", err)
		}
		strategyStats[strategy] = count
	}
	stats["by_strategy"] = strategyStats

	// Средняя уверенность классификации
	var avgConfidence float64
	err = db.conn.QueryRow("SELECT AVG(classification_confidence) FROM catalog_items WHERE classification_confidence > 0").Scan(&avgConfidence)
	if err != nil && err.Error() != "sql: Scan error on column index 0, name \"AVG(classification_confidence)\": converting NULL to float64 is unsupported" {
		return nil, fmt.Errorf("failed to get avg confidence: %w", err)
	}
	stats["average_confidence"] = avgConfidence

	return stats, nil
}

// ============================================================================
// Normalization Session Classification Integration
// ============================================================================

// UpdateSessionClassificationContext обновляет контекст классификации для стадии нормализации
func (db *DB) UpdateSessionClassificationContext(stageID int, categoryOriginal string, categoryFolded string, classificationStrategy string) error {
	query := `
		UPDATE normalization_stages 
		SET category_original = ?, category_folded = ?, classification_strategy = ?
		WHERE id = ?
	`

	_, err := db.conn.Exec(query, categoryOriginal, categoryFolded, classificationStrategy, stageID)
	if err != nil {
		return fmt.Errorf("failed to update session classification context: %w", err)
	}

	return nil
}

// GetSessionClassificationContext получает контекст классификации для сессии
func (db *DB) GetSessionClassificationContext(sessionID int) (map[string]interface{}, error) {
	query := `
		SELECT category_original, category_folded, classification_strategy
		FROM normalization_stages 
		WHERE session_id = ?
		ORDER BY created_at DESC
		LIMIT 1
	`

	var categoryOriginal, categoryFolded, classificationStrategy sql.NullString

	err := db.conn.QueryRow(query, sessionID).Scan(
		&categoryOriginal,
		&categoryFolded,
		&classificationStrategy,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get session classification context: %w", err)
	}

	result := make(map[string]interface{})

	if categoryOriginal.Valid {
		result["category_original"] = categoryOriginal.String
	}

	if categoryFolded.Valid {
		result["category_folded"] = categoryFolded.String
	}

	if classificationStrategy.Valid {
		result["classification_strategy"] = classificationStrategy.String
	}

	return result, nil
}
