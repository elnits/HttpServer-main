package server

import (
	"sort"

	"httpserver/normalization"
)

// compareSnapshotIterations сравнивает итерации в срезе
func (s *Server) compareSnapshotIterations(snapshotID int) (*SnapshotComparisonResponse, error) {
	_, uploads, err := s.db.GetSnapshotWithUploads(snapshotID)
	if err != nil {
		return nil, err
	}

	var iterations []IterationComparison
	totalItemsMap := make(map[int]int)

	for _, upload := range uploads {
		// Получаем элементы выгрузки
		items, _, err := s.db.GetCatalogItemsByUpload(upload.ID, nil, 0, 0)
		if err != nil {
			return nil, err
		}

		// Вычисляем метрики для этой итерации
		uniqueItems := make(map[string]bool)
		for _, item := range items {
			normalizedName := normalization.NewNameNormalizer().NormalizeName(item.Name)
			if normalizedName != "" {
				uniqueItems[normalizedName] = true
			}
		}

		totalItemsMap[upload.ID] = len(items)

		iteration := IterationComparison{
			UploadID:        upload.ID,
			IterationNumber: upload.IterationNumber,
			IterationLabel:  upload.IterationLabel,
			StartedAt:        upload.StartedAt,
			TotalItems:       len(items),
			TotalCatalogs:    upload.TotalCatalogs,
			DatabaseID:       upload.DatabaseID,
		}
		iterations = append(iterations, iteration)
	}

	// Сортируем итерации по номеру
	sort.Slice(iterations, func(i, j int) bool {
		return iterations[i].IterationNumber < iterations[j].IterationNumber
	})

	return &SnapshotComparisonResponse{
		SnapshotID: snapshotID,
		Iterations: iterations,
		TotalItems: totalItemsMap,
	}, nil
}

// calculateSnapshotMetrics вычисляет метрики для среза
func (s *Server) calculateSnapshotMetrics(snapshotID int) (*SnapshotMetricsResponse, error) {
	normalizedData, err := s.db.GetSnapshotNormalizedData(snapshotID)
	if err != nil {
		return nil, err
	}

	// Вычисляем общие метрики
	totalItems := len(normalizedData)
	uniqueItems := make(map[string]bool)
	qualityScores := make(map[int]float64)

	// Группируем по upload_id для вычисления метрик по выгрузкам
	itemsByUpload := make(map[int][]map[string]interface{})
	for _, item := range normalizedData {
		uploadID, ok := item["upload_id"].(int)
		if !ok {
			// Пытаемся получить как interface{} и преобразовать
			if uploadIDInterface, ok2 := item["upload_id"]; ok2 {
				if uploadIDInt, ok3 := uploadIDInterface.(int); ok3 {
					uploadID = uploadIDInt
				} else if uploadIDFloat, ok4 := uploadIDInterface.(float64); ok4 {
					uploadID = int(uploadIDFloat)
				} else {
					continue
				}
			} else {
				continue
			}
		}
		itemsByUpload[uploadID] = append(itemsByUpload[uploadID], item)

		normalizedName, _ := item["normalized_name"].(string)
		if normalizedName != "" {
			uniqueItems[normalizedName] = true
		}
	}

	// Вычисляем quality score для каждой выгрузки
	for uploadID, items := range itemsByUpload {
		uniqueInUpload := make(map[string]bool)
		for _, item := range items {
			normalizedName, _ := item["normalized_name"].(string)
			if normalizedName != "" {
				uniqueInUpload[normalizedName] = true
			}
		}
		if len(items) > 0 {
			qualityScores[uploadID] = float64(len(uniqueInUpload)) / float64(len(items))
		}
	}

	duplicateRate := 1.0 - float64(len(uniqueItems))/float64(totalItems)
	if duplicateRate < 0 {
		duplicateRate = 0
	}

	// Определяем общий тренд
	overallTrend := "stable"
	if len(qualityScores) > 1 {
		scores := make([]float64, 0, len(qualityScores))
		for _, score := range qualityScores {
			scores = append(scores, score)
		}
		sort.Float64s(scores)
		if len(scores) >= 2 {
			if scores[len(scores)-1] > scores[0]+0.1 {
				overallTrend = "improving"
			} else if scores[len(scores)-1] < scores[0]-0.1 {
				overallTrend = "degrading"
			}
		}
	}

	return &SnapshotMetricsResponse{
		SnapshotID:    snapshotID,
		QualityScores: qualityScores,
		OverallTrend:  overallTrend,
	}, nil
}

// getSnapshotEvolution возвращает данные об эволюции номенклатуры в срезе
func (s *Server) getSnapshotEvolution(snapshotID int) (*SnapshotEvolutionResponse, error) {
	_, uploads, err := s.db.GetSnapshotWithUploads(snapshotID)
	if err != nil {
		return nil, err
	}

	// Сортируем выгрузки по номеру итерации
	sort.Slice(uploads, func(i, j int) bool {
		return uploads[i].IterationNumber < uploads[j].IterationNumber
	})

	// Собираем историю элементов
	itemHistory := make(map[string][]NomenclatureHistoryItem) // normalized_name -> история
	nameNormalizer := normalization.NewNameNormalizer()

	for _, upload := range uploads {
		items, _, err := s.db.GetCatalogItemsByUpload(upload.ID, nil, 0, 0)
		if err != nil {
			return nil, err
		}

		for _, item := range items {
			normalizedName := nameNormalizer.NormalizeName(item.Name)
			if normalizedName == "" {
				normalizedName = item.Name // Используем исходное имя, если нормализация дала пустую строку
			}

			itemHistory[normalizedName] = append(itemHistory[normalizedName], NomenclatureHistoryItem{
				UploadID:        upload.ID,
				IterationNumber: upload.IterationNumber,
				Name:            item.Name,
				Category:        item.CatalogName,
				ChangedAt:       upload.StartedAt,
			})
		}
	}

	// Формируем эволюцию
	var evolution []NomenclatureEvolution
	for normalizedName, history := range itemHistory {
		if len(history) == 0 {
			continue
		}

		// Определяем статус
		status := "stable"
		if len(history) == 1 {
			status = "new"
		} else {
			// Проверяем, были ли изменения в названиях
			firstName := history[0].Name
			hasChanges := false
			for _, h := range history[1:] {
				if h.Name != firstName {
					hasChanges = true
					break
				}
			}
			if hasChanges {
				status = "modified"
			}
		}

		evolution = append(evolution, NomenclatureEvolution{
			Reference: normalizedName,
			Name:      history[0].Name, // Используем первое имя
			History:   history,
			Status:    status,
		})
	}

	// Ограничиваем количество элементов для производительности
	if len(evolution) > 1000 {
		evolution = evolution[:1000]
	}

	return &SnapshotEvolutionResponse{
		SnapshotID:    snapshotID,
		Evolution:     evolution,
		TotalTracked: len(evolution),
	}, nil
}

