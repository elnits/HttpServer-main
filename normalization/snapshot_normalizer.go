package normalization

import (
	"sort"
	"strings"

	"httpserver/database"
)

// SnapshotNormalizer нормализатор для работы со срезами данных
type SnapshotNormalizer struct {
	nameNormalizer *NameNormalizer
}

// NewSnapshotNormalizer создает новый нормализатор срезов
func NewSnapshotNormalizer() *SnapshotNormalizer {
	return &SnapshotNormalizer{
		nameNormalizer: NewNameNormalizer(),
	}
}

// NormalizedItem представляет нормализованный элемент
type NormalizedItem struct {
	SourceReference       string `json:"source_reference"`
	SourceName            string `json:"source_name"`
	Code                  string `json:"code"`
	NormalizedName        string `json:"normalized_name"`
	NormalizedReference   string `json:"normalized_reference"`
	Category              string `json:"category"`
	MergedCount           int    `json:"merged_count"`
	SourceDatabaseID      int    `json:"source_database_id"`
	SourceIterationNumber int    `json:"source_iteration_number"`
}

// NormalizationChanges представляет изменения после нормализации
type NormalizationChanges struct {
	Added   int `json:"added"`
	Updated int `json:"updated"`
	Deleted int `json:"deleted"`
}

// UploadNormalizationResult представляет результат нормализации одной выгрузки в срезе
type UploadNormalizationResult struct {
	UploadID       int                  `json:"upload_id"`
	ProcessedCount int                  `json:"processed_count"`
	GroupCount     int                  `json:"group_count"`
	NormalizedData []NormalizedItem     `json:"normalized_data,omitempty"`
	Changes        *NormalizationChanges `json:"changes,omitempty"`
	Error          string               `json:"error,omitempty"`
}

// SnapshotNormalizationResult представляет результат нормализации среза
type SnapshotNormalizationResult struct {
	SnapshotID      int                                  `json:"snapshot_id"`
	MasterReference map[string]string                    `json:"master_reference"` // normalized_name -> reference
	UploadResults   map[int]*UploadNormalizationResult   `json:"upload_results"`
	TotalProcessed  int                                  `json:"total_processed"`
	TotalGroups     int                                  `json:"total_groups"`
	CompletedAt     string                               `json:"completed_at"`
}

// NormalizeSnapshot выполняет сквозную нормализацию среза
func (sn *SnapshotNormalizer) NormalizeSnapshot(db *database.DB, snapshot *database.DataSnapshot, uploads []*database.Upload) (*SnapshotNormalizationResult, error) {
	// Извлекаем все элементы из всех выгрузок
	allItems, err := sn.extractAllItems(db, uploads)
	if err != nil {
		return nil, err
	}

	// Создаем общий эталон на основе всех элементов
	masterReference := sn.createMasterReference(allItems)

	// Нормализуем каждую выгрузку отдельно с использованием общего эталона
	uploadResults := make(map[int]*UploadNormalizationResult)
	totalProcessed := 0
	totalGroups := len(masterReference)

	for _, upload := range uploads {
		result, err := sn.normalizeUpload(db, upload, masterReference)
		if err != nil {
			uploadResults[upload.ID] = &UploadNormalizationResult{
				UploadID: upload.ID,
				Error:    err.Error(),
			}
			continue
		}
		uploadResults[upload.ID] = result
		totalProcessed += result.ProcessedCount
	}

	return &SnapshotNormalizationResult{
		SnapshotID:      snapshot.ID,
		MasterReference: masterReference,
		UploadResults:   uploadResults,
		TotalProcessed:  totalProcessed,
		TotalGroups:     totalGroups,
	}, nil
}

// extractAllItems извлекает все элементы из всех выгрузок
func (sn *SnapshotNormalizer) extractAllItems(db *database.DB, uploads []*database.Upload) ([]NormalizedItem, error) {
	var allItems []NormalizedItem
	for _, upload := range uploads {
		// Получаем все элементы выгрузки (без пагинации)
		items, _, err := db.GetCatalogItemsByUpload(upload.ID, nil, 0, 0)
		if err != nil {
			return nil, err
		}

		databaseID := 0
		if upload.DatabaseID != nil {
			databaseID = *upload.DatabaseID
		}

		for _, item := range items {
			allItems = append(allItems, NormalizedItem{
				SourceReference:       item.Reference,
				SourceName:            item.Name,
				Code:                  item.Code,
				SourceDatabaseID:      databaseID,
				SourceIterationNumber: upload.IterationNumber,
			})
		}
	}
	return allItems, nil
}

// createMasterReference создает общий эталон нормализации на основе всех элементов
func (sn *SnapshotNormalizer) createMasterReference(items []NormalizedItem) map[string]string {
	master := make(map[string]string)
	seenNames := make(map[string]bool)

	// Сортируем элементы по имени для последовательной обработки
	sort.Slice(items, func(i, j int) bool {
		return strings.ToLower(items[i].SourceName) < strings.ToLower(items[j].SourceName)
	})

	for _, item := range items {
		normalizedName := sn.nameNormalizer.NormalizeName(item.SourceName)
		if normalizedName == "" {
			continue
		}
		if !seenNames[normalizedName] {
			master[normalizedName] = item.SourceReference
			seenNames[normalizedName] = true
		}
	}

	return master
}

// normalizeUpload нормализует одну выгрузку с использованием общего эталона
func (sn *SnapshotNormalizer) normalizeUpload(db *database.DB, upload *database.Upload, masterReference map[string]string) (*UploadNormalizationResult, error) {
	// Получаем все элементы выгрузки
	items, _, err := db.GetCatalogItemsByUpload(upload.ID, nil, 0, 0)
	if err != nil {
		return nil, err
	}

	var normalizedData []NormalizedItem
	databaseID := 0
	if upload.DatabaseID != nil {
		databaseID = *upload.DatabaseID
	}

	for _, item := range items {
		normalizedName := sn.nameNormalizer.NormalizeName(item.Name)
		if normalizedName == "" {
			normalizedName = item.Name // Используем исходное имя, если нормализация дала пустую строку
		}

		normalizedReference, exists := masterReference[normalizedName]
		if !exists {
			// Если не найдено в эталоне, используем исходные данные
			normalizedReference = item.Reference
		}

		normalizedData = append(normalizedData, NormalizedItem{
			SourceReference:       item.Reference,
			SourceName:            item.Name,
			Code:                  item.Code,
			NormalizedName:        normalizedName,
			NormalizedReference:   normalizedReference,
			Category:              item.CatalogName, // Используем имя справочника как категорию
			MergedCount:           1,
			SourceDatabaseID:      databaseID,
			SourceIterationNumber: upload.IterationNumber,
		})
	}

	// Вычисляем изменения
	changes := sn.calculateChanges(items, normalizedData)

	// Подсчитываем группы (уникальные normalized_name)
	uniqueGroups := make(map[string]bool)
	for _, item := range normalizedData {
		uniqueGroups[item.NormalizedName] = true
	}

	return &UploadNormalizationResult{
		UploadID:       upload.ID,
		ProcessedCount: len(normalizedData),
		GroupCount:     len(uniqueGroups),
		NormalizedData: normalizedData,
		Changes:        changes,
	}, nil
}

// calculateChanges вычисляет изменения после нормализации
func (sn *SnapshotNormalizer) calculateChanges(original []*database.CatalogItem, normalized []NormalizedItem) *NormalizationChanges {
	changes := &NormalizationChanges{}
	// Простая логика: считаем, что все элементы обновлены, если их normalized_name отличается от исходного имени
	for i, norm := range normalized {
		if i < len(original) {
			originalName := strings.ToLower(strings.TrimSpace(original[i].Name))
			normalizedName := strings.ToLower(strings.TrimSpace(norm.NormalizedName))
			if originalName != normalizedName {
				changes.Updated++
			}
		}
	}
	return changes
}

