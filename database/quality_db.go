package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// QualityAssessment оценка качества записи
type QualityAssessment struct {
	ID                  int       `json:"id"`
	NormalizedItemID    int       `json:"normalized_item_id"`
	AssessmentDate      time.Time `json:"assessment_date"`
	OverallScore        float64   `json:"overall_score"`
	CategoryConfidence  float64   `json:"category_confidence"`
	NameClarity         float64   `json:"name_clarity"`
	Consistency         float64   `json:"consistency"`
	Completeness        float64   `json:"completeness"`
	Standardization     float64   `json:"standardization"`
	KpvedAccuracy       float64   `json:"kpved_accuracy"`
	DuplicateScore      float64   `json:"duplicate_score"`
	DataEnrichment      float64   `json:"data_enrichment"`
	IsBenchmark         bool      `json:"is_benchmark"`
	Issues              []string  `json:"issues"` // Десериализуется из issues_json
}

// QualityViolation нарушение правила качества
type QualityViolation struct {
	ID               int       `json:"id"`
	NormalizedItemID int       `json:"normalized_item_id"`
	RuleName         string    `json:"rule_name"`
	Category         string    `json:"category"`
	Severity         string    `json:"severity"`
	Description      string    `json:"description"`
	Field            string    `json:"field"`
	CurrentValue     string    `json:"current_value"`
	Recommendation   string    `json:"recommendation"`
	DetectedAt       time.Time `json:"detected_at"`
	ResolvedAt       *time.Time `json:"resolved_at,omitempty"`
	ResolvedBy       string    `json:"resolved_by,omitempty"`
}

// QualitySuggestion предложение по улучшению
type QualitySuggestion struct {
	ID              int        `json:"id"`
	NormalizedItemID int       `json:"normalized_item_id"`
	SuggestionType  string     `json:"suggestion_type"`
	Priority        string     `json:"priority"`
	Field           string     `json:"field"`
	CurrentValue    string     `json:"current_value"`
	SuggestedValue  string     `json:"suggested_value"`
	Confidence      float64    `json:"confidence"`
	Reasoning       string     `json:"reasoning"`
	AutoApplyable   bool       `json:"auto_applyable"`
	Applied         bool       `json:"applied"`
	AppliedAt       *time.Time `json:"applied_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
}

// DuplicateGroup группа дубликатов
type DuplicateGroup struct {
	ID                int       `json:"id"`
	GroupHash         string    `json:"group_hash"`
	DuplicateType     string    `json:"duplicate_type"`
	SimilarityScore   float64   `json:"similarity_score"`
	ItemIDs           []int     `json:"item_ids"` // Десериализуется из item_ids_json
	SuggestedMasterID int       `json:"suggested_master_id"`
	Confidence        float64   `json:"confidence"`
	Reason            string    `json:"reason"`
	Merged            bool      `json:"merged"`
	MergedAt          *time.Time `json:"merged_at,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

// --- Quality Assessments ---

// SaveQualityAssessment сохраняет оценку качества
func (db *DB) SaveQualityAssessment(assessment *QualityAssessment) error {
	issuesJSON, err := json.Marshal(assessment.Issues)
	if err != nil {
		return fmt.Errorf("failed to marshal issues: %w", err)
	}

	query := `
		INSERT INTO quality_assessments (
			normalized_item_id, assessment_date, overall_score,
			category_confidence, name_clarity, consistency,
			completeness, standardization, kpved_accuracy,
			duplicate_score, data_enrichment, is_benchmark, issues_json
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	result, err := db.conn.Exec(query,
		assessment.NormalizedItemID,
		assessment.AssessmentDate,
		assessment.OverallScore,
		assessment.CategoryConfidence,
		assessment.NameClarity,
		assessment.Consistency,
		assessment.Completeness,
		assessment.Standardization,
		assessment.KpvedAccuracy,
		assessment.DuplicateScore,
		assessment.DataEnrichment,
		assessment.IsBenchmark,
		string(issuesJSON),
	)

	if err != nil {
		return fmt.Errorf("failed to save quality assessment: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get last insert ID: %w", err)
	}

	assessment.ID = int(id)
	return nil
}

// GetQualityAssessment получает оценку качества по ID записи
func (db *DB) GetQualityAssessment(normalizedItemID int) (*QualityAssessment, error) {
	query := `
		SELECT id, normalized_item_id, assessment_date, overall_score,
			category_confidence, name_clarity, consistency,
			completeness, standardization, kpved_accuracy,
			duplicate_score, data_enrichment, is_benchmark, issues_json
		FROM quality_assessments
		WHERE normalized_item_id = ?
		ORDER BY assessment_date DESC
		LIMIT 1
	`

	var assessment QualityAssessment
	var issuesJSON string

	err := db.conn.QueryRow(query, normalizedItemID).Scan(
		&assessment.ID,
		&assessment.NormalizedItemID,
		&assessment.AssessmentDate,
		&assessment.OverallScore,
		&assessment.CategoryConfidence,
		&assessment.NameClarity,
		&assessment.Consistency,
		&assessment.Completeness,
		&assessment.Standardization,
		&assessment.KpvedAccuracy,
		&assessment.DuplicateScore,
		&assessment.DataEnrichment,
		&assessment.IsBenchmark,
		&issuesJSON,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get quality assessment: %w", err)
	}

	// Десериализуем issues
	if err := json.Unmarshal([]byte(issuesJSON), &assessment.Issues); err != nil {
		assessment.Issues = []string{} // Fallback на пустой массив
	}

	return &assessment, nil
}

// GetQualityAssessmentHistory получает историю оценок качества
func (db *DB) GetQualityAssessmentHistory(normalizedItemID int, limit int) ([]QualityAssessment, error) {
	query := `
		SELECT id, normalized_item_id, assessment_date, overall_score,
			category_confidence, name_clarity, consistency,
			completeness, standardization, kpved_accuracy,
			duplicate_score, data_enrichment, is_benchmark, issues_json
		FROM quality_assessments
		WHERE normalized_item_id = ?
		ORDER BY assessment_date DESC
		LIMIT ?
	`

	rows, err := db.conn.Query(query, normalizedItemID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query quality assessments: %w", err)
	}
	defer rows.Close()

	var assessments []QualityAssessment
	for rows.Next() {
		var assessment QualityAssessment
		var issuesJSON string

		err := rows.Scan(
			&assessment.ID,
			&assessment.NormalizedItemID,
			&assessment.AssessmentDate,
			&assessment.OverallScore,
			&assessment.CategoryConfidence,
			&assessment.NameClarity,
			&assessment.Consistency,
			&assessment.Completeness,
			&assessment.Standardization,
			&assessment.KpvedAccuracy,
			&assessment.DuplicateScore,
			&assessment.DataEnrichment,
			&assessment.IsBenchmark,
			&issuesJSON,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan assessment: %w", err)
		}

		// Десериализуем issues
		if err := json.Unmarshal([]byte(issuesJSON), &assessment.Issues); err != nil {
			assessment.Issues = []string{}
		}

		assessments = append(assessments, assessment)
	}

	return assessments, nil
}

// --- Quality Violations ---

// SaveQualityViolation сохраняет нарушение правила
func (db *DB) SaveQualityViolation(violation *QualityViolation) error {
	query := `
		INSERT INTO quality_violations (
			normalized_item_id, rule_name, category, severity,
			description, field, current_value, recommendation, detected_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	result, err := db.conn.Exec(query,
		violation.NormalizedItemID,
		violation.RuleName,
		violation.Category,
		violation.Severity,
		violation.Description,
		violation.Field,
		violation.CurrentValue,
		violation.Recommendation,
		violation.DetectedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to save violation: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get last insert ID: %w", err)
	}

	violation.ID = int(id)
	return nil
}

// GetViolations получает все нарушения
func (db *DB) GetViolations(filters map[string]interface{}, limit, offset int) ([]QualityViolation, int, error) {
	whereClause, args := buildWhereClause(filters)

	// Получаем общее количество
	countQuery := "SELECT COUNT(*) FROM quality_violations" + whereClause
	var total int
	err := db.conn.QueryRow(countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count violations: %w", err)
	}

	// Получаем данные
	query := `
		SELECT id, normalized_item_id, rule_name, category, severity,
			description, field, current_value, recommendation,
			detected_at, resolved_at, resolved_by
		FROM quality_violations
	` + whereClause + ` ORDER BY detected_at DESC LIMIT ? OFFSET ?`

	args = append(args, limit, offset)
	rows, err := db.conn.Query(query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query violations: %w", err)
	}
	defer rows.Close()

	var violations []QualityViolation
	for rows.Next() {
		var v QualityViolation
		err := rows.Scan(
			&v.ID,
			&v.NormalizedItemID,
			&v.RuleName,
			&v.Category,
			&v.Severity,
			&v.Description,
			&v.Field,
			&v.CurrentValue,
			&v.Recommendation,
			&v.DetectedAt,
			&v.ResolvedAt,
			&v.ResolvedBy,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan violation: %w", err)
		}
		violations = append(violations, v)
	}

	return violations, total, nil
}

// ResolveViolation помечает нарушение как решенное
func (db *DB) ResolveViolation(id int, resolvedBy string) error {
	query := `
		UPDATE quality_violations
		SET resolved_at = ?, resolved_by = ?
		WHERE id = ?
	`

	_, err := db.conn.Exec(query, time.Now(), resolvedBy, id)
	if err != nil {
		return fmt.Errorf("failed to resolve violation: %w", err)
	}

	return nil
}

// --- Quality Suggestions ---

// SaveQualitySuggestion сохраняет предложение по улучшению
func (db *DB) SaveQualitySuggestion(suggestion *QualitySuggestion) error {
	query := `
		INSERT INTO quality_suggestions (
			normalized_item_id, suggestion_type, priority, field,
			current_value, suggested_value, confidence, reasoning,
			auto_applyable, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	result, err := db.conn.Exec(query,
		suggestion.NormalizedItemID,
		suggestion.SuggestionType,
		suggestion.Priority,
		suggestion.Field,
		suggestion.CurrentValue,
		suggestion.SuggestedValue,
		suggestion.Confidence,
		suggestion.Reasoning,
		suggestion.AutoApplyable,
		suggestion.CreatedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to save suggestion: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get last insert ID: %w", err)
	}

	suggestion.ID = int(id)
	return nil
}

// GetSuggestions получает предложения с фильтрацией
func (db *DB) GetSuggestions(filters map[string]interface{}, limit, offset int) ([]QualitySuggestion, int, error) {
	whereClause, args := buildWhereClause(filters)

	// Получаем общее количество
	countQuery := "SELECT COUNT(*) FROM quality_suggestions" + whereClause
	var total int
	err := db.conn.QueryRow(countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count suggestions: %w", err)
	}

	// Получаем данные
	query := `
		SELECT id, normalized_item_id, suggestion_type, priority, field,
			current_value, suggested_value, confidence, reasoning,
			auto_applyable, applied, applied_at, created_at
		FROM quality_suggestions
	` + whereClause + ` ORDER BY priority DESC, confidence DESC, created_at DESC LIMIT ? OFFSET ?`

	args = append(args, limit, offset)
	rows, err := db.conn.Query(query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query suggestions: %w", err)
	}
	defer rows.Close()

	var suggestions []QualitySuggestion
	for rows.Next() {
		var s QualitySuggestion
		err := rows.Scan(
			&s.ID,
			&s.NormalizedItemID,
			&s.SuggestionType,
			&s.Priority,
			&s.Field,
			&s.CurrentValue,
			&s.SuggestedValue,
			&s.Confidence,
			&s.Reasoning,
			&s.AutoApplyable,
			&s.Applied,
			&s.AppliedAt,
			&s.CreatedAt,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan suggestion: %w", err)
		}
		suggestions = append(suggestions, s)
	}

	return suggestions, total, nil
}

// ApplySuggestion помечает предложение как примененное
func (db *DB) ApplySuggestion(id int) error {
	query := `
		UPDATE quality_suggestions
		SET applied = TRUE, applied_at = ?
		WHERE id = ?
	`

	_, err := db.conn.Exec(query, time.Now(), id)
	if err != nil {
		return fmt.Errorf("failed to apply suggestion: %w", err)
	}

	return nil
}

// --- Duplicate Groups ---

// SaveDuplicateGroup сохраняет группу дубликатов
func (db *DB) SaveDuplicateGroup(group *DuplicateGroup) error {
	itemIDsJSON, err := json.Marshal(group.ItemIDs)
	if err != nil {
		return fmt.Errorf("failed to marshal item IDs: %w", err)
	}

	query := `
		INSERT INTO duplicate_groups (
			group_hash, duplicate_type, similarity_score, item_ids_json,
			suggested_master_id, confidence, reason, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	result, err := db.conn.Exec(query,
		group.GroupHash,
		group.DuplicateType,
		group.SimilarityScore,
		string(itemIDsJSON),
		group.SuggestedMasterID,
		group.Confidence,
		group.Reason,
		group.CreatedAt,
		group.UpdatedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to save duplicate group: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get last insert ID: %w", err)
	}

	group.ID = int(id)
	return nil
}

// GetDuplicateGroups получает группы дубликатов
func (db *DB) GetDuplicateGroups(onlyUnmerged bool, limit, offset int) ([]DuplicateGroup, int, error) {
	whereClause := ""
	var args []interface{}

	if onlyUnmerged {
		whereClause = " WHERE merged = FALSE"
	}

	// Получаем общее количество
	countQuery := "SELECT COUNT(*) FROM duplicate_groups" + whereClause
	var total int
	err := db.conn.QueryRow(countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count duplicate groups: %w", err)
	}

	// Получаем данные
	query := `
		SELECT id, group_hash, duplicate_type, similarity_score, item_ids_json,
			suggested_master_id, confidence, reason, merged, merged_at,
			created_at, updated_at
		FROM duplicate_groups
	` + whereClause + ` ORDER BY similarity_score DESC, created_at DESC LIMIT ? OFFSET ?`

	args = append(args, limit, offset)
	rows, err := db.conn.Query(query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query duplicate groups: %w", err)
	}
	defer rows.Close()

	var groups []DuplicateGroup
	for rows.Next() {
		var g DuplicateGroup
		var itemIDsJSON string

		err := rows.Scan(
			&g.ID,
			&g.GroupHash,
			&g.DuplicateType,
			&g.SimilarityScore,
			&itemIDsJSON,
			&g.SuggestedMasterID,
			&g.Confidence,
			&g.Reason,
			&g.Merged,
			&g.MergedAt,
			&g.CreatedAt,
			&g.UpdatedAt,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan duplicate group: %w", err)
		}

		// Десериализуем item IDs
		if err := json.Unmarshal([]byte(itemIDsJSON), &g.ItemIDs); err != nil {
			g.ItemIDs = []int{}
		}

		groups = append(groups, g)
	}

	return groups, total, nil
}

// MarkDuplicateGroupMerged помечает группу дубликатов как объединенную
func (db *DB) MarkDuplicateGroupMerged(id int) error {
	query := `
		UPDATE duplicate_groups
		SET merged = TRUE, merged_at = ?, updated_at = ?
		WHERE id = ?
	`

	now := time.Now()
	_, err := db.conn.Exec(query, now, now, id)
	if err != nil {
		return fmt.Errorf("failed to mark duplicate group as merged: %w", err)
	}

	return nil
}

// --- Helper functions ---

// buildWhereClause строит WHERE clause из фильтров
func buildWhereClause(filters map[string]interface{}) (string, []interface{}) {
	if len(filters) == 0 {
		return "", []interface{}{}
	}

	var conditions []string
	var args []interface{}

	for key, value := range filters {
		conditions = append(conditions, fmt.Sprintf("%s = ?", key))
		args = append(args, value)
	}

	whereClause := " WHERE " + joinStrings(conditions, " AND ")
	return whereClause, args
}

// joinStrings объединяет строки с разделителем
func joinStrings(strs []string, sep string) string {
	if len(strs) == 0 {
		return ""
	}
	result := strs[0]
	for i := 1; i < len(strs); i++ {
		result += sep + strs[i]
	}
	return result
}
