package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// DBConfig конфигурация подключения к БД (используется и для ServiceDB)
type DBConfig struct {
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

// ServiceDB обертка для работы с сервисной базой данных
type ServiceDB struct {
	conn *sql.DB
}

// NewServiceDB создает новое подключение к сервисной базе данных
func NewServiceDB(dbPath string) (*ServiceDB, error) {
	return NewServiceDBWithConfig(dbPath, DBConfig{})
}

// NewServiceDBWithConfig создает новое подключение к сервисной базе данных с конфигурацией
func NewServiceDBWithConfig(dbPath string, config DBConfig) (*ServiceDB, error) {
	conn, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open service database: %w", err)
	}

	// Настройка connection pooling
	if config.MaxOpenConns > 0 {
		conn.SetMaxOpenConns(config.MaxOpenConns)
	} else {
		conn.SetMaxOpenConns(25)
	}

	if config.MaxIdleConns > 0 {
		conn.SetMaxIdleConns(config.MaxIdleConns)
	} else {
		conn.SetMaxIdleConns(5)
	}

	if config.ConnMaxLifetime > 0 {
		conn.SetConnMaxLifetime(config.ConnMaxLifetime)
	} else {
		conn.SetConnMaxLifetime(5 * time.Minute)
	}

	// Проверяем подключение
	if err := conn.Ping(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to ping service database: %w", err)
	}

	serviceDB := &ServiceDB{conn: conn}

	// Инициализируем схему сервисной БД
	if err := InitServiceSchema(conn); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to initialize service schema: %w", err)
	}

	return serviceDB, nil
}

// Close закрывает подключение к сервисной базе данных
func (db *ServiceDB) Close() error {
	return db.conn.Close()
}

// GetDB возвращает указатель на sql.DB для прямого доступа
func (db *ServiceDB) GetDB() *sql.DB {
	return db.conn
}

// QueryRow выполняет запрос и возвращает одну строку
func (db *ServiceDB) QueryRow(query string, args ...interface{}) *sql.Row {
	return db.conn.QueryRow(query, args...)
}

// Query выполняет запрос и возвращает несколько строк
func (db *ServiceDB) Query(query string, args ...interface{}) (*sql.Rows, error) {
	return db.conn.Query(query, args...)
}

// Exec выполняет запрос без возврата строк
func (db *ServiceDB) Exec(query string, args ...interface{}) (sql.Result, error) {
	return db.conn.Exec(query, args...)
}

// Client структура клиента
type Client struct {
	ID           int       `json:"id"`
	Name         string    `json:"name"`
	LegalName    string    `json:"legal_name"`
	Description  string    `json:"description"`
	ContactEmail string    `json:"contact_email"`
	ContactPhone string    `json:"contact_phone"`
	TaxID        string    `json:"tax_id"`
	Status       string    `json:"status"`
	CreatedBy    string    `json:"created_by"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// ClientProject структура проекта клиента
type ClientProject struct {
	ID                 int       `json:"id"`
	ClientID           int       `json:"client_id"`
	Name               string    `json:"name"`
	ProjectType        string    `json:"project_type"`
	Description        string    `json:"description"`
	SourceSystem       string    `json:"source_system"`
	Status             string    `json:"status"`
	TargetQualityScore float64   `json:"target_quality_score"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// ClientBenchmark структура эталонной записи
type ClientBenchmark struct {
	ID              int        `json:"id"`
	ClientProjectID int        `json:"client_project_id"`
	OriginalName    string     `json:"original_name"`
	NormalizedName  string     `json:"normalized_name"`
	Category        string     `json:"category"`
	Subcategory     string     `json:"subcategory"`
	Attributes      string     `json:"attributes"`
	QualityScore    float64    `json:"quality_score"`
	IsApproved      bool       `json:"is_approved"`
	ApprovedBy      string     `json:"approved_by"`
	ApprovedAt      *time.Time `json:"approved_at"`
	SourceDatabase  string     `json:"source_database"`
	UsageCount      int        `json:"usage_count"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// NormalizationConfig структура конфигурации нормализации
type NormalizationConfig struct {
	ID              int       `json:"id"`
	DatabasePath    string    `json:"database_path"`
	SourceTable     string    `json:"source_table"`
	ReferenceColumn string    `json:"reference_column"`
	CodeColumn      string    `json:"code_column"`
	NameColumn      string    `json:"name_column"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// ProjectDatabase структура базы данных проекта
type ProjectDatabase struct {
	ID              int        `json:"id"`
	ClientProjectID int        `json:"client_project_id"`
	Name            string     `json:"name"`
	FilePath        string     `json:"file_path"`
	Description     string     `json:"description"`
	IsActive        bool       `json:"is_active"`
	FileSize        int64      `json:"file_size"`
	LastUsedAt      *time.Time `json:"last_used_at"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// CreateClient создает нового клиента
func (db *ServiceDB) CreateClient(name, legalName, description, contactEmail, contactPhone, taxID, createdBy string) (*Client, error) {
	query := `
		INSERT INTO clients (name, legal_name, description, contact_email, contact_phone, tax_id, created_by)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`

	result, err := db.conn.Exec(query, name, legalName, description, contactEmail, contactPhone, taxID, createdBy)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("failed to get client ID: %w", err)
	}

	return db.GetClient(int(id))
}

// GetClient получает клиента по ID
func (db *ServiceDB) GetClient(id int) (*Client, error) {
	query := `
		SELECT id, name, legal_name, description, contact_email, contact_phone, tax_id, 
		       status, created_by, created_at, updated_at
		FROM clients WHERE id = ?
	`

	row := db.conn.QueryRow(query, id)
	client := &Client{}

	var approvedAt sql.NullTime
	err := row.Scan(
		&client.ID, &client.Name, &client.LegalName, &client.Description,
		&client.ContactEmail, &client.ContactPhone, &client.TaxID,
		&client.Status, &client.CreatedBy, &client.CreatedAt, &client.UpdatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get client: %w", err)
	}

	_ = approvedAt // не используется в Client

	return client, nil
}

// UpdateClient обновляет информацию о клиенте
func (db *ServiceDB) UpdateClient(id int, name, legalName, description, contactEmail, contactPhone, taxID, status string) error {
	query := `
		UPDATE clients 
		SET name = ?, legal_name = ?, description = ?, contact_email = ?, 
		    contact_phone = ?, tax_id = ?, status = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`

	_, err := db.conn.Exec(query, name, legalName, description, contactEmail, contactPhone, taxID, status, id)
	if err != nil {
		return fmt.Errorf("failed to update client: %w", err)
	}

	return nil
}

// DeleteClient удаляет клиента
func (db *ServiceDB) DeleteClient(id int) error {
	query := `DELETE FROM clients WHERE id = ?`

	_, err := db.conn.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to delete client: %w", err)
	}

	return nil
}

// GetClientsWithStats получает список клиентов со статистикой
func (db *ServiceDB) GetClientsWithStats() ([]map[string]interface{}, error) {
	query := `
		SELECT 
			c.id,
			c.name,
			c.legal_name,
			c.description,
			c.status,
			COUNT(DISTINCT cp.id) as project_count,
			COUNT(DISTINCT cb.id) as benchmark_count,
			MAX(COALESCE(cp.updated_at, c.created_at)) as last_activity
		FROM clients c
		LEFT JOIN client_projects cp ON c.id = cp.client_id
		LEFT JOIN client_benchmarks cb ON cp.id = cb.client_project_id
		GROUP BY c.id
		ORDER BY c.created_at DESC
	`

	rows, err := db.conn.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to get clients: %w", err)
	}
	defer rows.Close()

	var clients []map[string]interface{}
	for rows.Next() {
		var id, projectCount, benchmarkCount int
		var name, legalName, description, status string
		var lastActivity string

		err := rows.Scan(&id, &name, &legalName, &description, &status, &projectCount, &benchmarkCount, &lastActivity)
		if err != nil {
			return nil, fmt.Errorf("failed to scan client: %w", err)
		}

		client := map[string]interface{}{
			"id":              id,
			"name":            name,
			"legal_name":      legalName,
			"description":     description,
			"status":          status,
			"project_count":   projectCount,
			"benchmark_count": benchmarkCount,
			"last_activity":   lastActivity,
		}

		clients = append(clients, client)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating clients: %w", err)
	}

	return clients, nil
}

// CreateClientProject создает новый проект клиента
func (db *ServiceDB) CreateClientProject(clientID int, name, projectType, description, sourceSystem string, targetQualityScore float64) (*ClientProject, error) {
	query := `
		INSERT INTO client_projects (client_id, name, project_type, description, source_system, target_quality_score)
		VALUES (?, ?, ?, ?, ?, ?)
	`

	result, err := db.conn.Exec(query, clientID, name, projectType, description, sourceSystem, targetQualityScore)
	if err != nil {
		return nil, fmt.Errorf("failed to create project: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("failed to get project ID: %w", err)
	}

	return db.GetClientProject(int(id))
}

// GetClientProject получает проект по ID
func (db *ServiceDB) GetClientProject(id int) (*ClientProject, error) {
	query := `
		SELECT id, client_id, name, project_type, description, source_system, 
		       status, target_quality_score, created_at, updated_at
		FROM client_projects WHERE id = ?
	`

	row := db.conn.QueryRow(query, id)
	project := &ClientProject{}

	err := row.Scan(
		&project.ID, &project.ClientID, &project.Name, &project.ProjectType,
		&project.Description, &project.SourceSystem, &project.Status,
		&project.TargetQualityScore, &project.CreatedAt, &project.UpdatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get project: %w", err)
	}

	return project, nil
}

// GetClientProjects получает все проекты клиента
func (db *ServiceDB) GetClientProjects(clientID int) ([]*ClientProject, error) {
	query := `
		SELECT id, client_id, name, project_type, description, source_system, 
		       status, target_quality_score, created_at, updated_at
		FROM client_projects 
		WHERE client_id = ?
		ORDER BY created_at DESC
	`

	rows, err := db.conn.Query(query, clientID)
	if err != nil {
		return nil, fmt.Errorf("failed to get projects: %w", err)
	}
	defer rows.Close()

	var projects []*ClientProject
	for rows.Next() {
		project := &ClientProject{}
		err := rows.Scan(
			&project.ID, &project.ClientID, &project.Name, &project.ProjectType,
			&project.Description, &project.SourceSystem, &project.Status,
			&project.TargetQualityScore, &project.CreatedAt, &project.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan project: %w", err)
		}
		projects = append(projects, project)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating projects: %w", err)
	}

	return projects, nil
}

// UpdateClientProject обновляет проект
func (db *ServiceDB) UpdateClientProject(id int, name, projectType, description, sourceSystem, status string, targetQualityScore float64) error {
	query := `
		UPDATE client_projects 
		SET name = ?, project_type = ?, description = ?, source_system = ?, 
		    status = ?, target_quality_score = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`

	_, err := db.conn.Exec(query, name, projectType, description, sourceSystem, status, targetQualityScore, id)
	if err != nil {
		return fmt.Errorf("failed to update project: %w", err)
	}

	return nil
}

// DeleteClientProject удаляет проект
func (db *ServiceDB) DeleteClientProject(id int) error {
	query := `DELETE FROM client_projects WHERE id = ?`

	_, err := db.conn.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to delete project: %w", err)
	}

	return nil
}

// CreateClientBenchmark создает эталонную запись
func (db *ServiceDB) CreateClientBenchmark(projectID int, originalName, normalizedName, category, subcategory, attributes, sourceDatabase string, qualityScore float64) (*ClientBenchmark, error) {
	query := `
		INSERT INTO client_benchmarks 
		(client_project_id, original_name, normalized_name, category, subcategory, attributes, quality_score, source_database)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`

	result, err := db.conn.Exec(query, projectID, originalName, normalizedName, category, subcategory, attributes, qualityScore, sourceDatabase)
	if err != nil {
		return nil, fmt.Errorf("failed to create benchmark: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("failed to get benchmark ID: %w", err)
	}

	return db.GetClientBenchmark(int(id))
}

// GetClientBenchmark получает эталон по ID
func (db *ServiceDB) GetClientBenchmark(id int) (*ClientBenchmark, error) {
	query := `
		SELECT id, client_project_id, original_name, normalized_name, category, subcategory,
		       attributes, quality_score, is_approved, approved_by, approved_at,
		       source_database, usage_count, created_at, updated_at
		FROM client_benchmarks WHERE id = ?
	`

	row := db.conn.QueryRow(query, id)
	benchmark := &ClientBenchmark{}

	var approvedAt sql.NullTime
	err := row.Scan(
		&benchmark.ID, &benchmark.ClientProjectID, &benchmark.OriginalName, &benchmark.NormalizedName,
		&benchmark.Category, &benchmark.Subcategory, &benchmark.Attributes, &benchmark.QualityScore,
		&benchmark.IsApproved, &benchmark.ApprovedBy, &approvedAt,
		&benchmark.SourceDatabase, &benchmark.UsageCount, &benchmark.CreatedAt, &benchmark.UpdatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get benchmark: %w", err)
	}

	if approvedAt.Valid {
		benchmark.ApprovedAt = &approvedAt.Time
	}

	return benchmark, nil
}

// FindClientBenchmark ищет эталон по названию для проекта
func (db *ServiceDB) FindClientBenchmark(projectID int, name string) (*ClientBenchmark, error) {
	query := `
		SELECT id, client_project_id, original_name, normalized_name, category, subcategory,
		       attributes, quality_score, is_approved, approved_by, approved_at,
		       source_database, usage_count, created_at, updated_at
		FROM client_benchmarks 
		WHERE client_project_id = ? 
		  AND (original_name = ? OR normalized_name = ?)
		  AND is_approved = TRUE
		ORDER BY quality_score DESC, usage_count DESC
		LIMIT 1
	`

	row := db.conn.QueryRow(query, projectID, name, name)
	benchmark := &ClientBenchmark{}

	var approvedAt sql.NullTime
	err := row.Scan(
		&benchmark.ID, &benchmark.ClientProjectID, &benchmark.OriginalName, &benchmark.NormalizedName,
		&benchmark.Category, &benchmark.Subcategory, &benchmark.Attributes, &benchmark.QualityScore,
		&benchmark.IsApproved, &benchmark.ApprovedBy, &approvedAt,
		&benchmark.SourceDatabase, &benchmark.UsageCount, &benchmark.CreatedAt, &benchmark.UpdatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to find benchmark: %w", err)
	}

	if approvedAt.Valid {
		benchmark.ApprovedAt = &approvedAt.Time
	}

	return benchmark, nil
}

// GetClientBenchmarks получает эталоны проекта
func (db *ServiceDB) GetClientBenchmarks(projectID int, category string, approvedOnly bool) ([]*ClientBenchmark, error) {
	query := `
		SELECT id, client_project_id, original_name, normalized_name, category, subcategory,
		       attributes, quality_score, is_approved, approved_by, approved_at,
		       source_database, usage_count, created_at, updated_at
		FROM client_benchmarks 
		WHERE client_project_id = ?
	`

	args := []interface{}{projectID}

	if category != "" {
		query += " AND category = ?"
		args = append(args, category)
	}

	if approvedOnly {
		query += " AND is_approved = TRUE"
	}

	query += " ORDER BY created_at DESC"

	rows, err := db.conn.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get benchmarks: %w", err)
	}
	defer rows.Close()

	var benchmarks []*ClientBenchmark
	for rows.Next() {
		benchmark := &ClientBenchmark{}
		var approvedAt sql.NullTime

		err := rows.Scan(
			&benchmark.ID, &benchmark.ClientProjectID, &benchmark.OriginalName, &benchmark.NormalizedName,
			&benchmark.Category, &benchmark.Subcategory, &benchmark.Attributes, &benchmark.QualityScore,
			&benchmark.IsApproved, &benchmark.ApprovedBy, &approvedAt,
			&benchmark.SourceDatabase, &benchmark.UsageCount, &benchmark.CreatedAt, &benchmark.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan benchmark: %w", err)
		}

		if approvedAt.Valid {
			benchmark.ApprovedAt = &approvedAt.Time
		}

		benchmarks = append(benchmarks, benchmark)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating benchmarks: %w", err)
	}

	return benchmarks, nil
}

// UpdateBenchmarkUsage увеличивает счетчик использования эталона
func (db *ServiceDB) UpdateBenchmarkUsage(benchmarkID int) error {
	query := `UPDATE client_benchmarks SET usage_count = usage_count + 1 WHERE id = ?`

	_, err := db.conn.Exec(query, benchmarkID)
	if err != nil {
		return fmt.Errorf("failed to update benchmark usage: %w", err)
	}

	return nil
}

// ApproveBenchmark утверждает эталон
func (db *ServiceDB) ApproveBenchmark(benchmarkID int, approvedBy string) error {
	query := `
		UPDATE client_benchmarks
		SET is_approved = TRUE, approved_by = ?, approved_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`

	_, err := db.conn.Exec(query, approvedBy, benchmarkID)
	if err != nil {
		return fmt.Errorf("failed to approve benchmark: %w", err)
	}

	return nil
}

// GetNormalizationConfig получает конфигурацию нормализации
func (db *ServiceDB) GetNormalizationConfig() (*NormalizationConfig, error) {
	query := `
		SELECT id, database_path, source_table, reference_column, code_column, name_column, created_at, updated_at
		FROM normalization_config
		WHERE id = 1
	`

	row := db.conn.QueryRow(query)
	config := &NormalizationConfig{}

	err := row.Scan(
		&config.ID, &config.DatabasePath, &config.SourceTable,
		&config.ReferenceColumn, &config.CodeColumn, &config.NameColumn,
		&config.CreatedAt, &config.UpdatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			// Возвращаем дефолтную конфигурацию
			return &NormalizationConfig{
				ID:              1,
				DatabasePath:    "",
				SourceTable:     "catalog_items",
				ReferenceColumn: "reference",
				CodeColumn:      "code",
				NameColumn:      "name",
			}, nil
		}
		return nil, fmt.Errorf("failed to get normalization config: %w", err)
	}

	return config, nil
}

// UpdateNormalizationConfig обновляет конфигурацию нормализации
func (db *ServiceDB) UpdateNormalizationConfig(databasePath, sourceTable, referenceColumn, codeColumn, nameColumn string) error {
	query := `
		UPDATE normalization_config
		SET database_path = ?, source_table = ?, reference_column = ?,
		    code_column = ?, name_column = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = 1
	`

	_, err := db.conn.Exec(query, databasePath, sourceTable, referenceColumn, codeColumn, nameColumn)
	if err != nil {
		return fmt.Errorf("failed to update normalization config: %w", err)
	}

	return nil
}

// CreateProjectDatabase создает новую базу данных для проекта
func (db *ServiceDB) CreateProjectDatabase(projectID int, name, filePath, description string, fileSize int64) (*ProjectDatabase, error) {
	query := `
		INSERT INTO project_databases
		(client_project_id, name, file_path, description, file_size, is_active)
		VALUES (?, ?, ?, ?, ?, TRUE)
	`

	result, err := db.conn.Exec(query, projectID, name, filePath, description, fileSize)
	if err != nil {
		return nil, fmt.Errorf("failed to create project database: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("failed to get project database ID: %w", err)
	}

	return db.GetProjectDatabase(int(id))
}

// GetProjectDatabase получает базу данных проекта по ID
func (db *ServiceDB) GetProjectDatabase(id int) (*ProjectDatabase, error) {
	query := `
		SELECT id, client_project_id, name, file_path, description, is_active,
		       file_size, last_used_at, created_at, updated_at
		FROM project_databases WHERE id = ?
	`

	row := db.conn.QueryRow(query, id)
	projectDB := &ProjectDatabase{}

	var lastUsedAt sql.NullTime
	err := row.Scan(
		&projectDB.ID, &projectDB.ClientProjectID, &projectDB.Name, &projectDB.FilePath,
		&projectDB.Description, &projectDB.IsActive, &projectDB.FileSize, &lastUsedAt,
		&projectDB.CreatedAt, &projectDB.UpdatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get project database: %w", err)
	}

	if lastUsedAt.Valid {
		projectDB.LastUsedAt = &lastUsedAt.Time
	}

	return projectDB, nil
}

// GetProjectDatabases получает все базы данных проекта
func (db *ServiceDB) GetProjectDatabases(projectID int, activeOnly bool) ([]*ProjectDatabase, error) {
	query := `
		SELECT id, client_project_id, name, file_path, description, is_active,
		       file_size, last_used_at, created_at, updated_at
		FROM project_databases
		WHERE client_project_id = ?
	`

	args := []interface{}{projectID}

	if activeOnly {
		query += " AND is_active = TRUE"
	}

	query += " ORDER BY created_at DESC"

	rows, err := db.conn.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get project databases: %w", err)
	}
	defer rows.Close()

	var databases []*ProjectDatabase
	for rows.Next() {
		projectDB := &ProjectDatabase{}
		var lastUsedAt sql.NullTime

		err := rows.Scan(
			&projectDB.ID, &projectDB.ClientProjectID, &projectDB.Name, &projectDB.FilePath,
			&projectDB.Description, &projectDB.IsActive, &projectDB.FileSize, &lastUsedAt,
			&projectDB.CreatedAt, &projectDB.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan project database: %w", err)
		}

		if lastUsedAt.Valid {
			projectDB.LastUsedAt = &lastUsedAt.Time
		}

		databases = append(databases, projectDB)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating project databases: %w", err)
	}

	return databases, nil
}

// UpdateProjectDatabase обновляет базу данных проекта
func (db *ServiceDB) UpdateProjectDatabase(id int, name, filePath, description string, isActive bool) error {
	query := `
		UPDATE project_databases
		SET name = ?, file_path = ?, description = ?, is_active = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`

	_, err := db.conn.Exec(query, name, filePath, description, isActive, id)
	if err != nil {
		return fmt.Errorf("failed to update project database: %w", err)
	}

	return nil
}

// DeleteProjectDatabase удаляет базу данных проекта
func (db *ServiceDB) DeleteProjectDatabase(id int) error {
	query := `DELETE FROM project_databases WHERE id = ?`

	_, err := db.conn.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to delete project database: %w", err)
	}

	return nil
}

// UpdateProjectDatabaseLastUsed обновляет время последнего использования базы данных
func (db *ServiceDB) UpdateProjectDatabaseLastUsed(id int) error {
	query := `
		UPDATE project_databases
		SET last_used_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`

	_, err := db.conn.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to update last_used_at: %w", err)
	}

	return nil
}

// GetQualityMetricsForProject получает метрики качества для проекта
func (db *ServiceDB) GetQualityMetricsForProject(projectID int, period string) ([]DataQualityMetric, error) {
	query := `
		SELECT 
			id, upload_id, database_id, metric_category, metric_name, 
			metric_value, threshold_value, status, measured_at, details
		FROM data_quality_metrics
		WHERE database_id IN (
			SELECT id FROM project_databases 
			WHERE client_project_id = ?
		)
		AND measured_at >= ?
		ORDER BY measured_at DESC
	`

	var timeRange time.Time
	switch period {
	case "day":
		timeRange = time.Now().AddDate(0, 0, -1)
	case "week":
		timeRange = time.Now().AddDate(0, 0, -7)
	case "month":
		timeRange = time.Now().AddDate(0, -1, 0)
	default:
		timeRange = time.Now().AddDate(-1, 0, 0) // default to 1 year
	}

	rows, err := db.conn.Query(query, projectID, timeRange)
	if err != nil {
		return nil, fmt.Errorf("failed to get quality metrics: %w", err)
	}
	defer rows.Close()

	var metrics []DataQualityMetric
	for rows.Next() {
		var metric DataQualityMetric
		var details string

		err := rows.Scan(
			&metric.ID, &metric.UploadID, &metric.DatabaseID,
			&metric.MetricCategory, &metric.MetricName,
			&metric.MetricValue, &metric.ThresholdValue,
			&metric.Status, &metric.MeasuredAt, &details,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan metric: %w", err)
		}

		// Десериализация details из JSON
		if details != "" {
			if err := json.Unmarshal([]byte(details), &metric.Details); err != nil {
				log.Printf("Error unmarshaling metric details: %v", err)
			}
		}

		metrics = append(metrics, metric)
	}

	return metrics, nil
}

// GetQualityTrendsForClient получает тренды качества для клиента
func (db *ServiceDB) GetQualityTrendsForClient(clientID int, period string) ([]QualityTrend, error) {
	query := `
		SELECT 
			id, database_id, measurement_date, 
			overall_score, completeness_score, 
			consistency_score, uniqueness_score, 
			validity_score, records_analyzed, 
			issues_count, created_at
		FROM quality_trends
		WHERE database_id IN (
			SELECT id FROM project_databases 
			WHERE client_project_id IN (
				SELECT id FROM client_projects 
				WHERE client_id = ?
			)
		)
		AND measurement_date >= ?
		ORDER BY measurement_date ASC
	`

	var timeRange time.Time
	switch period {
	case "week":
		timeRange = time.Now().AddDate(0, 0, -7)
	case "month":
		timeRange = time.Now().AddDate(0, -1, 0)
	case "quarter":
		timeRange = time.Now().AddDate(0, -3, 0)
	default:
		timeRange = time.Now().AddDate(-1, 0, 0) // default to 1 year
	}

	rows, err := db.conn.Query(query, clientID, timeRange)
	if err != nil {
		return nil, fmt.Errorf("failed to get quality trends: %w", err)
	}
	defer rows.Close()

	var trends []QualityTrend
	for rows.Next() {
		var trend QualityTrend
		err := rows.Scan(
			&trend.ID, &trend.DatabaseID, &trend.MeasurementDate,
			&trend.OverallScore, &trend.CompletenessScore,
			&trend.ConsistencyScore, &trend.UniquenessScore,
			&trend.ValidityScore, &trend.RecordsAnalyzed,
			&trend.IssuesCount, &trend.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan trend: %w", err)
		}
		trends = append(trends, trend)
	}

	return trends, nil
}

// CompareProjectsQuality сравнивает метрики качества между проектами
func (db *ServiceDB) CompareProjectsQuality(projectIDs []int) (map[int][]DataQualityMetric, error) {
	query := `
		SELECT 
			id, upload_id, database_id, metric_category, metric_name, 
			metric_value, threshold_value, status, measured_at, details
		FROM data_quality_metrics
		WHERE database_id IN (
			SELECT id FROM project_databases 
			WHERE client_project_id IN (` + placeholders(len(projectIDs)) + `)
		)
		AND measured_at >= (
			SELECT MAX(measured_at) FROM data_quality_metrics
			WHERE database_id IN (
				SELECT id FROM project_databases 
				WHERE client_project_id IN (` + placeholders(len(projectIDs)) + `)
			)
		)
	`

	args := make([]interface{}, 0, len(projectIDs)*2)
	for _, id := range projectIDs {
		args = append(args, id)
	}
	for _, id := range projectIDs {
		args = append(args, id)
	}

	rows, err := db.conn.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to compare projects: %w", err)
	}
	defer rows.Close()

	results := make(map[int][]DataQualityMetric)
	for rows.Next() {
		var metric DataQualityMetric
		var details string
		var dbID int

		err := rows.Scan(
			&metric.ID, &metric.UploadID, &dbID,
			&metric.MetricCategory, &metric.MetricName,
			&metric.MetricValue, &metric.ThresholdValue,
			&metric.Status, &metric.MeasuredAt, &details,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan metric: %w", err)
		}

		// Десериализация details из JSON
		if details != "" {
			if err := json.Unmarshal([]byte(details), &metric.Details); err != nil {
				log.Printf("Error unmarshaling metric details: %v", err)
			}
		}

		// Получаем projectID для текущей базы данных
		var projectID int
		err = db.conn.QueryRow("SELECT client_project_id FROM project_databases WHERE id = ?", dbID).Scan(&projectID)
		if err != nil {
			log.Printf("Error getting project ID for database %d: %v", dbID, err)
			continue
		}

		results[projectID] = append(results[projectID], metric)
	}

	return results, nil
}

// placeholders генерирует строку с n плейсхолдерами для SQL запроса
func placeholders(n int) string {
	ph := make([]string, n)
	for i := range ph {
		ph[i] = "?"
	}
	return strings.Join(ph, ",")
}

// DatabaseMetadata структура метаданных базы данных
type DatabaseMetadata struct {
	ID            int       `json:"id"`
	FilePath      string    `json:"file_path"`
	DatabaseType  string    `json:"database_type"`
	Description   string    `json:"description"`
	FirstSeenAt    time.Time `json:"first_seen_at"`
	LastAnalyzedAt *time.Time `json:"last_analyzed_at"`
	MetadataJSON  string    `json:"metadata_json"`
}

// GetDatabaseMetadata получает метаданные базы данных по пути
func (db *ServiceDB) GetDatabaseMetadata(filePath string) (*DatabaseMetadata, error) {
	query := `
		SELECT id, file_path, database_type, description, first_seen_at, last_analyzed_at, metadata_json
		FROM database_metadata
		WHERE file_path = ?
	`

	row := db.conn.QueryRow(query, filePath)
	metadata := &DatabaseMetadata{}
	var lastAnalyzedAt sql.NullTime

	err := row.Scan(
		&metadata.ID, &metadata.FilePath, &metadata.DatabaseType, &metadata.Description,
		&metadata.FirstSeenAt, &lastAnalyzedAt, &metadata.MetadataJSON,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get database metadata: %w", err)
	}

	if lastAnalyzedAt.Valid {
		metadata.LastAnalyzedAt = &lastAnalyzedAt.Time
	}

	return metadata, nil
}

// UpsertDatabaseMetadata создает или обновляет метаданные базы данных
func (db *ServiceDB) UpsertDatabaseMetadata(filePath, databaseType, description, metadataJSON string) error {
	query := `
		INSERT INTO database_metadata (file_path, database_type, description, last_analyzed_at, metadata_json)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP, ?)
		ON CONFLICT(file_path) DO UPDATE SET
			database_type = ?,
			description = ?,
			last_analyzed_at = CURRENT_TIMESTAMP,
			metadata_json = ?
	`

	_, err := db.conn.Exec(query, filePath, databaseType, description, metadataJSON,
		databaseType, description, metadataJSON)
	if err != nil {
		return fmt.Errorf("failed to upsert database metadata: %w", err)
	}

	return nil
}

// GetAllDatabaseMetadata получает все метаданные баз данных
func (db *ServiceDB) GetAllDatabaseMetadata() ([]*DatabaseMetadata, error) {
	query := `
		SELECT id, file_path, database_type, description, first_seen_at, last_analyzed_at, metadata_json
		FROM database_metadata
		ORDER BY last_analyzed_at DESC NULLS LAST, first_seen_at DESC
	`

	rows, err := db.conn.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to get all database metadata: %w", err)
	}
	defer rows.Close()

	var metadataList []*DatabaseMetadata
	for rows.Next() {
		metadata := &DatabaseMetadata{}
		var lastAnalyzedAt sql.NullTime

		err := rows.Scan(
			&metadata.ID, &metadata.FilePath, &metadata.DatabaseType, &metadata.Description,
			&metadata.FirstSeenAt, &lastAnalyzedAt, &metadata.MetadataJSON,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan database metadata: %w", err)
		}

		if lastAnalyzedAt.Valid {
			metadata.LastAnalyzedAt = &lastAnalyzedAt.Time
		}

		metadataList = append(metadataList, metadata)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating database metadata: %w", err)
	}

	return metadataList, nil
}

// GetWorkerConfig получает конфигурацию воркеров из БД
func (db *ServiceDB) GetWorkerConfig() (string, error) {
	query := `SELECT config_json FROM worker_config WHERE id = 1`
	var configJSON string
	err := db.conn.QueryRow(query).Scan(&configJSON)
	if err == sql.ErrNoRows {
		return "", nil // Конфигурация еще не сохранена
	}
	if err != nil {
		return "", fmt.Errorf("failed to get worker config: %w", err)
	}
	return configJSON, nil
}

// SaveWorkerConfig сохраняет конфигурацию воркеров в БД
func (db *ServiceDB) SaveWorkerConfig(configJSON string) error {
	query := `
		INSERT INTO worker_config (id, config_json, updated_at)
		VALUES (1, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(id) DO UPDATE SET
			config_json = ?,
			updated_at = CURRENT_TIMESTAMP
	`
	_, err := db.conn.Exec(query, configJSON, configJSON)
	if err != nil {
		return fmt.Errorf("failed to save worker config: %w", err)
	}
	return nil
}