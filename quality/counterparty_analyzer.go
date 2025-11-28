package quality

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

	"httpserver/database"
)

// analyzeCounterparties анализирует качество данных контрагентов
func (qa *QualityAnalyzer) analyzeCounterparties(uploadID int, databaseID int) error {
	log.Printf("Analyzing counterparties quality for upload %d", uploadID)

	// 1. Проверка полноты данных
	completenessMetric, err := qa.analyzeCounterpartyCompleteness(uploadID, databaseID)
	if err != nil {
		log.Printf("Error analyzing counterparty completeness: %v", err)
	} else {
		if err := qa.db.SaveQualityMetric(&completenessMetric); err != nil {
			log.Printf("Error saving completeness metric: %v", err)
		}
	}

	// 2. Проверка уникальности
	uniquenessMetric, err := qa.analyzeCounterpartyUniqueness(uploadID, databaseID)
	if err != nil {
		log.Printf("Error analyzing counterparty uniqueness: %v", err)
	} else {
		if err := qa.db.SaveQualityMetric(&uniquenessMetric); err != nil {
			log.Printf("Error saving uniqueness metric: %v", err)
		}
	}

	// 3. Валидация ИНН
	if err := qa.validateCounterpartyINN(uploadID, databaseID); err != nil {
		log.Printf("Error validating counterparty INN: %v", err)
	}

	// 4. Валидация КПП
	if err := qa.validateCounterpartyKPP(uploadID, databaseID); err != nil {
		log.Printf("Error validating counterparty KPP: %v", err)
	}

	return nil
}

// analyzeCounterpartyCompleteness анализирует полноту данных контрагентов
func (qa *QualityAnalyzer) analyzeCounterpartyCompleteness(uploadID int, databaseID int) (database.DataQualityMetric, error) {
	metric := database.DataQualityMetric{
		UploadID:       uploadID,
		DatabaseID:     databaseID,
		MetricCategory: "completeness",
		MetricName:     "counterparty_completeness",
		MeasuredAt:     time.Now(),
	}

	// Получаем элементы справочника контрагентов
	query := `
		SELECT ci.id, ci.attributes_xml
		FROM catalog_items ci
		INNER JOIN catalogs c ON ci.catalog_id = c.id
		WHERE c.upload_id = ? AND c.name = 'Контрагенты'
	`

	rows, err := qa.db.Query(query, uploadID)
	if err != nil {
		return metric, fmt.Errorf("failed to query counterparties: %w", err)
	}
	defer rows.Close()

	var total, withINN, withKPP, withAddress, withContacts int

	for rows.Next() {
		var id int
		var attributesXML sql.NullString
		if err := rows.Scan(&id, &attributesXML); err != nil {
			continue
		}

		total++

		if attributesXML.Valid && attributesXML.String != "" {
			// Проверяем наличие ИНН
			if inn, err := ExtractINNFromAttributes(attributesXML.String); err == nil && inn != "" {
				withINN++
			}

			// Проверяем наличие КПП
			if kpp, err := ExtractKPPFromAttributes(attributesXML.String); err == nil && kpp != "" {
				withKPP++
			}

			// Проверяем наличие адреса (простая проверка по ключевым словам)
			if containsAddress(attributesXML.String) {
				withAddress++
			}

			// Проверяем наличие контактов
			if containsContacts(attributesXML.String) {
				withContacts++
			}
		}
	}

	if total == 0 {
		metric.MetricValue = 0
		metric.Status = "FAIL"
		return metric, nil
	}

	innCompleteness := float64(withINN) / float64(total) * 100
	kppCompleteness := float64(withKPP) / float64(total) * 100
	addressCompleteness := float64(withAddress) / float64(total) * 100
	contactsCompleteness := float64(withContacts) / float64(total) * 100

	// Средний балл полноты
	metric.MetricValue = (innCompleteness + kppCompleteness + addressCompleteness + contactsCompleteness) / 4
	metric.Details = map[string]interface{}{
		"total_records":        total,
		"inn_completeness":     innCompleteness,
		"kpp_completeness":     kppCompleteness,
		"address_completeness": addressCompleteness,
		"contacts_completeness": contactsCompleteness,
	}

	// Определяем статус
	if metric.MetricValue >= 80 {
		metric.Status = "PASS"
	} else if metric.MetricValue >= 60 {
		metric.Status = "WARNING"
	} else {
		metric.Status = "FAIL"
	}

	// Выявление проблем
	if innCompleteness < 80 {
		issue := database.DataQualityIssue{
			UploadID:      uploadID,
			DatabaseID:    databaseID,
			EntityType:    "counterparty",
			IssueType:     "missing_inn",
			IssueSeverity: "HIGH",
			FieldName:     "ИНН",
			Description:   fmt.Sprintf("Не заполнен ИНН у %.1f%% контрагентов", 100-innCompleteness),
			DetectedAt:    time.Now(),
			Status:        "OPEN",
		}
		if err := qa.db.SaveQualityIssue(&issue); err != nil {
			log.Printf("Error saving missing INN issue: %v", err)
		}
	}

	return metric, nil
}

// analyzeCounterpartyUniqueness анализирует уникальность контрагентов
func (qa *QualityAnalyzer) analyzeCounterpartyUniqueness(uploadID int, databaseID int) (database.DataQualityMetric, error) {
	metric := database.DataQualityMetric{
		UploadID:       uploadID,
		DatabaseID:     databaseID,
		MetricCategory: "uniqueness",
		MetricName:     "counterparty_uniqueness",
		MeasuredAt:     time.Now(),
	}

	// Получаем все контрагенты с ИНН
	query := `
		SELECT ci.id, ci.attributes_xml, ci.name
		FROM catalog_items ci
		INNER JOIN catalogs c ON ci.catalog_id = c.id
		WHERE c.upload_id = ? AND c.name = 'Контрагенты'
	`

	rows, err := qa.db.Query(query, uploadID)
	if err != nil {
		return metric, fmt.Errorf("failed to query counterparties: %w", err)
	}
	defer rows.Close()

	innMap := make(map[string][]string) // ИНН -> список имен
	nameMap := make(map[string][]string) // имя -> список ИНН

	var total int
	for rows.Next() {
		var id int
		var attributesXML sql.NullString
		var name sql.NullString
		if err := rows.Scan(&id, &attributesXML, &name); err != nil {
			continue
		}

		total++

		if attributesXML.Valid && attributesXML.String != "" {
			inn, err := ExtractINNFromAttributes(attributesXML.String)
			if err == nil && inn != "" {
				entityName := name.String
				if entityName == "" {
					entityName = fmt.Sprintf("ID:%d", id)
				}
				innMap[inn] = append(innMap[inn], entityName)
			}
		}

		if name.Valid && name.String != "" {
			entityName := name.String
			nameMap[entityName] = append(nameMap[entityName], entityName)
		}
	}

	// Подсчитываем дубликаты
	var duplicateByINN, duplicateByName int
	duplicateINNs := []string{}
	duplicateNames := []string{}

	for inn, names := range innMap {
		if len(names) > 1 {
			duplicateByINN += len(names)
			duplicateINNs = append(duplicateINNs, inn)
		}
	}

	for name, entities := range nameMap {
		if len(entities) > 1 {
			duplicateByName += len(entities)
			duplicateNames = append(duplicateNames, name)
		}
	}

	if total == 0 {
		metric.MetricValue = 100
		metric.Status = "PASS"
		return metric, nil
	}

	uniquenessScore := float64(total-duplicateByINN) / float64(total) * 100
	if uniquenessScore > float64(total-duplicateByName)/float64(total)*100 {
		uniquenessScore = float64(total-duplicateByName) / float64(total) * 100
	}

	metric.MetricValue = uniquenessScore
	metric.Details = map[string]interface{}{
		"total_records":     total,
		"duplicate_by_inn":  duplicateByINN,
		"duplicate_by_name": duplicateByName,
		"duplicate_inns":    duplicateINNs[:min(10, len(duplicateINNs))], // Ограничиваем для JSON
		"duplicate_names":   duplicateNames[:min(10, len(duplicateNames))],
	}

	// Определяем статус
	if uniquenessScore >= 99 {
		metric.Status = "PASS"
	} else if uniquenessScore >= 95 {
		metric.Status = "WARNING"
	} else {
		metric.Status = "FAIL"
	}

	// Сохранение проблем дубликатов
	for _, inn := range duplicateINNs[:min(10, len(duplicateINNs))] {
		issue := database.DataQualityIssue{
			UploadID:      uploadID,
			DatabaseID:    databaseID,
			EntityType:    "counterparty",
			IssueType:     "duplicate_inn",
			IssueSeverity: "HIGH",
			FieldName:     "ИНН",
			ActualValue:   inn,
			Description:   fmt.Sprintf("Дубликат ИНН: %s", inn),
			DetectedAt:    time.Now(),
			Status:        "OPEN",
		}
		if err := qa.db.SaveQualityIssue(&issue); err != nil {
			log.Printf("Error saving duplicate INN issue: %v", err)
		}
	}

	return metric, nil
}

// validateCounterpartyINN валидирует ИНН контрагентов
func (qa *QualityAnalyzer) validateCounterpartyINN(uploadID int, databaseID int) error {
	query := `
		SELECT ci.id, ci.attributes_xml, ci.reference
		FROM catalog_items ci
		INNER JOIN catalogs c ON ci.catalog_id = c.id
		WHERE c.upload_id = ? AND c.name = 'Контрагенты'
	`

	rows, err := qa.db.Query(query, uploadID)
	if err != nil {
		return fmt.Errorf("failed to query counterparties: %w", err)
	}
	defer rows.Close()

	invalidCount := 0
	for rows.Next() {
		var id int
		var attributesXML sql.NullString
		var reference sql.NullString
		if err := rows.Scan(&id, &attributesXML, &reference); err != nil {
			continue
		}

		if attributesXML.Valid && attributesXML.String != "" {
			inn, err := ExtractINNFromAttributes(attributesXML.String)
			if err == nil && inn != "" {
				if !ValidateINN(inn) {
					invalidCount++
					issue := database.DataQualityIssue{
						UploadID:       uploadID,
						DatabaseID:     databaseID,
						EntityType:     "counterparty",
						EntityReference: reference.String,
						IssueType:      "invalid_inn",
						IssueSeverity:  "HIGH",
						FieldName:      "ИНН",
						ActualValue:    inn,
						Description:    fmt.Sprintf("Неверный формат ИНН: %s", inn),
						DetectedAt:     time.Now(),
						Status:         "OPEN",
					}
					if err := qa.db.SaveQualityIssue(&issue); err != nil {
						log.Printf("Error saving invalid INN issue: %v", err)
					}
				}
			}
		}
	}

	log.Printf("Validated INN for counterparties: %d invalid found", invalidCount)
	return nil
}

// validateCounterpartyKPP валидирует КПП контрагентов
func (qa *QualityAnalyzer) validateCounterpartyKPP(uploadID int, databaseID int) error {
	query := `
		SELECT ci.id, ci.attributes_xml, ci.reference
		FROM catalog_items ci
		INNER JOIN catalogs c ON ci.catalog_id = c.id
		WHERE c.upload_id = ? AND c.name = 'Контрагенты'
	`

	rows, err := qa.db.Query(query, uploadID)
	if err != nil {
		return fmt.Errorf("failed to query counterparties: %w", err)
	}
	defer rows.Close()

	invalidCount := 0
	for rows.Next() {
		var id int
		var attributesXML sql.NullString
		var reference sql.NullString
		if err := rows.Scan(&id, &attributesXML, &reference); err != nil {
			continue
		}

		if attributesXML.Valid && attributesXML.String != "" {
			kpp, err := ExtractKPPFromAttributes(attributesXML.String)
			if err == nil && kpp != "" {
				if !ValidateKPP(kpp) {
					invalidCount++
					issue := database.DataQualityIssue{
						UploadID:       uploadID,
						DatabaseID:     databaseID,
						EntityType:     "counterparty",
						EntityReference: reference.String,
						IssueType:      "invalid_kpp",
						IssueSeverity:  "MEDIUM",
						FieldName:      "КПП",
						ActualValue:    kpp,
						Description:    fmt.Sprintf("Неверный формат КПП: %s", kpp),
						DetectedAt:     time.Now(),
						Status:         "OPEN",
					}
					if err := qa.db.SaveQualityIssue(&issue); err != nil {
						log.Printf("Error saving invalid KPP issue: %v", err)
					}
				}
			}
		}
	}

	log.Printf("Validated KPP for counterparties: %d invalid found", invalidCount)
	return nil
}

// containsAddress проверяет наличие адреса в XML
func containsAddress(xml string) bool {
	keywords := []string{"адрес", "address", "Адрес", "улица", "street", "город", "city"}
	lowerXML := strings.ToLower(xml)
	for _, keyword := range keywords {
		if strings.Contains(lowerXML, strings.ToLower(keyword)) {
			return true
		}
	}
	return false
}

// containsContacts проверяет наличие контактов в XML
func containsContacts(xml string) bool {
	keywords := []string{"телефон", "phone", "email", "почта", "контакт", "contact"}
	lowerXML := strings.ToLower(xml)
	for _, keyword := range keywords {
		if strings.Contains(lowerXML, strings.ToLower(keyword)) {
			return true
		}
	}
	return false
}

// min возвращает минимальное из двух чисел
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

