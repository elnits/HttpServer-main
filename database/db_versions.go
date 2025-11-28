package database

import (
	"fmt"
)

// CreateNormalizationSession создает новую сессию нормализации
func (db *DB) CreateNormalizationSession(catalogItemID int, originalName string) (int, error) {
	query := `
		INSERT INTO normalization_sessions (catalog_item_id, original_name, current_name, status)
		VALUES (?, ?, ?, 'in_progress')
	`
	result, err := db.conn.Exec(query, catalogItemID, originalName, originalName)
	if err != nil {
		return 0, fmt.Errorf("failed to create normalization session: %w", err)
	}

	sessionID, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("failed to get session ID: %w", err)
	}

	return int(sessionID), nil
}

// AddNormalizationStage добавляет стадию нормализации
func (db *DB) AddNormalizationStage(stage *NormalizationStage) error {
	query := `
		INSERT INTO normalization_stages 
		(session_id, stage_type, stage_name, input_name, output_name, applied_patterns, 
		 ai_context, category_original, category_folded, classification_strategy, confidence, status)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := db.conn.Exec(query,
		stage.SessionID,
		stage.StageType,
		stage.StageName,
		stage.InputName,
		stage.OutputName,
		stage.AppliedPatterns,
		stage.AIContext,
		stage.CategoryOriginal,
		stage.CategoryFolded,
		stage.ClassificationStrategy,
		stage.Confidence,
		stage.Status,
	)
	if err != nil {
		return fmt.Errorf("failed to add normalization stage: %w", err)
	}

	// Обновляем счетчик стадий и текущее имя в сессии
	updateQuery := `
		UPDATE normalization_sessions 
		SET stages_count = stages_count + 1, 
		    current_name = ?,
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`
	_, err = db.conn.Exec(updateQuery, stage.OutputName, stage.SessionID)
	if err != nil {
		return fmt.Errorf("failed to update session: %w", err)
	}

	return nil
}

// GetSessionHistory получает полную историю стадий для сессии
func (db *DB) GetSessionHistory(sessionID int) ([]*NormalizationStage, error) {
	query := `
		SELECT id, session_id, stage_type, stage_name, input_name, output_name,
		       applied_patterns, ai_context, category_original, category_folded,
		       classification_strategy, confidence, status, created_at
		FROM normalization_stages
		WHERE session_id = ?
		ORDER BY created_at ASC
	`
	rows, err := db.conn.Query(query, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get session history: %w", err)
	}
	defer rows.Close()

	var stages []*NormalizationStage
	for rows.Next() {
		stage := &NormalizationStage{}
		err := rows.Scan(
			&stage.ID,
			&stage.SessionID,
			&stage.StageType,
			&stage.StageName,
			&stage.InputName,
			&stage.OutputName,
			&stage.AppliedPatterns,
			&stage.AIContext,
			&stage.CategoryOriginal,
			&stage.CategoryFolded,
			&stage.ClassificationStrategy,
			&stage.Confidence,
			&stage.Status,
			&stage.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan stage: %w", err)
		}
		stages = append(stages, stage)
	}

	return stages, nil
}

// GetNormalizationSession получает сессию по ID
func (db *DB) GetNormalizationSession(sessionID int) (*NormalizationSession, error) {
	query := `
		SELECT id, catalog_item_id, original_name, current_name, stages_count, status, created_at, updated_at
		FROM normalization_sessions
		WHERE id = ?
	`
	session := &NormalizationSession{}
	err := db.conn.QueryRow(query, sessionID).Scan(
		&session.ID,
		&session.CatalogItemID,
		&session.OriginalName,
		&session.CurrentName,
		&session.StagesCount,
		&session.Status,
		&session.CreatedAt,
		&session.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	return session, nil
}

// RevertToStage откатывает сессию к указанной стадии
func (db *DB) RevertToStage(sessionID int, targetStageID int) error {
	// Получаем целевую стадию
	var targetOutputName string
	err := db.conn.QueryRow(`
		SELECT output_name FROM normalization_stages WHERE id = ? AND session_id = ?
	`, targetStageID, sessionID).Scan(&targetOutputName)
	if err != nil {
		return fmt.Errorf("failed to get target stage: %w", err)
	}

	// Удаляем все стадии после целевой
	_, err = db.conn.Exec(`
		DELETE FROM normalization_stages 
		WHERE session_id = ? AND id > ?
	`, sessionID, targetStageID)
	if err != nil {
		return fmt.Errorf("failed to delete stages: %w", err)
	}

	// Обновляем сессию
	_, err = db.conn.Exec(`
		UPDATE normalization_sessions 
		SET current_name = ?,
		    stages_count = (SELECT COUNT(*) FROM normalization_stages WHERE session_id = ?),
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, targetOutputName, sessionID, sessionID)
	if err != nil {
		return fmt.Errorf("failed to update session: %w", err)
	}

	return nil
}

// UpdateSessionStatus обновляет статус сессии
func (db *DB) UpdateSessionStatus(sessionID int, status string) error {
	_, err := db.conn.Exec(`
		UPDATE normalization_sessions 
		SET status = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, status, sessionID)
	if err != nil {
		return fmt.Errorf("failed to update session status: %w", err)
	}
	return nil
}

