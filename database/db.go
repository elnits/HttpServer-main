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

// DB обертка для работы с базой данных
type DB struct {
	conn *sql.DB
}

// Upload представляет выгрузку из 1С
type Upload struct {
	ID             int        `json:"id"`
	UploadUUID     string     `json:"upload_uuid"`
	StartedAt      time.Time  `json:"started_at"`
	CompletedAt    *time.Time `json:"completed_at"`
	Status         string     `json:"status"`
	Version1C      string     `json:"version_1c"`
	ConfigName     string     `json:"config_name"`
	TotalConstants int        `json:"total_constants"`
	TotalCatalogs  int        `json:"total_catalogs"`
	TotalItems     int        `json:"total_items"`
	DatabaseID     *int       `json:"database_id,omitempty"`
	ClientID       *int       `json:"client_id,omitempty"`
	ProjectID      *int       `json:"project_id,omitempty"`
	ComputerName   string     `json:"computer_name,omitempty"`
	UserName       string     `json:"user_name,omitempty"`
	ConfigVersion  string     `json:"config_version,omitempty"`
	// Поля для итераций
	IterationNumber int    `json:"iteration_number"`
	IterationLabel  string `json:"iteration_label,omitempty"`
	ProgrammerName  string `json:"programmer_name,omitempty"`
	UploadPurpose   string `json:"upload_purpose,omitempty"`
	ParentUploadID  *int   `json:"parent_upload_id,omitempty"`
}

// Constant представляет константу из 1С
type Constant struct {
	ID        int       `json:"id"`
	UploadID  int       `json:"upload_id"`
	Name      string    `json:"name"`
	Synonym   string    `json:"synonym"`
	Type      string    `json:"type"`
	Value     string    `json:"value"`
	CreatedAt time.Time `json:"created_at"`
}

// Catalog представляет справочник из 1С
type Catalog struct {
	ID        int       `json:"id"`
	UploadID  int       `json:"upload_id"`
	Name      string    `json:"name"`
	Synonym   string    `json:"synonym"`
	CreatedAt time.Time `json:"created_at"`
}

// CatalogItem представляет элемент справочника из 1С
type CatalogItem struct {
	ID          int       `json:"id" xml:"id"`
	CatalogID   int       `json:"catalog_id" xml:"catalog_id"`
	CatalogName string    `json:"catalog_name" xml:"catalog_name"` // Имя справочника
	Reference   string    `json:"reference" xml:"reference"`
	Code        string    `json:"code" xml:"code"`
	Name        string    `json:"name" xml:"name"`
	Attributes  string    `json:"attributes" xml:"attributes"`  // XML строка
	TableParts  string    `json:"table_parts" xml:"table_parts"`  // XML строка
	CreatedAt   time.Time `json:"created_at" xml:"created_at"`
}

// NewDB создает новое подключение к базе данных
func NewDB(dbPath string) (*DB, error) {
	return NewDBWithConfig(dbPath, DBConfig{})
}

// NewDBWithConfig создает новое подключение к базе данных с конфигурацией
func NewDBWithConfig(dbPath string, config DBConfig) (*DB, error) {
	conn, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Настройка connection pooling
	if config.MaxOpenConns > 0 {
		conn.SetMaxOpenConns(config.MaxOpenConns)
	} else {
		conn.SetMaxOpenConns(25) // Значение по умолчанию
	}

	if config.MaxIdleConns > 0 {
		conn.SetMaxIdleConns(config.MaxIdleConns)
	} else {
		conn.SetMaxIdleConns(5) // Значение по умолчанию
	}

	if config.ConnMaxLifetime > 0 {
		conn.SetConnMaxLifetime(config.ConnMaxLifetime)
	} else {
		conn.SetConnMaxLifetime(5 * time.Minute) // Значение по умолчанию
	}

	// Проверяем подключение
	if err := conn.Ping(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	db := &DB{conn: conn}
	
	// Инициализируем схему
	if err := InitSchema(conn); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return db, nil
}

// CreateNewDatabaseWithSchema создает новую базу данных с инициализированной схемой
// Используется для создания отдельной БД для каждой выгрузки
func CreateNewDatabaseWithSchema(dbPath string, config DBConfig) (*DB, error) {
	// Создаем новую БД (SQLite создаст файл автоматически при первом подключении)
	return NewDBWithConfig(dbPath, config)
}

// Close закрывает подключение к базе данных
func (db *DB) Close() error {
	return db.conn.Close()
}

// GetDB возвращает указатель на sql.DB для прямого доступа
func (db *DB) GetDB() *sql.DB {
	return db.conn
}

// CreateUpload создает новую выгрузку
func (db *DB) CreateUpload(uploadUUID, version1C, configName string) (*Upload, error) {
	return db.CreateUploadWithDatabase(uploadUUID, version1C, configName, nil, "", "", "", 1, "", "", "", nil)
}

// CreateUploadWithDatabase создает новую выгрузку с привязкой к базе данных
func (db *DB) CreateUploadWithDatabase(uploadUUID, version1C, configName string, databaseID *int, computerName, userName, configVersion string, iterationNumber int, iterationLabel, programmerName, uploadPurpose string, parentUploadID *int) (*Upload, error) {
	// Если указан database_id, получаем информацию о клиенте и проекте
	var clientID, projectID *int
	if databaseID != nil {
		// Получаем информацию о базе данных из service_db
		// Это будет реализовано через сервер, который имеет доступ к serviceDB
		// Здесь мы просто сохраняем database_id
	}

	// Если iterationNumber не указан, используем значение по умолчанию 1
	if iterationNumber <= 0 {
		iterationNumber = 1
	}
	
	query := `
		INSERT INTO uploads (upload_uuid, version_1c, config_name, status, database_id, client_id, project_id, computer_name, user_name, config_version, iteration_number, iteration_label, programmer_name, upload_purpose, parent_upload_id)
		VALUES (?, ?, ?, 'in_progress', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	
	result, err := db.conn.Exec(query, uploadUUID, version1C, configName, databaseID, clientID, projectID, computerName, userName, configVersion, iterationNumber, iterationLabel, programmerName, uploadPurpose, parentUploadID)
	if err != nil {
		return nil, fmt.Errorf("failed to create upload: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("failed to get upload ID: %w", err)
	}

	return db.GetUploadByID(int(id))
}

// GetUploadByID получает выгрузку по ID
func (db *DB) GetUploadByID(id int) (*Upload, error) {
	query := `
		SELECT id, upload_uuid, started_at, completed_at, status, 
		       version_1c, config_name, total_constants, total_catalogs, total_items,
		       database_id, client_id, project_id, computer_name, user_name, config_version,
		       iteration_number, iteration_label, programmer_name, upload_purpose, parent_upload_id
		FROM uploads WHERE id = ?
	`
	
	row := db.conn.QueryRow(query, id)
	upload := &Upload{}
	var databaseID, clientID, projectID, parentUploadID sql.NullInt64
	var completedAt sql.NullTime
	
	err := row.Scan(
		&upload.ID, &upload.UploadUUID, &upload.StartedAt, &completedAt,
		&upload.Status, &upload.Version1C, &upload.ConfigName,
		&upload.TotalConstants, &upload.TotalCatalogs, &upload.TotalItems,
		&databaseID, &clientID, &projectID,
		&upload.ComputerName, &upload.UserName, &upload.ConfigVersion,
		&upload.IterationNumber, &upload.IterationLabel, &upload.ProgrammerName, &upload.UploadPurpose, &parentUploadID,
	)
	
	if err != nil {
		return nil, fmt.Errorf("failed to get upload: %w", err)
	}
	
	if databaseID.Valid {
		val := int(databaseID.Int64)
		upload.DatabaseID = &val
	}
	if clientID.Valid {
		val := int(clientID.Int64)
		upload.ClientID = &val
	}
	if projectID.Valid {
		val := int(projectID.Int64)
		upload.ProjectID = &val
	}
	if parentUploadID.Valid {
		val := int(parentUploadID.Int64)
		upload.ParentUploadID = &val
	}
	if completedAt.Valid {
		upload.CompletedAt = &completedAt.Time
	}
	
	return upload, nil
}

// GetUploadByUUID получает выгрузку по UUID
func (db *DB) GetUploadByUUID(uuid string) (*Upload, error) {
	query := `
		SELECT id, upload_uuid, started_at, completed_at, status, 
		       version_1c, config_name, total_constants, total_catalogs, total_items,
		       database_id, client_id, project_id, computer_name, user_name, config_version,
		       iteration_number, iteration_label, programmer_name, upload_purpose, parent_upload_id
		FROM uploads WHERE upload_uuid = ?
	`
	
	row := db.conn.QueryRow(query, uuid)
	upload := &Upload{}
	
	var databaseID, clientID, projectID, parentUploadID sql.NullInt64
	var completedAt sql.NullTime
	
	err := row.Scan(
		&upload.ID, &upload.UploadUUID, &upload.StartedAt, &completedAt,
		&upload.Status, &upload.Version1C, &upload.ConfigName,
		&upload.TotalConstants, &upload.TotalCatalogs, &upload.TotalItems,
		&databaseID, &clientID, &projectID,
		&upload.ComputerName, &upload.UserName, &upload.ConfigVersion,
		&upload.IterationNumber, &upload.IterationLabel, &upload.ProgrammerName, &upload.UploadPurpose, &parentUploadID,
	)
	
	if err != nil {
		return nil, fmt.Errorf("failed to get upload by UUID: %w", err)
	}
	
	if databaseID.Valid {
		id := int(databaseID.Int64)
		upload.DatabaseID = &id
	}
	if clientID.Valid {
		id := int(clientID.Int64)
		upload.ClientID = &id
	}
	if projectID.Valid {
		id := int(projectID.Int64)
		upload.ProjectID = &id
	}
	if parentUploadID.Valid {
		id := int(parentUploadID.Int64)
		upload.ParentUploadID = &id
	}
	if completedAt.Valid {
		upload.CompletedAt = &completedAt.Time
	}
	
	return upload, nil
}

// CompleteUpload завершает выгрузку
func (db *DB) CompleteUpload(uploadID int) error {
	query := `
		UPDATE uploads 
		SET completed_at = CURRENT_TIMESTAMP, status = 'completed'
		WHERE id = ?
	`
	
	_, err := db.conn.Exec(query, uploadID)
	if err != nil {
		return fmt.Errorf("failed to complete upload: %w", err)
	}
	
	return nil
}

// AddConstant добавляет константу
func (db *DB) AddConstant(uploadID int, name, synonym, constType, value string) error {
	// Используем транзакцию для атомарности операций
	tx, err := db.conn.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()
	
	query := `
		INSERT INTO constants (upload_id, name, synonym, type, value)
		VALUES (?, ?, ?, ?, ?)
	`
	
	_, err = tx.Exec(query, uploadID, name, synonym, constType, value)
	if err != nil {
		return fmt.Errorf("failed to add constant: %w", err)
	}
	
	// Обновляем счетчик в той же транзакции
	_, err = tx.Exec("UPDATE uploads SET total_constants = total_constants + 1 WHERE id = ?", uploadID)
	if err != nil {
		return fmt.Errorf("failed to update constants counter: %w", err)
	}
	
	// Подтверждаем транзакцию
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	
	return nil
}

// AddCatalog добавляет справочник
func (db *DB) AddCatalog(uploadID int, name, synonym string) (*Catalog, error) {
	// Используем транзакцию для атомарности операций
	tx, err := db.conn.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()
	
	query := `
		INSERT INTO catalogs (upload_id, name, synonym)
		VALUES (?, ?, ?)
	`
	
	result, err := tx.Exec(query, uploadID, name, synonym)
	if err != nil {
		return nil, fmt.Errorf("failed to add catalog: %w", err)
	}
	
	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("failed to get catalog ID: %w", err)
	}
	
	// Обновляем счетчик в той же транзакции
	_, err = tx.Exec("UPDATE uploads SET total_catalogs = total_catalogs + 1 WHERE id = ?", uploadID)
	if err != nil {
		return nil, fmt.Errorf("failed to update catalogs counter: %w", err)
	}
	
	// Подтверждаем транзакцию
	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}
	
	return &Catalog{
		ID:       int(id),
		UploadID: uploadID,
		Name:     name,
		Synonym:  synonym,
	}, nil
}

// AddCatalogItem добавляет элемент справочника
func (db *DB) AddCatalogItem(catalogID int, reference, code, name string, attributes, tableParts interface{}) error {
	var attrsXML, partsXML string
	
	// ОТЛАДКА: Логируем входящие данные
	log.Printf("[DEBUG DB] --- AddCatalogItem ---")
	log.Printf("[DEBUG DB] catalogID: %d", catalogID)
	log.Printf("[DEBUG DB] reference: %s", reference)
	log.Printf("[DEBUG DB] code: %s", code)
	log.Printf("[DEBUG DB] name: %s", name)
	log.Printf("[DEBUG DB] attributes тип: %T", attributes)
	if attributes != nil {
		log.Printf("[DEBUG DB] attributes значение (длина): %d", func() int {
			if str, ok := attributes.(string); ok {
				return len(str)
			}
			return 0
		}())
	} else {
		log.Printf("[DEBUG DB] ⚠ attributes = nil")
	}
	
	// Преобразуем в строку (уже XML из 1С)
	if attributes != nil {
		if str, ok := attributes.(string); ok {
			attrsXML = str
			log.Printf("[DEBUG DB] attributes преобразован в строку, длина: %d", len(attrsXML))
			if len(attrsXML) > 0 {
				if len(attrsXML) > 500 {
					log.Printf("[DEBUG DB] attrsXML (первые 500 символов):\n%s...", attrsXML[:500])
				} else {
					log.Printf("[DEBUG DB] Полное содержимое attrsXML:\n%s", attrsXML)
				}
				attrsCount := strings.Count(attrsXML, "<Реквизит")
				log.Printf("[DEBUG DB] Найдено элементов <Реквизит>: %d", attrsCount)
			} else {
				log.Printf("[DEBUG DB] ⚠ ВНИМАНИЕ: attrsXML ПУСТОЙ после преобразования!")
			}
		} else {
			attrsBytes, err := json.Marshal(attributes)
			if err != nil {
				log.Printf("[DEBUG DB] ✗ Ошибка marshal attributes: %v", err)
				return fmt.Errorf("failed to marshal attributes: %w", err)
			}
			attrsXML = string(attrsBytes)
			log.Printf("[DEBUG DB] attributes преобразован через JSON, длина: %d", len(attrsXML))
		}
	} else {
		log.Printf("[DEBUG DB] ⚠ attributes = nil, attrsXML будет пустым")
	}
	
	if tableParts != nil {
		if str, ok := tableParts.(string); ok {
			partsXML = str
			log.Printf("[DEBUG DB] tableParts преобразован в строку, длина: %d", len(partsXML))
		} else {
			partsBytes, err := json.Marshal(tableParts)
			if err != nil {
				log.Printf("[DEBUG DB] ✗ Ошибка marshal tableParts: %v", err)
				return fmt.Errorf("failed to marshal table parts: %w", err)
			}
			partsXML = string(partsBytes)
			log.Printf("[DEBUG DB] tableParts преобразован через JSON, длина: %d", len(partsXML))
		}
	} else {
		log.Printf("[DEBUG DB] tableParts = nil, partsXML будет пустым")
	}
	
	// Используем транзакцию для атомарности операций
	tx, err := db.conn.Begin()
	if err != nil {
		log.Printf("[DEBUG DB] ✗ Ошибка начала транзакции: %v", err)
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()
	
	query := `
		INSERT INTO catalog_items (catalog_id, reference, code, name, attributes_xml, table_parts_xml)
		VALUES (?, ?, ?, ?, ?, ?)
	`
	
	log.Printf("[DEBUG DB] Выполнение INSERT запроса...")
	log.Printf("[DEBUG DB] Параметры: catalog_id=%d, reference=%s, code=%s, name=%s", catalogID, reference, code, name)
	log.Printf("[DEBUG DB] attributes_xml длина: %d", len(attrsXML))
	log.Printf("[DEBUG DB] table_parts_xml длина: %d", len(partsXML))
	
	_, err = tx.Exec(query, catalogID, reference, code, name, attrsXML, partsXML)
	if err != nil {
		log.Printf("[DEBUG DB] ✗ ОШИБКА при выполнении INSERT: %v", err)
		return fmt.Errorf("failed to add catalog item: %w", err)
	}
	
	log.Printf("[DEBUG DB] ✓ INSERT выполнен успешно")
	
	// Обновляем счетчик в uploads через catalog в той же транзакции
	var uploadID int
	err = tx.QueryRow("SELECT upload_id FROM catalogs WHERE id = ?", catalogID).Scan(&uploadID)
	if err != nil {
		log.Printf("[DEBUG DB] ✗ Ошибка получения upload_id: %v", err)
		return fmt.Errorf("failed to get upload_id for catalog: %w", err)
	}
	
	_, err = tx.Exec("UPDATE uploads SET total_items = total_items + 1 WHERE id = ?", uploadID)
	if err != nil {
		log.Printf("[DEBUG DB] ✗ Ошибка обновления счетчика: %v", err)
		return fmt.Errorf("failed to update items counter: %w", err)
	}
	
	log.Printf("[DEBUG DB] Счетчик total_items обновлен для upload_id=%d", uploadID)
	
	// Подтверждаем транзакцию
	if err = tx.Commit(); err != nil {
		log.Printf("[DEBUG DB] ✗ Ошибка коммита транзакции: %v", err)
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	
	log.Printf("[DEBUG DB] ✓ Транзакция успешно закоммичена")
	log.Printf("[DEBUG DB] ✓ Элемент сохранен в БД: catalog_id=%d, code=%s, name=%s, attributes_xml длина=%d", catalogID, code, name, len(attrsXML))
	log.Printf("[DEBUG DB] --- Конец AddCatalogItem ---")
	
	return nil
}

// NomenclatureItem представляет элемент номенклатуры с характеристикой
type NomenclatureItem struct {
	ID                    int       `json:"id"`
	UploadID              int       `json:"upload_id"`
	NomenclatureReference string    `json:"nomenclature_reference"`
	NomenclatureCode      string    `json:"nomenclature_code"`
	NomenclatureName      string    `json:"nomenclature_name"`
	CharacteristicReference string  `json:"characteristic_reference,omitempty"`
	CharacteristicName     string   `json:"characteristic_name,omitempty"`
	AttributesXML         string    `json:"attributes_xml"`
	TablePartsXML         string    `json:"table_parts_xml"`
	CreatedAt             time.Time `json:"created_at"`
}

// AddNomenclatureItem добавляет элемент номенклатуры с характеристикой
func (db *DB) AddNomenclatureItem(uploadID int, nomenclatureRef, nomenclatureCode, nomenclatureName string, characteristicRef, characteristicName string, attributes, tableParts interface{}) error {
	var attrsXML, partsXML string
	
	// Преобразуем в строку (уже XML из 1С)
	if attributes != nil {
		if str, ok := attributes.(string); ok {
			attrsXML = str
		} else {
			attrsBytes, err := json.Marshal(attributes)
			if err != nil {
				return fmt.Errorf("failed to marshal attributes: %w", err)
			}
			attrsXML = string(attrsBytes)
		}
	}
	
	if tableParts != nil {
		if str, ok := tableParts.(string); ok {
			partsXML = str
		} else {
			partsBytes, err := json.Marshal(tableParts)
			if err != nil {
				return fmt.Errorf("failed to marshal table parts: %w", err)
			}
			partsXML = string(partsBytes)
		}
	}
	
	// Используем транзакцию для атомарности операций
	tx, err := db.conn.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()
	
	query := `
		INSERT INTO nomenclature_items (upload_id, nomenclature_reference, nomenclature_code, nomenclature_name, characteristic_reference, characteristic_name, attributes_xml, table_parts_xml)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`
	
	_, err = tx.Exec(query, uploadID, nomenclatureRef, nomenclatureCode, nomenclatureName, characteristicRef, characteristicName, attrsXML, partsXML)
	if err != nil {
		return fmt.Errorf("failed to add nomenclature item: %w", err)
	}
	
	// Обновляем счетчик в uploads
	_, err = tx.Exec("UPDATE uploads SET total_items = total_items + 1 WHERE id = ?", uploadID)
	if err != nil {
		return fmt.Errorf("failed to update items counter: %w", err)
	}
	
	// Подтверждаем транзакцию
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	
	return nil
}

// AddNomenclatureItemsBatch добавляет пакет элементов номенклатуры
func (db *DB) AddNomenclatureItemsBatch(uploadID int, items []NomenclatureItem) error {
	if len(items) == 0 {
		return nil
	}
	
	tx, err := db.conn.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()
	
	query := `
		INSERT INTO nomenclature_items (upload_id, nomenclature_reference, nomenclature_code, nomenclature_name, characteristic_reference, characteristic_name, attributes_xml, table_parts_xml)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`
	
	stmt, err := tx.Prepare(query)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()
	
	for _, item := range items {
		_, err = stmt.Exec(uploadID, item.NomenclatureReference, item.NomenclatureCode, item.NomenclatureName, 
			item.CharacteristicReference, item.CharacteristicName, item.AttributesXML, item.TablePartsXML)
		if err != nil {
			return fmt.Errorf("failed to add nomenclature item: %w", err)
		}
	}
	
	// Обновляем счетчик в uploads
	_, err = tx.Exec("UPDATE uploads SET total_items = total_items + ? WHERE id = ?", len(items), uploadID)
	if err != nil {
		return fmt.Errorf("failed to update items counter: %w", err)
	}
	
	// Подтверждаем транзакцию
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	
	return nil
}

// QueryRow выполняет запрос и возвращает одну строку
func (db *DB) QueryRow(query string, args ...interface{}) *sql.Row {
	return db.conn.QueryRow(query, args...)
}

// Query выполняет запрос и возвращает несколько строк
func (db *DB) Query(query string, args ...interface{}) (*sql.Rows, error) {
	return db.conn.Query(query, args...)
}

// Exec выполняет запрос без возврата строк (INSERT, UPDATE, DELETE)
func (db *DB) Exec(query string, args ...interface{}) (sql.Result, error) {
	return db.conn.Exec(query, args...)
}

// GetStats получает статистику по выгрузкам
func (db *DB) GetStats() (map[string]interface{}, error) {
	stats := make(map[string]interface{})
	
	// Общее количество выгрузок
	var totalUploads int
	err := db.conn.QueryRow("SELECT COUNT(*) FROM uploads").Scan(&totalUploads)
	if err != nil {
		return nil, fmt.Errorf("failed to get total uploads: %w", err)
	}
	stats["total_uploads"] = totalUploads
	
	// Активные выгрузки
	var activeUploads int
	err = db.conn.QueryRow("SELECT COUNT(*) FROM uploads WHERE status = 'in_progress'").Scan(&activeUploads)
	if err != nil {
		return nil, fmt.Errorf("failed to get active uploads: %w", err)
	}
	stats["active_uploads"] = activeUploads
	
	// Общее количество констант
	var totalConstants int
	err = db.conn.QueryRow("SELECT COUNT(*) FROM constants").Scan(&totalConstants)
	if err != nil {
		return nil, fmt.Errorf("failed to get total constants: %w", err)
	}
	stats["total_constants"] = totalConstants
	
	// Общее количество справочников
	var totalCatalogs int
	err = db.conn.QueryRow("SELECT COUNT(*) FROM catalogs").Scan(&totalCatalogs)
	if err != nil {
		return nil, fmt.Errorf("failed to get total catalogs: %w", err)
	}
	stats["total_catalogs"] = totalCatalogs
	
	// Общее количество элементов
	var totalItems int
	err = db.conn.QueryRow("SELECT COUNT(*) FROM catalog_items").Scan(&totalItems)
	if err != nil {
		return nil, fmt.Errorf("failed to get total items: %w", err)
	}
	stats["total_items"] = totalItems
	
	return stats, nil
}

// GetAllUploads получает список всех выгрузок
func (db *DB) GetAllUploads() ([]*Upload, error) {
	query := `
		SELECT id, upload_uuid, started_at, completed_at, status, 
		       version_1c, config_name, total_constants, total_catalogs, total_items,
		       database_id, client_id, project_id, computer_name, user_name, config_version,
		       iteration_number, iteration_label, programmer_name, upload_purpose, parent_upload_id
		FROM uploads
		ORDER BY started_at DESC
	`
	
	rows, err := db.conn.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to get uploads: %w", err)
	}
	defer rows.Close()
	
	var uploads []*Upload
	for rows.Next() {
		upload := &Upload{}
		var databaseID, clientID, projectID, parentUploadID sql.NullInt64
		var completedAt sql.NullTime
		err := rows.Scan(
			&upload.ID, &upload.UploadUUID, &upload.StartedAt, &completedAt,
			&upload.Status, &upload.Version1C, &upload.ConfigName,
			&upload.TotalConstants, &upload.TotalCatalogs, &upload.TotalItems,
			&databaseID, &clientID, &projectID,
			&upload.ComputerName, &upload.UserName, &upload.ConfigVersion,
			&upload.IterationNumber, &upload.IterationLabel, &upload.ProgrammerName, &upload.UploadPurpose, &parentUploadID,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan upload: %w", err)
		}
		
		if databaseID.Valid {
			id := int(databaseID.Int64)
			upload.DatabaseID = &id
		}
		if clientID.Valid {
			id := int(clientID.Int64)
			upload.ClientID = &id
		}
		if projectID.Valid {
			id := int(projectID.Int64)
			upload.ProjectID = &id
		}
		if parentUploadID.Valid {
			id := int(parentUploadID.Int64)
			upload.ParentUploadID = &id
		}
		if completedAt.Valid {
			upload.CompletedAt = &completedAt.Time
		}
		
		uploads = append(uploads, upload)
	}
	
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating uploads: %w", err)
	}
	
	return uploads, nil
}

// FindSimilarUpload ищет похожую выгрузку по косвенным параметрам
// Возвращает выгрузку с database_id, client_id, project_id если найдена похожая
func (db *DB) FindSimilarUpload(computerName, userName, configName, version1C, configVersion string) (*Upload, error) {
	// Приоритет 1: Точное совпадение всех параметров
	if computerName != "" && userName != "" && configName != "" && version1C != "" {
		query := `
			SELECT id, upload_uuid, started_at, completed_at, status, 
			       version_1c, config_name, total_constants, total_catalogs, total_items,
			       database_id, client_id, project_id, computer_name, user_name, config_version
			FROM uploads
			WHERE computer_name = ? AND user_name = ? AND config_name = ? AND version_1c = ?
			  AND database_id IS NOT NULL
			ORDER BY started_at DESC
			LIMIT 1
		`
		
		upload := &Upload{}
		var databaseID, clientID, projectID sql.NullInt64
		err := db.conn.QueryRow(query, computerName, userName, configName, version1C).Scan(
			&upload.ID, &upload.UploadUUID, &upload.StartedAt, &upload.CompletedAt,
			&upload.Status, &upload.Version1C, &upload.ConfigName,
			&upload.TotalConstants, &upload.TotalCatalogs, &upload.TotalItems,
			&databaseID, &clientID, &projectID,
			&upload.ComputerName, &upload.UserName, &upload.ConfigVersion,
		)
		
		if err == nil {
			if databaseID.Valid {
				id := int(databaseID.Int64)
				upload.DatabaseID = &id
			}
			if clientID.Valid {
				id := int(clientID.Int64)
				upload.ClientID = &id
			}
			if projectID.Valid {
				id := int(projectID.Int64)
				upload.ProjectID = &id
			}
			return upload, nil
		}
	}
	
	// Приоритет 2: Совпадение по computer_name + config_name + version_1c
	if computerName != "" && configName != "" && version1C != "" {
		query := `
			SELECT id, upload_uuid, started_at, completed_at, status, 
			       version_1c, config_name, total_constants, total_catalogs, total_items,
			       database_id, client_id, project_id, computer_name, user_name, config_version
			FROM uploads
			WHERE computer_name = ? AND config_name = ? AND version_1c = ?
			  AND database_id IS NOT NULL
			ORDER BY started_at DESC
			LIMIT 1
		`
		
		upload := &Upload{}
		var databaseID, clientID, projectID sql.NullInt64
		err := db.conn.QueryRow(query, computerName, configName, version1C).Scan(
			&upload.ID, &upload.UploadUUID, &upload.StartedAt, &upload.CompletedAt,
			&upload.Status, &upload.Version1C, &upload.ConfigName,
			&upload.TotalConstants, &upload.TotalCatalogs, &upload.TotalItems,
			&databaseID, &clientID, &projectID,
			&upload.ComputerName, &upload.UserName, &upload.ConfigVersion,
		)
		
		if err == nil {
			if databaseID.Valid {
				id := int(databaseID.Int64)
				upload.DatabaseID = &id
			}
			if clientID.Valid {
				id := int(clientID.Int64)
				upload.ClientID = &id
			}
			if projectID.Valid {
				id := int(projectID.Int64)
				upload.ProjectID = &id
			}
			return upload, nil
		}
	}
	
	// Приоритет 3: Совпадение по computer_name + config_name
	if computerName != "" && configName != "" {
		query := `
			SELECT id, upload_uuid, started_at, completed_at, status, 
			       version_1c, config_name, total_constants, total_catalogs, total_items,
			       database_id, client_id, project_id, computer_name, user_name, config_version
			FROM uploads
			WHERE computer_name = ? AND config_name = ?
			  AND database_id IS NOT NULL
			ORDER BY started_at DESC
			LIMIT 1
		`
		
		upload := &Upload{}
		var databaseID, clientID, projectID sql.NullInt64
		err := db.conn.QueryRow(query, computerName, configName).Scan(
			&upload.ID, &upload.UploadUUID, &upload.StartedAt, &upload.CompletedAt,
			&upload.Status, &upload.Version1C, &upload.ConfigName,
			&upload.TotalConstants, &upload.TotalCatalogs, &upload.TotalItems,
			&databaseID, &clientID, &projectID,
			&upload.ComputerName, &upload.UserName, &upload.ConfigVersion,
		)
		
		if err == nil {
			if databaseID.Valid {
				id := int(databaseID.Int64)
				upload.DatabaseID = &id
			}
			if clientID.Valid {
				id := int(clientID.Int64)
				upload.ClientID = &id
			}
			if projectID.Valid {
				id := int(projectID.Int64)
				upload.ProjectID = &id
			}
			return upload, nil
		}
	}
	
	// Приоритет 4: Совпадение по config_name + version_1c
	if configName != "" && version1C != "" {
		query := `
			SELECT id, upload_uuid, started_at, completed_at, status, 
			       version_1c, config_name, total_constants, total_catalogs, total_items,
			       database_id, client_id, project_id, computer_name, user_name, config_version
			FROM uploads
			WHERE config_name = ? AND version_1c = ?
			  AND database_id IS NOT NULL
			ORDER BY started_at DESC
			LIMIT 1
		`
		
		upload := &Upload{}
		var databaseID, clientID, projectID sql.NullInt64
		err := db.conn.QueryRow(query, configName, version1C).Scan(
			&upload.ID, &upload.UploadUUID, &upload.StartedAt, &upload.CompletedAt,
			&upload.Status, &upload.Version1C, &upload.ConfigName,
			&upload.TotalConstants, &upload.TotalCatalogs, &upload.TotalItems,
			&databaseID, &clientID, &projectID,
			&upload.ComputerName, &upload.UserName, &upload.ConfigVersion,
		)
		
		if err == nil {
			if databaseID.Valid {
				id := int(databaseID.Int64)
				upload.DatabaseID = &id
			}
			if clientID.Valid {
				id := int(clientID.Int64)
				upload.ClientID = &id
			}
			if projectID.Valid {
				id := int(projectID.Int64)
				upload.ProjectID = &id
			}
			return upload, nil
		}
	}
	
	// Приоритет 5: Совпадение только по config_name (самый слабый критерий)
	if configName != "" {
		query := `
			SELECT id, upload_uuid, started_at, completed_at, status, 
			       version_1c, config_name, total_constants, total_catalogs, total_items,
			       database_id, client_id, project_id, computer_name, user_name, config_version
			FROM uploads
			WHERE config_name = ?
			  AND database_id IS NOT NULL
			ORDER BY started_at DESC
			LIMIT 1
		`
		
		upload := &Upload{}
		var databaseID, clientID, projectID sql.NullInt64
		err := db.conn.QueryRow(query, configName).Scan(
			&upload.ID, &upload.UploadUUID, &upload.StartedAt, &upload.CompletedAt,
			&upload.Status, &upload.Version1C, &upload.ConfigName,
			&upload.TotalConstants, &upload.TotalCatalogs, &upload.TotalItems,
			&databaseID, &clientID, &projectID,
			&upload.ComputerName, &upload.UserName, &upload.ConfigVersion,
		)
		
		if err == nil {
			if databaseID.Valid {
				id := int(databaseID.Int64)
				upload.DatabaseID = &id
			}
			if clientID.Valid {
				id := int(clientID.Int64)
				upload.ClientID = &id
			}
			if projectID.Valid {
				id := int(projectID.Int64)
				upload.ProjectID = &id
			}
			return upload, nil
		}
	}
	
	// Не найдено
	return nil, sql.ErrNoRows
}

// GetUploadDetails получает детальную информацию о выгрузке
func (db *DB) GetUploadDetails(uuid string) (*Upload, []*Catalog, []*Constant, error) {
	upload, err := db.GetUploadByUUID(uuid)
	if err != nil {
		return nil, nil, nil, err
	}
	
	// Получаем справочники
	catalogs, err := db.GetCatalogsByUpload(upload.ID)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to get catalogs: %w", err)
	}
	
	// Получаем константы
	constants, err := db.GetConstantsByUpload(upload.ID)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to get constants: %w", err)
	}
	
	return upload, catalogs, constants, nil
}

// GetConstantsByUpload получает все константы выгрузки
func (db *DB) GetConstantsByUpload(uploadID int) ([]*Constant, error) {
	query := `
		SELECT id, upload_id, name, synonym, type, value, created_at
		FROM constants
		WHERE upload_id = ?
		ORDER BY id
	`
	
	rows, err := db.conn.Query(query, uploadID)
	if err != nil {
		return nil, fmt.Errorf("failed to get constants: %w", err)
	}
	defer rows.Close()
	
	var constants []*Constant
	for rows.Next() {
		constant := &Constant{}
		err := rows.Scan(
			&constant.ID, &constant.UploadID, &constant.Name, &constant.Synonym,
			&constant.Type, &constant.Value, &constant.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan constant: %w", err)
		}
		constants = append(constants, constant)
	}
	
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating constants: %w", err)
	}
	
	return constants, nil
}

// GetCatalogsByUpload получает все справочники выгрузки с количеством элементов
func (db *DB) GetCatalogsByUpload(uploadID int) ([]*Catalog, error) {
	query := `
		SELECT c.id, c.upload_id, c.name, c.synonym, c.created_at
		FROM catalogs c
		WHERE c.upload_id = ?
		ORDER BY c.name
	`
	
	rows, err := db.conn.Query(query, uploadID)
	if err != nil {
		return nil, fmt.Errorf("failed to get catalogs: %w", err)
	}
	defer rows.Close()
	
	var catalogs []*Catalog
	for rows.Next() {
		catalog := &Catalog{}
		err := rows.Scan(
			&catalog.ID, &catalog.UploadID, &catalog.Name, &catalog.Synonym, &catalog.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan catalog: %w", err)
		}
		catalogs = append(catalogs, catalog)
	}
	
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating catalogs: %w", err)
	}
	
	return catalogs, nil
}

// GetCatalogItemsByUpload получает элементы справочников выгрузки с фильтрацией и пагинацией
func (db *DB) GetCatalogItemsByUpload(uploadID int, catalogNames []string, offset, limit int) ([]*CatalogItem, int, error) {
	// Строим запрос с фильтрацией
	// Используем правильные имена колонок: attributes_xml и table_parts_xml
	// Включаем все поля из БД, включая catalog_name
	// Используем c.name напрямую, так как catalog_name может отсутствовать в catalog_items
	// Если в будущем добавится колонка catalog_name в catalog_items, можно будет использовать COALESCE
	query := `
		SELECT ci.id, ci.catalog_id, c.name as catalog_name, 
		       ci.reference, ci.code, ci.name, 
		       COALESCE(ci.attributes_xml, '') as attributes, 
		       COALESCE(ci.table_parts_xml, '') as table_parts, 
		       ci.created_at
		FROM catalog_items ci
		INNER JOIN catalogs c ON ci.catalog_id = c.id
		WHERE c.upload_id = ?
	`
	
	args := []interface{}{uploadID}
	
	if len(catalogNames) > 0 {
		query += " AND c.name IN ("
		for i, name := range catalogNames {
			if i > 0 {
				query += ","
			}
			query += "?"
			args = append(args, name)
		}
		query += ")"
	}
	
	// Сортируем по ID элемента, чтобы сохранить порядок вставки в БД
	query += " ORDER BY ci.id"
	
	// Получаем общее количество для пагинации
	var totalCount int
	countQuery := "SELECT COUNT(*) FROM (" + query + ")"
	err := db.conn.QueryRow(countQuery, args...).Scan(&totalCount)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get total count: %w", err)
	}
	
	// Добавляем пагинацию
	if limit > 0 {
		query += " LIMIT ? OFFSET ?"
		args = append(args, limit, offset)
	}
	
	rows, err := db.conn.Query(query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get catalog items: %w", err)
	}
	defer rows.Close()
	
	var items []*CatalogItem
	for rows.Next() {
		item := &CatalogItem{}
		err := rows.Scan(
			&item.ID, &item.CatalogID, &item.CatalogName, &item.Reference, &item.Code, &item.Name,
			&item.Attributes, &item.TableParts, &item.CreatedAt,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan catalog item: %w", err)
		}
		items = append(items, item)
	}
	
	if err = rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating catalog items: %w", err)
	}
	
	return items, totalCount, nil
}

// GetCatalogItemCountByCatalog получает количество элементов для каждого справочника в выгрузке
func (db *DB) GetCatalogItemCountByCatalog(uploadID int) (map[int]int, error) {
	query := `
		SELECT c.id, COUNT(ci.id) as item_count
		FROM catalogs c
		LEFT JOIN catalog_items ci ON ci.catalog_id = c.id
		WHERE c.upload_id = ?
		GROUP BY c.id
	`
	
	rows, err := db.conn.Query(query, uploadID)
	if err != nil {
		return nil, fmt.Errorf("failed to get catalog item counts: %w", err)
	}
	defer rows.Close()
	
	counts := make(map[int]int)
	for rows.Next() {
		var catalogID, itemCount int
		err := rows.Scan(&catalogID, &itemCount)
		if err != nil {
			return nil, fmt.Errorf("failed to scan catalog item count: %w", err)
		}
		counts[catalogID] = itemCount
	}
	
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating catalog item counts: %w", err)
	}
	
	return counts, nil
}

// GetConstantsByUploadWithPagination получает константы с пагинацией
func (db *DB) GetConstantsByUploadWithPagination(uploadID int, limit, offset int) ([]*Constant, error) {
	query := `
		SELECT id, upload_id, name, synonym, type, value, created_at
		FROM constants
		WHERE upload_id = ?
		ORDER BY id
		LIMIT ? OFFSET ?
	`
	
	rows, err := db.conn.Query(query, uploadID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to get constants: %w", err)
	}
	defer rows.Close()
	
	var constants []*Constant
	for rows.Next() {
		constant := &Constant{}
		err := rows.Scan(
			&constant.ID, &constant.UploadID, &constant.Name, &constant.Synonym,
			&constant.Type, &constant.Value, &constant.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan constant: %w", err)
		}
		constants = append(constants, constant)
	}
	
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating constants: %w", err)
	}
	
	return constants, nil
}

// GetCatalogByNameAndUpload находит справочник по имени и upload_id
func (db *DB) GetCatalogByNameAndUpload(catalogName string, uploadID int) (*Catalog, error) {
	query := `
		SELECT id, upload_id, name, synonym, created_at
		FROM catalogs
		WHERE name = ? AND upload_id = ?
	`
	
	catalog := &Catalog{}
	err := db.conn.QueryRow(query, catalogName, uploadID).Scan(
		&catalog.ID, &catalog.UploadID, &catalog.Name, &catalog.Synonym, &catalog.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get catalog: %w", err)
	}
	
	return catalog, nil
}

// GetCatalogItemsCount получает количество элементов в справочнике
func (db *DB) GetCatalogItemsCount(catalogID int) (int, error) {
	query := `
		SELECT COUNT(*) 
		FROM catalog_items 
		WHERE catalog_id = ?
	`
	
	var count int
	err := db.conn.QueryRow(query, catalogID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to get catalog items count: %w", err)
	}
	
	return count, nil
}

// GetCatalogItemsWithPagination получает элементы справочника с пагинацией
func (db *DB) GetCatalogItemsWithPagination(catalogID int, limit, offset int) ([]*CatalogItem, error) {
	query := `
		SELECT id, catalog_id, reference, code, name, 
		       COALESCE(attributes_xml, '') as attributes, 
		       COALESCE(table_parts_xml, '') as table_parts, 
		       created_at
		FROM catalog_items
		WHERE catalog_id = ?
		ORDER BY id
		LIMIT ? OFFSET ?
	`
	
	rows, err := db.conn.Query(query, catalogID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to get catalog items: %w", err)
	}
	defer rows.Close()
	
	var items []*CatalogItem
	for rows.Next() {
		item := &CatalogItem{}
		err := rows.Scan(
			&item.ID, &item.CatalogID, &item.Reference, &item.Code, &item.Name,
			&item.Attributes, &item.TableParts, &item.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan catalog item: %w", err)
		}
		items = append(items, item)
	}
	
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating catalog items: %w", err)
	}
	
	return items, nil
}

// GetAllCatalogs получает все справочники из БД (все выгрузки)
func (db *DB) GetAllCatalogs() ([]*Catalog, error) {
	query := `
		SELECT id, upload_id, name, synonym, created_at
		FROM catalogs
		ORDER BY name
	`
	
	rows, err := db.conn.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to get catalogs: %w", err)
	}
	defer rows.Close()
	
	var catalogs []*Catalog
	for rows.Next() {
		catalog := &Catalog{}
		err := rows.Scan(
			&catalog.ID, &catalog.UploadID, &catalog.Name, &catalog.Synonym, &catalog.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan catalog: %w", err)
		}
		catalogs = append(catalogs, catalog)
	}
	
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating catalogs: %w", err)
	}
	
	return catalogs, nil
}

// CleanOldCatalogItems удаляет все записи из catalog_items кроме последних 15973
func (db *DB) CleanOldCatalogItems() error {
	// Проверяем общее количество записей
	var totalCount int
	err := db.conn.QueryRow("SELECT COUNT(*) FROM catalog_items").Scan(&totalCount)
	if err != nil {
		return fmt.Errorf("failed to get total count: %w", err)
	}

	// Если записей меньше или равно 15973, ничего не удаляем
	if totalCount <= 15973 {
		return nil
	}

	// Начинаем транзакцию
	tx, err := db.conn.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Удаляем все записи кроме последних 15973
	_, err = tx.Exec(`
		DELETE FROM catalog_items 
		WHERE id NOT IN (
			SELECT id FROM catalog_items 
			ORDER BY id DESC 
			LIMIT 15973
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to delete old records: %w", err)
	}

	// Подтверждаем транзакцию
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// NormalizedItem представляет нормализованную запись
type NormalizedItem struct {
	ID                  int       `json:"id"`
	SourceReference     string    `json:"source_reference"`
	SourceName          string    `json:"source_name"`
	Code                string    `json:"code"`
	NormalizedName      string    `json:"normalized_name"`
	NormalizedReference string    `json:"normalized_reference"`
	Category            string    `json:"category"`
	MergedCount         int       `json:"merged_count"`
	AIConfidence        float64   `json:"ai_confidence"`
	AIReasoning         string    `json:"ai_reasoning"`
	ProcessingLevel     string    `json:"processing_level"`
	KpvedCode           string    `json:"kpved_code"`
	KpvedName           string    `json:"kpved_name"`
	KpvedConfidence     float64   `json:"kpved_confidence"`
	QualityScore        float64   `json:"quality_score"`
	CreatedAt           time.Time `json:"created_at"`
}

// ItemAttribute представляет извлеченный атрибут товара
type ItemAttribute struct {
	ID                int       `json:"id"`
	NormalizedItemID  int       `json:"normalized_item_id"`
	AttributeType     string    `json:"attribute_type"`     // dimension, unit, article_code, technical_code, numeric_value, text_value
	AttributeName     string    `json:"attribute_name"`     // width, height, thickness, weight, etc.
	AttributeValue    string    `json:"attribute_value"`    // значение атрибута
	Unit              string    `json:"unit"`               // единица измерения (mm, cm, kg, etc.)
	OriginalText      string    `json:"original_text"`      // исходный текст, из которого извлечен атрибут
	Confidence        float64   `json:"confidence"`         // уверенность в извлечении (0.0-1.0)
	CreatedAt         time.Time `json:"created_at"`
}

// NormalizationSession представляет сессию нормализации для элемента
type NormalizationSession struct {
	ID            int       `json:"id"`
	CatalogItemID int       `json:"catalog_item_id"`
	OriginalName  string    `json:"original_name"`
	CurrentName   string    `json:"current_name"`
	StagesCount   int       `json:"stages_count"`
	Status        string    `json:"status"` // in_progress, completed, reverted
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// NormalizationStage представляет стадию нормализации
type NormalizationStage struct {
	ID                   int       `json:"id"`
	SessionID            int       `json:"session_id"`
	StageType            string    `json:"stage_type"`            // algorithmic, ai_single, ai_chat, manual
	StageName            string    `json:"stage_name"`            // pattern_correction, ai_correction, etc.
	InputName            string    `json:"input_name"`
	OutputName           string    `json:"output_name"`
	AppliedPatterns      string    `json:"applied_patterns"`      // JSON
	AIContext            string    `json:"ai_context"`            // JSON для промптов/ответов
	CategoryOriginal     string    `json:"category_original"`    // JSON
	CategoryFolded      string    `json:"category_folded"`        // JSON
	ClassificationStrategy string  `json:"classification_strategy"`
	Confidence           float64   `json:"confidence"`
	Status               string    `json:"status"`                // applied, pending, rejected
	CreatedAt            time.Time `json:"created_at"`
}

// GetAllCatalogItems получает все записи из catalog_items (устаревший метод для совместимости)
func (db *DB) GetAllCatalogItems() ([]*CatalogItem, error) {
	return db.GetCatalogItemsFromTable("catalog_items", "reference", "code", "name")
}

// GetCatalogItemsFromTable получает все записи из указанной таблицы с указанными колонками
func (db *DB) GetCatalogItemsFromTable(tableName, referenceCol, codeCol, nameCol string) ([]*CatalogItem, error) {
	// Формируем запрос с динамическими именами колонок
	// ВАЖНО: Здесь нет SQL-инъекции т.к. имена таблиц и колонок контролируются через админ-интерфейс
	query := fmt.Sprintf(`
		SELECT
			COALESCE(id, ROW_NUMBER() OVER (ORDER BY %s)) as id,
			COALESCE(catalog_id, 0) as catalog_id,
			%s as reference,
			%s as code,
			%s as name
		FROM %s
		ORDER BY id
	`, nameCol, referenceCol, codeCol, nameCol, tableName)

	rows, err := db.conn.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to get catalog items from %s: %w", tableName, err)
	}
	defer rows.Close()

	var items []*CatalogItem
	for rows.Next() {
		item := &CatalogItem{}
		err := rows.Scan(&item.ID, &item.CatalogID, &item.Reference, &item.Code, &item.Name)
		if err != nil {
			return nil, fmt.Errorf("failed to scan catalog item: %w", err)
		}
		items = append(items, item)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating catalog items: %w", err)
	}

	return items, nil
}

// InsertNormalizedItem вставляет нормализованную запись в normalized_data
func (db *DB) InsertNormalizedItem(sourceReference, sourceName, code, normalizedName, normalizedReference, category string, mergedCount int) error {
	query := `
		INSERT INTO normalized_data 
		(source_reference, source_name, code, normalized_name, normalized_reference, category, merged_count)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`

	_, err := db.conn.Exec(query, sourceReference, sourceName, code, normalizedName, normalizedReference, category, mergedCount)
	if err != nil {
		return fmt.Errorf("failed to insert normalized item: %w", err)
	}

	return nil
}

// InsertNormalizedItemsBatch вставляет пакет нормализованных записей в транзакции
// Возвращает map[code]id для связи с атрибутами
func (db *DB) InsertNormalizedItemsBatch(items []*NormalizedItem) (map[string]int, error) {
	if len(items) == 0 {
		return make(map[string]int), nil
	}

	// Начинаем транзакцию
	tx, err := db.conn.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT OR REPLACE INTO normalized_data
		(source_reference, source_name, code, normalized_name, normalized_reference, category, merged_count, ai_confidence, ai_reasoning, processing_level, kpved_code, kpved_name, kpved_confidence)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	codeToID := make(map[string]int)

	for _, item := range items {
		result, err := stmt.Exec(
			item.SourceReference,
			item.SourceName,
			item.Code,
			item.NormalizedName,
			item.NormalizedReference,
			item.Category,
			item.MergedCount,
			item.AIConfidence,
			item.AIReasoning,
			item.ProcessingLevel,
			item.KpvedCode,
			item.KpvedName,
			item.KpvedConfidence,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to insert normalized item: %w", err)
		}
		
		// Получаем ID вставленной записи
		id, err := result.LastInsertId()
		if err != nil {
			return nil, fmt.Errorf("failed to get last insert id: %w", err)
		}
		
		// ВАЖНО: Если несколько элементов имеют одинаковый код, 
		// в map будет сохранен только ID последнего элемента с таким кодом.
		// Это нормально, если код должен быть уникальным в рамках батча.
		if item.Code != "" {
			codeToID[item.Code] = int(id)
		}
	}

	// Подтверждаем транзакцию
	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return codeToID, nil
}

// GetNormalizedItems получает нормализованные записи с пагинацией
func (db *DB) GetNormalizedItems(offset, limit int) ([]*NormalizedItem, error) {
	query := `
		SELECT id, source_reference, source_name, code, normalized_name,
		       normalized_reference, category, merged_count, ai_confidence,
		       ai_reasoning, processing_level, kpved_code, kpved_name, kpved_confidence, created_at
		FROM normalized_data
		ORDER BY id
	`

	args := []interface{}{}
	if limit > 0 {
		query += " LIMIT ? OFFSET ?"
		args = append(args, limit, offset)
	}

	rows, err := db.conn.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query normalized items: %w", err)
	}
	defer rows.Close()

	var items []*NormalizedItem
	for rows.Next() {
		item := &NormalizedItem{}
		err := rows.Scan(
			&item.ID,
			&item.SourceReference,
			&item.SourceName,
			&item.Code,
			&item.NormalizedName,
			&item.NormalizedReference,
			&item.Category,
			&item.MergedCount,
			&item.AIConfidence,
			&item.AIReasoning,
			&item.ProcessingLevel,
			&item.KpvedCode,
			&item.KpvedName,
			&item.KpvedConfidence,
			&item.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan normalized item: %w", err)
		}
		items = append(items, item)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating normalized items: %w", err)
	}

	return items, nil
}

// InsertItemAttributesBatch вставляет пакет атрибутов для нормализованного товара
func (db *DB) InsertItemAttributesBatch(normalizedItemID int, attributes []*ItemAttribute) error {
	if len(attributes) == 0 {
		return nil
	}

	// Начинаем транзакцию
	tx, err := db.conn.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO normalized_item_attributes
		(normalized_item_id, attribute_type, attribute_name, attribute_value, unit, original_text, confidence)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	for _, attr := range attributes {
		_, err = stmt.Exec(
			normalizedItemID,
			attr.AttributeType,
			attr.AttributeName,
			attr.AttributeValue,
			attr.Unit,
			attr.OriginalText,
			attr.Confidence,
		)
		if err != nil {
			return fmt.Errorf("failed to insert attribute: %w", err)
		}
	}

	// Подтверждаем транзакцию
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// InsertNormalizedItemsWithAttributesBatch вставляет items И их attributes в ОДНОЙ транзакции
// Это гарантирует атомарность: либо вставляется все (items + attributes), либо ничего
// Критично для предотвращения частичной вставки при сбоях
func (db *DB) InsertNormalizedItemsWithAttributesBatch(items []*NormalizedItem, itemAttributes map[string][]*ItemAttribute) (map[string]int, error) {
	if len(items) == 0 {
		return make(map[string]int), nil
	}

	// Начинаем ОДНУ транзакцию для ВСЕХ операций
	tx, err := db.conn.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback() // Откатим если что-то пошло не так

	// Подготавливаем statement для вставки items
	itemStmt, err := tx.Prepare(`
		INSERT OR REPLACE INTO normalized_data
		(source_reference, source_name, code, normalized_name, normalized_reference, category, merged_count, ai_confidence, ai_reasoning, processing_level, kpved_code, kpved_name, kpved_confidence)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare item statement: %w", err)
	}
	defer itemStmt.Close()

	// Подготавливаем statement для вставки attributes
	attrStmt, err := tx.Prepare(`
		INSERT INTO normalized_item_attributes
		(normalized_item_id, attribute_type, attribute_name, attribute_value, unit, original_text, confidence)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare attribute statement: %w", err)
	}
	defer attrStmt.Close()

	codeToID := make(map[string]int)

	// Вставляем items
	for _, item := range items {
		result, err := itemStmt.Exec(
			item.SourceReference,
			item.SourceName,
			item.Code,
			item.NormalizedName,
			item.NormalizedReference,
			item.Category,
			item.MergedCount,
			item.AIConfidence,
			item.AIReasoning,
			item.ProcessingLevel,
			item.KpvedCode,
			item.KpvedName,
			item.KpvedConfidence,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to insert normalized item: %w", err)
		}

		// Получаем ID вставленной записи
		id, err := result.LastInsertId()
		if err != nil {
			return nil, fmt.Errorf("failed to get last insert id: %w", err)
		}

		itemID := int(id)
		if item.Code != "" {
			codeToID[item.Code] = itemID

			// Сразу вставляем attributes для этого item (если есть)
			if attrs, ok := itemAttributes[item.Code]; ok && len(attrs) > 0 {
				for _, attr := range attrs {
					_, err = attrStmt.Exec(
						itemID,
						attr.AttributeType,
						attr.AttributeName,
						attr.AttributeValue,
						attr.Unit,
						attr.OriginalText,
						attr.Confidence,
					)
					if err != nil {
						// Если вставка атрибута упала - откатываем ВСЕ (items + attributes)
						return nil, fmt.Errorf("failed to insert attribute for item %d (code: %s): %w", itemID, item.Code, err)
					}
				}
			}
		}
	}

	// Коммитим транзакцию - либо все вставилось, либо ничего
	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return codeToID, nil
}

// GetItemAttributes получает все атрибуты для нормализованного товара
func (db *DB) GetItemAttributes(normalizedItemID int) ([]*ItemAttribute, error) {
	query := `
		SELECT id, normalized_item_id, attribute_type, attribute_name, attribute_value, 
		       unit, original_text, confidence, created_at
		FROM normalized_item_attributes
		WHERE normalized_item_id = ?
		ORDER BY attribute_type, attribute_name
	`

	rows, err := db.conn.Query(query, normalizedItemID)
	if err != nil {
		return nil, fmt.Errorf("failed to query attributes: %w", err)
	}
	defer rows.Close()

	var attributes []*ItemAttribute
	for rows.Next() {
		attr := &ItemAttribute{}
		err := rows.Scan(
			&attr.ID,
			&attr.NormalizedItemID,
			&attr.AttributeType,
			&attr.AttributeName,
			&attr.AttributeValue,
			&attr.Unit,
			&attr.OriginalText,
			&attr.Confidence,
			&attr.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan attribute: %w", err)
		}
		attributes = append(attributes, attr)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating attributes: %w", err)
	}

	return attributes, nil
}

// UpdateProcessingLevel обновляет уровень обработки и оценку качества
func (db *DB) UpdateProcessingLevel(id int, processingLevel string, qualityScore float64) error {
	query := `
		UPDATE normalized_data
		SET processing_level = ?,
		    ai_confidence = CASE
		        WHEN ? > ai_confidence THEN ?
		        ELSE ai_confidence
		    END
		WHERE id = ?
	`

	_, err := db.conn.Exec(query, processingLevel, qualityScore, qualityScore, id)
	if err != nil {
		return fmt.Errorf("failed to update processing level: %w", err)
	}

	return nil
}

// GetQualityStats получает статистику по качеству нормализации
func (db *DB) GetQualityStats() (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// Общее количество записей
	var totalCount int
	err := db.conn.QueryRow("SELECT COUNT(*) FROM normalized_data").Scan(&totalCount)
	if err != nil {
		return nil, fmt.Errorf("failed to get total count: %w", err)
	}
	stats["total_items"] = totalCount

	// Количество по уровням обработки
	rows, err := db.conn.Query(`
		SELECT 
			COALESCE(processing_level, 'basic') as processing_level, 
			COUNT(*) as count, 
			AVG(CASE WHEN ai_confidence > 0 THEN ai_confidence ELSE NULL END) as avg_quality
		FROM normalized_data
		GROUP BY COALESCE(processing_level, 'basic')
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to get level stats: %w", err)
	}
	defer rows.Close()

	levelStats := make(map[string]map[string]interface{})
	// Базовые оценки качества для каждого уровня (если нет AI оценок)
	levelQualityDefaults := map[string]float64{
		"basic":       50.0,
		"enhanced":    70.0,
		"ai_enhanced": 85.0,
		"benchmark":   95.0,
	}
	
	for rows.Next() {
		var level string
		var count int
		var avgQuality sql.NullFloat64
		if err := rows.Scan(&level, &count, &avgQuality); err != nil {
			return nil, fmt.Errorf("failed to scan level stats: %w", err)
		}
		
		// Используем AI оценку если есть, иначе базовую
		qualityValue := levelQualityDefaults[level]
		if avgQuality.Valid && avgQuality.Float64 > 0 {
			qualityValue = avgQuality.Float64
		}
		
		levelStats[level] = map[string]interface{}{
			"count":        count,
			"avg_quality":  qualityValue,
			"percentage":   float64(count) / float64(totalCount) * 100,
		}
	}
	stats["by_level"] = levelStats

	// Средняя оценка качества
	// Используем ai_confidence если есть, иначе используем базовую оценку на основе processing_level
	var avgQuality sql.NullFloat64
	err = db.conn.QueryRow("SELECT AVG(ai_confidence) FROM normalized_data WHERE ai_confidence > 0").Scan(&avgQuality)
	if err != nil && err.Error() != "sql: Scan error on column index 0, name \"AVG(ai_confidence)\": converting NULL to float64 is unsupported" {
		// Если нет записей с ai_confidence > 0, рассчитываем базовую оценку
		avgQuality = sql.NullFloat64{Valid: false}
	}
	
	if avgQuality.Valid && avgQuality.Float64 > 0 {
		stats["average_quality"] = avgQuality.Float64
	} else {
		// Если нет AI оценок, используем базовую оценку на основе processing_level
		// basic = 50%, enhanced = 70%, ai_enhanced = 85%, benchmark = 95%
		var basicCount, enhancedCount, aiEnhancedCount, benchmarkCount int
		db.conn.QueryRow("SELECT COUNT(*) FROM normalized_data WHERE processing_level = 'basic'").Scan(&basicCount)
		db.conn.QueryRow("SELECT COUNT(*) FROM normalized_data WHERE processing_level = 'enhanced'").Scan(&enhancedCount)
		db.conn.QueryRow("SELECT COUNT(*) FROM normalized_data WHERE processing_level = 'ai_enhanced'").Scan(&aiEnhancedCount)
		db.conn.QueryRow("SELECT COUNT(*) FROM normalized_data WHERE processing_level = 'benchmark'").Scan(&benchmarkCount)
		
		if totalCount > 0 {
			calculatedQuality := (float64(basicCount)*50.0 + 
				float64(enhancedCount)*70.0 + 
				float64(aiEnhancedCount)*85.0 + 
				float64(benchmarkCount)*95.0) / float64(totalCount)
			stats["average_quality"] = calculatedQuality
		} else {
			stats["average_quality"] = 0.0
		}
	}

	// Количество benchmark записей
	var benchmarkCount int
	err = db.conn.QueryRow("SELECT COUNT(*) FROM normalized_data WHERE processing_level = 'benchmark'").Scan(&benchmarkCount)
	if err != nil {
		return nil, fmt.Errorf("failed to get benchmark count: %w", err)
	}
	stats["benchmark_count"] = benchmarkCount
	if totalCount > 0 {
		stats["benchmark_percentage"] = float64(benchmarkCount) / float64(totalCount) * 100
	} else {
		stats["benchmark_percentage"] = 0.0
	}

	return stats, nil
}

// ============================================================================
// Data Quality Structures and Functions
// ============================================================================

// DataQualityMetric представляет метрику качества данных
type DataQualityMetric struct {
	ID            int                    `json:"id"`
	UploadID      int                    `json:"upload_id"`
	DatabaseID    int                    `json:"database_id"`
	MetricCategory string                `json:"metric_category"`
	MetricName    string                 `json:"metric_name"`
	MetricValue   float64                `json:"metric_value"`
	ThresholdValue *float64              `json:"threshold_value,omitempty"`
	Status        string                 `json:"status"`
	MeasuredAt    time.Time              `json:"measured_at"`
	Details       map[string]interface{} `json:"details,omitempty"`
}

// DataQualityIssue представляет проблему качества данных
type DataQualityIssue struct {
	ID             int        `json:"id"`
	UploadID       int        `json:"upload_id"`
	DatabaseID     int        `json:"database_id"`
	EntityType     string     `json:"entity_type"`
	EntityReference string    `json:"entity_reference,omitempty"`
	IssueType      string     `json:"issue_type"`
	IssueSeverity  string     `json:"issue_severity"`
	FieldName      string     `json:"field_name,omitempty"`
	ExpectedValue  string     `json:"expected_value,omitempty"`
	ActualValue    string     `json:"actual_value,omitempty"`
	Description    string     `json:"description"`
	DetectedAt     time.Time  `json:"detected_at"`
	ResolvedAt     *time.Time `json:"resolved_at,omitempty"`
	Status         string     `json:"status"`
}

// QualityTrend представляет тренд качества по базе данных
type QualityTrend struct {
	ID                int       `json:"id"`
	DatabaseID        int       `json:"database_id"`
	MeasurementDate   time.Time `json:"measurement_date"`
	OverallScore      float64   `json:"overall_score"`
	CompletenessScore *float64  `json:"completeness_score,omitempty"`
	ConsistencyScore  *float64  `json:"consistency_score,omitempty"`
	UniquenessScore   *float64  `json:"uniqueness_score,omitempty"`
	ValidityScore     *float64  `json:"validity_score,omitempty"`
	RecordsAnalyzed   int       `json:"records_analyzed"`
	IssuesCount       int       `json:"issues_count"`
	CreatedAt         time.Time `json:"created_at"`
}

// SaveQualityMetric сохраняет метрику качества
func (db *DB) SaveQualityMetric(metric *DataQualityMetric) error {
	detailsJSON := ""
	if metric.Details != nil {
		detailsBytes, err := json.Marshal(metric.Details)
		if err != nil {
			return fmt.Errorf("failed to marshal details: %w", err)
		}
		detailsJSON = string(detailsBytes)
	}

	query := `
		INSERT INTO data_quality_metrics (
			upload_id, database_id, metric_category, metric_name,
			metric_value, threshold_value, status, measured_at, details
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	result, err := db.conn.Exec(query,
		metric.UploadID,
		metric.DatabaseID,
		metric.MetricCategory,
		metric.MetricName,
		metric.MetricValue,
		metric.ThresholdValue,
		metric.Status,
		metric.MeasuredAt,
		detailsJSON,
	)

	if err != nil {
		return fmt.Errorf("failed to save quality metric: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get last insert ID: %w", err)
	}

	metric.ID = int(id)
	return nil
}

// GetQualityMetrics получает метрики качества для выгрузки
func (db *DB) GetQualityMetrics(uploadID int) ([]DataQualityMetric, error) {
	query := `
		SELECT id, upload_id, database_id, metric_category, metric_name,
			metric_value, threshold_value, status, measured_at, details
		FROM data_quality_metrics
		WHERE upload_id = ?
		ORDER BY measured_at DESC
	`

	rows, err := db.conn.Query(query, uploadID)
	if err != nil {
		return nil, fmt.Errorf("failed to query quality metrics: %w", err)
	}
	defer rows.Close()

	var metrics []DataQualityMetric
	for rows.Next() {
		var metric DataQualityMetric
		var thresholdValue sql.NullFloat64
		var detailsJSON sql.NullString

		err := rows.Scan(
			&metric.ID,
			&metric.UploadID,
			&metric.DatabaseID,
			&metric.MetricCategory,
			&metric.MetricName,
			&metric.MetricValue,
			&thresholdValue,
			&metric.Status,
			&metric.MeasuredAt,
			&detailsJSON,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan quality metric: %w", err)
		}

		if thresholdValue.Valid {
			val := thresholdValue.Float64
			metric.ThresholdValue = &val
		}

		if detailsJSON.Valid && detailsJSON.String != "" {
			if err := json.Unmarshal([]byte(detailsJSON.String), &metric.Details); err != nil {
				metric.Details = make(map[string]interface{})
			}
		}

		metrics = append(metrics, metric)
	}

	return metrics, nil
}

// SaveQualityIssue сохраняет проблему качества
func (db *DB) SaveQualityIssue(issue *DataQualityIssue) error {
	query := `
		INSERT INTO data_quality_issues (
			upload_id, database_id, entity_type, entity_reference,
			issue_type, issue_severity, field_name, expected_value,
			actual_value, description, detected_at, status
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	result, err := db.conn.Exec(query,
		issue.UploadID,
		issue.DatabaseID,
		issue.EntityType,
		issue.EntityReference,
		issue.IssueType,
		issue.IssueSeverity,
		issue.FieldName,
		issue.ExpectedValue,
		issue.ActualValue,
		issue.Description,
		issue.DetectedAt,
		issue.Status,
	)

	if err != nil {
		return fmt.Errorf("failed to save quality issue: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get last insert ID: %w", err)
	}

	issue.ID = int(id)
	return nil
}

// GetQualityIssues получает проблемы качества с фильтрацией и пагинацией
// Возвращает список issues, общее количество и ошибку
func (db *DB) GetQualityIssues(uploadID int, filters map[string]interface{}, limit, offset int) ([]DataQualityIssue, int, error) {
	// Сначала получаем общее количество для пагинации
	countQuery := `SELECT COUNT(*) FROM data_quality_issues WHERE upload_id = ?`
	countArgs := []interface{}{uploadID}

	if entityType, ok := filters["entity_type"]; ok {
		countQuery += " AND entity_type = ?"
		countArgs = append(countArgs, entityType)
	}

	if severity, ok := filters["severity"]; ok {
		countQuery += " AND issue_severity = ?"
		countArgs = append(countArgs, severity)
	}

	if status, ok := filters["status"]; ok {
		countQuery += " AND status = ?"
		countArgs = append(countArgs, status)
	}

	var totalCount int
	err := db.conn.QueryRow(countQuery, countArgs...).Scan(&totalCount)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count quality issues: %w", err)
	}

	// Теперь получаем данные с пагинацией
	query := `SELECT id, upload_id, database_id, entity_type, entity_reference,
		issue_type, issue_severity, field_name, expected_value, actual_value,
		description, detected_at, resolved_at, status
		FROM data_quality_issues
		WHERE upload_id = ?`

	args := []interface{}{uploadID}

	if entityType, ok := filters["entity_type"]; ok {
		query += " AND entity_type = ?"
		args = append(args, entityType)
	}

	if severity, ok := filters["severity"]; ok {
		query += " AND issue_severity = ?"
		args = append(args, severity)
	}

	if status, ok := filters["status"]; ok {
		query += " AND status = ?"
		args = append(args, status)
	}

	query += " ORDER BY detected_at DESC"

	// Добавляем пагинацию только если указаны limit и offset
	if limit > 0 {
		query += " LIMIT ? OFFSET ?"
		args = append(args, limit, offset)
	}

	rows, err := db.conn.Query(query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query quality issues: %w", err)
	}
	defer rows.Close()

	var issues []DataQualityIssue
	for rows.Next() {
		var issue DataQualityIssue
		var resolvedAt sql.NullTime

		err := rows.Scan(
			&issue.ID,
			&issue.UploadID,
			&issue.DatabaseID,
			&issue.EntityType,
			&issue.EntityReference,
			&issue.IssueType,
			&issue.IssueSeverity,
			&issue.FieldName,
			&issue.ExpectedValue,
			&issue.ActualValue,
			&issue.Description,
			&issue.DetectedAt,
			&resolvedAt,
			&issue.Status,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan quality issue: %w", err)
		}

		if resolvedAt.Valid {
			issue.ResolvedAt = &resolvedAt.Time
		}

		issues = append(issues, issue)
	}

	return issues, totalCount, nil
}

// UpdateQualityTrends обновляет тренды качества для базы данных
func (db *DB) UpdateQualityTrends(databaseID int) error {
	// Получаем последние метрики для базы данных
	query := `
		SELECT 
			AVG(CASE WHEN metric_category = 'completeness' THEN metric_value END) as completeness,
			AVG(CASE WHEN metric_category = 'consistency' THEN metric_value END) as consistency,
			AVG(CASE WHEN metric_category = 'uniqueness' THEN metric_value END) as uniqueness,
			AVG(CASE WHEN metric_category = 'validity' THEN metric_value END) as validity,
			COUNT(DISTINCT upload_id) as uploads_count
		FROM data_quality_metrics
		WHERE database_id = ?
			AND measured_at >= date('now', '-1 day')
	`

	var completeness, consistency, uniqueness, validity sql.NullFloat64
	var uploadsCount int

	err := db.conn.QueryRow(query, databaseID).Scan(
		&completeness,
		&consistency,
		&uniqueness,
		&validity,
		&uploadsCount,
	)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("failed to get quality metrics: %w", err)
	}

	// Рассчитываем общий балл
	var overallScore float64
	count := 0
	if completeness.Valid {
		overallScore += completeness.Float64
		count++
	}
	if consistency.Valid {
		overallScore += consistency.Float64
		count++
	}
	if uniqueness.Valid {
		overallScore += uniqueness.Float64
		count++
	}
	if validity.Valid {
		overallScore += validity.Float64
		count++
	}
	if count > 0 {
		overallScore = overallScore / float64(count)
	}

	// Получаем количество проблем
	var issuesCount int
	err = db.conn.QueryRow(`
		SELECT COUNT(*) FROM data_quality_issues
		WHERE database_id = ? AND status = 'OPEN'
	`, databaseID).Scan(&issuesCount)
	if err != nil {
		issuesCount = 0
	}

	// Получаем количество проанализированных записей
	var recordsAnalyzed int
	err = db.conn.QueryRow(`
		SELECT COUNT(DISTINCT entity_reference) FROM data_quality_issues
		WHERE database_id = ?
	`, databaseID).Scan(&recordsAnalyzed)
	if err != nil {
		recordsAnalyzed = 0
	}

	// Вставляем или обновляем тренд
	insertQuery := `
		INSERT INTO quality_trends (
			database_id, measurement_date, overall_score,
			completeness_score, consistency_score, uniqueness_score, validity_score,
			records_analyzed, issues_count
		) VALUES (?, date('now'), ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(database_id, measurement_date) DO UPDATE SET
			overall_score = excluded.overall_score,
			completeness_score = excluded.completeness_score,
			consistency_score = excluded.consistency_score,
			uniqueness_score = excluded.uniqueness_score,
			validity_score = excluded.validity_score,
			records_analyzed = excluded.records_analyzed,
			issues_count = excluded.issues_count
	`

	var completenessVal, consistencyVal, uniquenessVal, validityVal interface{}
	if completeness.Valid {
		completenessVal = completeness.Float64
	}
	if consistency.Valid {
		consistencyVal = consistency.Float64
	}
	if uniqueness.Valid {
		uniquenessVal = uniqueness.Float64
	}
	if validity.Valid {
		validityVal = validity.Float64
	}

	_, err = db.conn.Exec(insertQuery,
		databaseID,
		overallScore,
		completenessVal,
		consistencyVal,
		uniquenessVal,
		validityVal,
		recordsAnalyzed,
		issuesCount,
	)

	if err != nil {
		return fmt.Errorf("failed to update quality trends: %w", err)
	}

	return nil
}

// GetQualityTrends получает тренды качества для базы данных
func (db *DB) GetQualityTrends(databaseID int, days int) ([]QualityTrend, error) {
	query := `
		SELECT id, database_id, measurement_date, overall_score,
			completeness_score, consistency_score, uniqueness_score, validity_score,
			records_analyzed, issues_count, created_at
		FROM quality_trends
		WHERE database_id = ?
			AND measurement_date >= date('now', '-' || ? || ' days')
		ORDER BY measurement_date DESC
	`

	rows, err := db.conn.Query(query, databaseID, days)
	if err != nil {
		return nil, fmt.Errorf("failed to query quality trends: %w", err)
	}
	defer rows.Close()

	var trends []QualityTrend
	for rows.Next() {
		var trend QualityTrend
		var completeness, consistency, uniqueness, validity sql.NullFloat64

		err := rows.Scan(
			&trend.ID,
			&trend.DatabaseID,
			&trend.MeasurementDate,
			&trend.OverallScore,
			&completeness,
			&consistency,
			&uniqueness,
			&validity,
			&trend.RecordsAnalyzed,
			&trend.IssuesCount,
			&trend.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan quality trend: %w", err)
		}

		if completeness.Valid {
			trend.CompletenessScore = &completeness.Float64
		}
		if consistency.Valid {
			trend.ConsistencyScore = &consistency.Float64
		}
		if uniqueness.Valid {
			trend.UniquenessScore = &uniqueness.Float64
		}
		if validity.Valid {
			trend.ValidityScore = &validity.Float64
		}

		trends = append(trends, trend)
	}

	return trends, nil
}

// GetCurrentQualityMetrics получает текущие метрики качества для базы данных
func (db *DB) GetCurrentQualityMetrics(databaseID int) ([]DataQualityMetric, error) {
	query := `
		SELECT id, upload_id, database_id, metric_category, metric_name,
			metric_value, threshold_value, status, measured_at, details
		FROM data_quality_metrics
		WHERE database_id = ?
			AND measured_at = (
				SELECT MAX(measured_at)
				FROM data_quality_metrics
				WHERE database_id = ?
			)
		ORDER BY metric_category, metric_name
	`

	rows, err := db.conn.Query(query, databaseID, databaseID)
	if err != nil {
		return nil, fmt.Errorf("failed to query current quality metrics: %w", err)
	}
	defer rows.Close()

	var metrics []DataQualityMetric
	for rows.Next() {
		var metric DataQualityMetric
		var thresholdValue sql.NullFloat64
		var detailsJSON sql.NullString

		err := rows.Scan(
			&metric.ID,
			&metric.UploadID,
			&metric.DatabaseID,
			&metric.MetricCategory,
			&metric.MetricName,
			&metric.MetricValue,
			&thresholdValue,
			&metric.Status,
			&metric.MeasuredAt,
			&detailsJSON,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan quality metric: %w", err)
		}

		if thresholdValue.Valid {
			val := thresholdValue.Float64
			metric.ThresholdValue = &val
		}

		if detailsJSON.Valid && detailsJSON.String != "" {
			if err := json.Unmarshal([]byte(detailsJSON.String), &metric.Details); err != nil {
				metric.Details = make(map[string]interface{})
			}
		}

		metrics = append(metrics, metric)
	}

	return metrics, nil
}

// GetTopQualityIssues получает топ проблем качества для базы данных
func (db *DB) GetTopQualityIssues(databaseID int, limit int) ([]DataQualityIssue, error) {
	query := `
		SELECT id, upload_id, database_id, entity_type, entity_reference,
			issue_type, issue_severity, field_name, expected_value, actual_value,
			description, detected_at, resolved_at, status
		FROM data_quality_issues
		WHERE database_id = ? AND status = 'OPEN'
		ORDER BY 
			CASE issue_severity
				WHEN 'CRITICAL' THEN 1
				WHEN 'HIGH' THEN 2
				WHEN 'MEDIUM' THEN 3
				WHEN 'LOW' THEN 4
			END,
			detected_at DESC
		LIMIT ?
	`

	rows, err := db.conn.Query(query, databaseID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query top quality issues: %w", err)
	}
	defer rows.Close()

	var issues []DataQualityIssue
	for rows.Next() {
		var issue DataQualityIssue
		var resolvedAt sql.NullTime

		err := rows.Scan(
			&issue.ID,
			&issue.UploadID,
			&issue.DatabaseID,
			&issue.EntityType,
			&issue.EntityReference,
			&issue.IssueType,
			&issue.IssueSeverity,
			&issue.FieldName,
			&issue.ExpectedValue,
			&issue.ActualValue,
			&issue.Description,
			&issue.DetectedAt,
			&resolvedAt,
			&issue.Status,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan quality issue: %w", err)
		}

		if resolvedAt.Valid {
			issue.ResolvedAt = &resolvedAt.Time
		}

		issues = append(issues, issue)
	}

	return issues, nil
}

// UpdateUploadQualityScore обновляет общий балл качества для выгрузки
func (db *DB) UpdateUploadQualityScore(uploadID int, score float64) error {
	query := `UPDATE uploads SET quality_score = ? WHERE id = ?`
	_, err := db.conn.Exec(query, score, uploadID)
	if err != nil {
		return fmt.Errorf("failed to update upload quality score: %w", err)
	}
	return nil
}

// DataSnapshot представляет срез данных
type DataSnapshot struct {
	ID           int       `json:"id"`
	Name         string    `json:"name"`
	Description  string    `json:"description"`
	CreatedBy    *int      `json:"created_by,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	SnapshotType string    `json:"snapshot_type"`
	ProjectID    *int      `json:"project_id,omitempty"`
	ClientID     *int      `json:"client_id,omitempty"`
}

// SnapshotUpload представляет связь выгрузки со срезом
type SnapshotUpload struct {
	ID             int    `json:"id"`
	SnapshotID     int    `json:"snapshot_id"`
	UploadID       int    `json:"upload_id"`
	IterationLabel string `json:"iteration_label,omitempty"`
	UploadOrder    int    `json:"upload_order"`
}

// CreateSnapshot создает новый срез и связывает выгрузки
func (db *DB) CreateSnapshot(snapshot *DataSnapshot, uploads []SnapshotUpload) (*DataSnapshot, error) {
	tx, err := db.conn.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Вставляем срез
	result, err := tx.Exec(`
		INSERT INTO data_snapshots (name, description, created_by, snapshot_type, project_id, client_id)
		VALUES (?, ?, ?, ?, ?, ?)
	`, snapshot.Name, snapshot.Description, snapshot.CreatedBy, snapshot.SnapshotType, snapshot.ProjectID, snapshot.ClientID)

	if err != nil {
		return nil, fmt.Errorf("failed to insert snapshot: %w", err)
	}

	snapshotID, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("failed to get snapshot ID: %w", err)
	}

	// Вставляем связи с выгрузками
	for _, upload := range uploads {
		_, err := tx.Exec(`
			INSERT INTO snapshot_uploads (snapshot_id, upload_id, iteration_label, upload_order)
			VALUES (?, ?, ?, ?)
		`, snapshotID, upload.UploadID, upload.IterationLabel, upload.UploadOrder)

		if err != nil {
			return nil, fmt.Errorf("failed to insert snapshot upload: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Получаем созданный срез
	return db.GetSnapshot(int(snapshotID))
}

// GetSnapshot получает срез по ID
func (db *DB) GetSnapshot(id int) (*DataSnapshot, error) {
	var snapshot DataSnapshot
	var createdBy, projectID, clientID sql.NullInt64

	err := db.conn.QueryRow(`
		SELECT id, name, description, created_by, created_at, snapshot_type, project_id, client_id
		FROM data_snapshots WHERE id = ?
	`, id).Scan(
		&snapshot.ID, &snapshot.Name, &snapshot.Description, &createdBy,
		&snapshot.CreatedAt, &snapshot.SnapshotType, &projectID, &clientID,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get snapshot: %w", err)
	}

	if createdBy.Valid {
		val := int(createdBy.Int64)
		snapshot.CreatedBy = &val
	}
	if projectID.Valid {
		val := int(projectID.Int64)
		snapshot.ProjectID = &val
	}
	if clientID.Valid {
		val := int(clientID.Int64)
		snapshot.ClientID = &val
	}

	return &snapshot, nil
}

// GetSnapshotWithUploads возвращает срез и связанные выгрузки
func (db *DB) GetSnapshotWithUploads(id int) (*DataSnapshot, []*Upload, error) {
	snapshot, err := db.GetSnapshot(id)
	if err != nil {
		return nil, nil, err
	}

	// Получаем выгрузки среза
	rows, err := db.conn.Query(`
		SELECT u.id, u.upload_uuid, u.started_at, u.completed_at, u.status,
		       u.version_1c, u.config_name, u.total_constants, u.total_catalogs, u.total_items,
		       u.database_id, u.client_id, u.project_id, u.computer_name, u.user_name, u.config_version,
		       u.iteration_number, u.iteration_label, u.programmer_name, u.upload_purpose, u.parent_upload_id
		FROM uploads u
		JOIN snapshot_uploads su ON u.id = su.upload_id
		WHERE su.snapshot_id = ?
		ORDER BY su.upload_order
	`, id)

	if err != nil {
		return nil, nil, fmt.Errorf("failed to get snapshot uploads: %w", err)
	}
	defer rows.Close()

	var uploads []*Upload
	for rows.Next() {
		upload := &Upload{}
		var databaseID, clientID, projectID, parentUploadID sql.NullInt64
		var completedAt sql.NullTime

		err := rows.Scan(
			&upload.ID, &upload.UploadUUID, &upload.StartedAt, &completedAt, &upload.Status,
			&upload.Version1C, &upload.ConfigName, &upload.TotalConstants, &upload.TotalCatalogs, &upload.TotalItems,
			&databaseID, &clientID, &projectID, &upload.ComputerName, &upload.UserName, &upload.ConfigVersion,
			&upload.IterationNumber, &upload.IterationLabel, &upload.ProgrammerName, &upload.UploadPurpose, &parentUploadID,
		)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to scan upload: %w", err)
		}

		if databaseID.Valid {
			val := int(databaseID.Int64)
			upload.DatabaseID = &val
		}
		if clientID.Valid {
			val := int(clientID.Int64)
			upload.ClientID = &val
		}
		if projectID.Valid {
			val := int(projectID.Int64)
			upload.ProjectID = &val
		}
		if parentUploadID.Valid {
			val := int(parentUploadID.Int64)
			upload.ParentUploadID = &val
		}
		if completedAt.Valid {
			upload.CompletedAt = &completedAt.Time
		}

		uploads = append(uploads, upload)
	}

	if err = rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("error iterating uploads: %w", err)
	}

	return snapshot, uploads, nil
}

// GetSnapshotsByProject получает все срезы проекта
func (db *DB) GetSnapshotsByProject(projectID int) ([]*DataSnapshot, error) {
	rows, err := db.conn.Query(`
		SELECT id, name, description, created_by, created_at, snapshot_type, project_id, client_id
		FROM data_snapshots WHERE project_id = ? ORDER BY created_at DESC
	`, projectID)

	if err != nil {
		return nil, fmt.Errorf("failed to get snapshots: %w", err)
	}
	defer rows.Close()

	var snapshots []*DataSnapshot
	for rows.Next() {
		var snapshot DataSnapshot
		var createdBy, projectIDVal, clientID sql.NullInt64

		err := rows.Scan(
			&snapshot.ID, &snapshot.Name, &snapshot.Description, &createdBy,
			&snapshot.CreatedAt, &snapshot.SnapshotType, &projectIDVal, &clientID,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan snapshot: %w", err)
		}

		if createdBy.Valid {
			val := int(createdBy.Int64)
			snapshot.CreatedBy = &val
		}
		if projectIDVal.Valid {
			val := int(projectIDVal.Int64)
			snapshot.ProjectID = &val
		}
		if clientID.Valid {
			val := int(clientID.Int64)
			snapshot.ClientID = &val
		}

		snapshots = append(snapshots, &snapshot)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating snapshots: %w", err)
	}

	return snapshots, nil
}

// GetLatestUploads получает последние N выгрузок для базы данных
func (db *DB) GetLatestUploads(databaseID int, count int) ([]*Upload, error) {
	rows, err := db.conn.Query(`
		SELECT id, upload_uuid, started_at, completed_at, status,
		       version_1c, config_name, total_constants, total_catalogs, total_items,
		       database_id, client_id, project_id, computer_name, user_name, config_version,
		       iteration_number, iteration_label, programmer_name, upload_purpose, parent_upload_id
		FROM uploads 
		WHERE database_id = ? 
		ORDER BY started_at DESC 
		LIMIT ?
	`, databaseID, count)

	if err != nil {
		return nil, fmt.Errorf("failed to get latest uploads: %w", err)
	}
	defer rows.Close()

	var uploads []*Upload
	for rows.Next() {
		upload := &Upload{}
		var databaseIDVal, clientID, projectID, parentUploadID sql.NullInt64
		var completedAt sql.NullTime

		err := rows.Scan(
			&upload.ID, &upload.UploadUUID, &upload.StartedAt, &completedAt, &upload.Status,
			&upload.Version1C, &upload.ConfigName, &upload.TotalConstants, &upload.TotalCatalogs, &upload.TotalItems,
			&databaseIDVal, &clientID, &projectID, &upload.ComputerName, &upload.UserName, &upload.ConfigVersion,
			&upload.IterationNumber, &upload.IterationLabel, &upload.ProgrammerName, &upload.UploadPurpose, &parentUploadID,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan upload: %w", err)
		}

		if databaseIDVal.Valid {
			val := int(databaseIDVal.Int64)
			upload.DatabaseID = &val
		}
		if clientID.Valid {
			val := int(clientID.Int64)
			upload.ClientID = &val
		}
		if projectID.Valid {
			val := int(projectID.Int64)
			upload.ProjectID = &val
		}
		if parentUploadID.Valid {
			val := int(parentUploadID.Int64)
			upload.ParentUploadID = &val
		}
		if completedAt.Valid {
			upload.CompletedAt = &completedAt.Time
		}

		uploads = append(uploads, upload)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating uploads: %w", err)
	}

	return uploads, nil
}

// UpdateUploadIteration обновляет поля итерации для выгрузки
func (db *DB) UpdateUploadIteration(uploadID int, iterationNumber int, iterationLabel, programmerName, uploadPurpose string) error {
	_, err := db.conn.Exec(`
		UPDATE uploads 
		SET iteration_number = ?, iteration_label = ?, programmer_name = ?, upload_purpose = ?
		WHERE id = ?
	`, iterationNumber, iterationLabel, programmerName, uploadPurpose, uploadID)

	if err != nil {
		return fmt.Errorf("failed to update upload iteration: %w", err)
	}

	return nil
}

// GetAllSnapshots получает все срезы
func (db *DB) GetAllSnapshots() ([]*DataSnapshot, error) {
	rows, err := db.conn.Query(`
		SELECT id, name, description, created_by, created_at, snapshot_type, project_id, client_id
		FROM data_snapshots ORDER BY created_at DESC
	`)

	if err != nil {
		return nil, fmt.Errorf("failed to get snapshots: %w", err)
	}
	defer rows.Close()

	var snapshots []*DataSnapshot
	for rows.Next() {
		var snapshot DataSnapshot
		var createdBy, projectID, clientID sql.NullInt64

		err := rows.Scan(
			&snapshot.ID, &snapshot.Name, &snapshot.Description, &createdBy,
			&snapshot.CreatedAt, &snapshot.SnapshotType, &projectID, &clientID,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan snapshot: %w", err)
		}

		if createdBy.Valid {
			val := int(createdBy.Int64)
			snapshot.CreatedBy = &val
		}
		if projectID.Valid {
			val := int(projectID.Int64)
			snapshot.ProjectID = &val
		}
		if clientID.Valid {
			val := int(clientID.Int64)
			snapshot.ClientID = &val
		}

		snapshots = append(snapshots, &snapshot)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating snapshots: %w", err)
	}

	return snapshots, nil
}

// DeleteSnapshot удаляет срез и все связанные данные (каскадно через FOREIGN KEY)
func (db *DB) DeleteSnapshot(id int) error {
	_, err := db.conn.Exec(`DELETE FROM data_snapshots WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("failed to delete snapshot: %w", err)
	}
	return nil
}

// SaveSnapshotNormalizedData сохраняет нормализованные данные для среза
func (db *DB) SaveSnapshotNormalizedData(snapshotID int, uploadID int, data []interface{}) error {
	// Используем интерфейс для гибкости, но на практике это будет []normalization.NormalizedItem
	tx, err := db.conn.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Удаляем старые данные для этой выгрузки в срезе
	_, err = tx.Exec(`DELETE FROM snapshot_normalized_data WHERE snapshot_id = ? AND upload_id = ?`, snapshotID, uploadID)
	if err != nil {
		return fmt.Errorf("failed to delete old normalized data: %w", err)
	}

	// Вставляем новые данные
	// Предполагаем, что data содержит элементы с методами для получения полей
	// В реальной реализации нужно будет использовать type assertion или рефлексию
	// Для упрощения, будем использовать прямой вызов с нормализованными данными
	stmt, err := tx.Prepare(`
		INSERT INTO snapshot_normalized_data 
		(snapshot_id, upload_id, source_reference, source_name, code, normalized_name, normalized_reference, category, merged_count, source_database_id, source_iteration_number)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	// Обрабатываем данные через рефлексию или type assertion
	// Для упрощения, будем использовать прямой вызов
	// В реальной реализации нужно будет проверить тип данных
	for _, item := range data {
		// Используем type assertion для получения полей
		// Это упрощенная версия, в реальности нужно использовать рефлексию или конкретный тип
		if normItem, ok := item.(map[string]interface{}); ok {
			_, err = stmt.Exec(
				snapshotID, uploadID,
				normItem["source_reference"], normItem["source_name"], normItem["code"],
				normItem["normalized_name"], normItem["normalized_reference"], normItem["category"],
				normItem["merged_count"], normItem["source_database_id"], normItem["source_iteration_number"],
			)
			if err != nil {
				return fmt.Errorf("failed to insert normalized data: %w", err)
			}
		}
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// SaveSnapshotNormalizedDataItems сохраняет нормализованные данные для среза (типизированная версия)
func (db *DB) SaveSnapshotNormalizedDataItems(snapshotID int, uploadID int, data []map[string]interface{}) error {
	tx, err := db.conn.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Удаляем старые данные для этой выгрузки в срезе
	_, err = tx.Exec(`DELETE FROM snapshot_normalized_data WHERE snapshot_id = ? AND upload_id = ?`, snapshotID, uploadID)
	if err != nil {
		return fmt.Errorf("failed to delete old normalized data: %w", err)
	}

	stmt, err := tx.Prepare(`
		INSERT INTO snapshot_normalized_data 
		(snapshot_id, upload_id, source_reference, source_name, code, normalized_name, normalized_reference, category, merged_count, source_database_id, source_iteration_number)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	for _, item := range data {
		sourceRef, _ := item["source_reference"].(string)
		sourceName, _ := item["source_name"].(string)
		code, _ := item["code"].(string)
		normalizedName, _ := item["normalized_name"].(string)
		normalizedRef, _ := item["normalized_reference"].(string)
		category, _ := item["category"].(string)
		mergedCount := 1
		if mc, ok := item["merged_count"].(int); ok {
			mergedCount = mc
		}
		sourceDBID := 0
		if dbid, ok := item["source_database_id"].(int); ok {
			sourceDBID = dbid
		}
		sourceIter := 0
		if iter, ok := item["source_iteration_number"].(int); ok {
			sourceIter = iter
		}

		_, err = stmt.Exec(
			snapshotID, uploadID,
			sourceRef, sourceName, code,
			normalizedName, normalizedRef, category,
			mergedCount, sourceDBID, sourceIter,
		)
		if err != nil {
			return fmt.Errorf("failed to insert normalized data: %w", err)
		}
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// GetSnapshotNormalizedData получает нормализованные данные для среза
func (db *DB) GetSnapshotNormalizedData(snapshotID int) ([]map[string]interface{}, error) {
	rows, err := db.conn.Query(`
		SELECT source_reference, source_name, code, normalized_name, normalized_reference, category, merged_count, source_database_id, source_iteration_number
		FROM snapshot_normalized_data
		WHERE snapshot_id = ?
		ORDER BY normalized_name
	`, snapshotID)
	if err != nil {
		return nil, fmt.Errorf("failed to get snapshot normalized data: %w", err)
	}
	defer rows.Close()

	var items []map[string]interface{}
	for rows.Next() {
		var uploadID sql.NullInt64
		var sourceRef, sourceName, code, normalizedName, normalizedRef, category sql.NullString
		var mergedCount, sourceDBID, sourceIter sql.NullInt64

		err := rows.Scan(&uploadID, &sourceRef, &sourceName, &code, &normalizedName, &normalizedRef, &category, &mergedCount, &sourceDBID, &sourceIter)
		if err != nil {
			return nil, fmt.Errorf("failed to scan normalized data: %w", err)
		}

		item := make(map[string]interface{})
		if uploadID.Valid {
			item["upload_id"] = int(uploadID.Int64)
		}
		if sourceRef.Valid {
			item["source_reference"] = sourceRef.String
		}
		if sourceName.Valid {
			item["source_name"] = sourceName.String
		}
		if code.Valid {
			item["code"] = code.String
		}
		if normalizedName.Valid {
			item["normalized_name"] = normalizedName.String
		}
		if normalizedRef.Valid {
			item["normalized_reference"] = normalizedRef.String
		}
		if category.Valid {
			item["category"] = category.String
		}
		if mergedCount.Valid {
			item["merged_count"] = int(mergedCount.Int64)
		}
		if sourceDBID.Valid {
			item["source_database_id"] = int(sourceDBID.Int64)
		}
		if sourceIter.Valid {
			item["source_iteration_number"] = int(sourceIter.Int64)
		}

		items = append(items, item)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating normalized data: %w", err)
	}

	return items, nil
}

// GetNormalizedItemsBySimilarNames получает нормализованные записи с похожими именами
// Используется для проверки дубликатов перед вставкой
// Возвращает записи, у которых normalized_name похоже на одно из переданных имен
func (db *DB) GetNormalizedItemsBySimilarNames(names []string) ([]*NormalizedItem, error) {
	if len(names) == 0 {
		return []*NormalizedItem{}, nil
	}

	// Строим WHERE условие с OR для всех имен
	// Используем LIKE с wildcards для нечеткого поиска
	query := `
		SELECT id, source_reference, source_name, code, normalized_name,
		       normalized_reference, category, merged_count, ai_confidence,
		       ai_reasoning, processing_level, kpved_code, kpved_name, kpved_confidence,
		       quality_score, created_at
		FROM normalized_data
		WHERE `

	conditions := make([]string, len(names))
	args := make([]interface{}, len(names))

	for i, name := range names {
		conditions[i] = "normalized_name LIKE ?"
		// Добавляем wildcards для нечеткого поиска
		args[i] = "%" + name + "%"
	}

	query += "(" + joinStrings(conditions, " OR ") + ")"

	rows, err := db.conn.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query normalized items: %w", err)
	}
	defer rows.Close()

	var items []*NormalizedItem
	for rows.Next() {
		item := &NormalizedItem{}
		var createdAt string

		err := rows.Scan(
			&item.ID,
			&item.SourceReference,
			&item.SourceName,
			&item.Code,
			&item.NormalizedName,
			&item.NormalizedReference,
			&item.Category,
			&item.MergedCount,
			&item.AIConfidence,
			&item.AIReasoning,
			&item.ProcessingLevel,
			&item.KpvedCode,
			&item.KpvedName,
			&item.KpvedConfidence,
			&item.QualityScore,
			&createdAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan normalized item: %w", err)
		}

		// Парсим created_at
		if createdAt != "" {
			t, err := time.Parse("2006-01-02 15:04:05", createdAt)
			if err == nil {
				item.CreatedAt = t
			}
		}

		items = append(items, item)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating normalized items: %w", err)
	}

	return items, nil
}

// IncrementMergedCount увеличивает счетчик объединенных записей
// Используется при обнаружении дубликата - вместо вставки новой записи увеличиваем счетчик
func (db *DB) IncrementMergedCount(id int) error {
	query := `
		UPDATE normalized_data
		SET merged_count = merged_count + 1
		WHERE id = ?
	`

	_, err := db.conn.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to increment merged_count: %w", err)
	}

	return nil
}

// PerformanceMetricsSnapshot представляет снимок метрик производительности для сохранения в истории
type PerformanceMetricsSnapshot struct {
	ID                  int       `json:"id"`
	Timestamp           time.Time `json:"timestamp"`
	MetricType          string    `json:"metric_type"`
	MetricData          string    `json:"metric_data"`           // JSON со всеми метриками
	UptimeSeconds       int       `json:"uptime_seconds"`
	Throughput          float64   `json:"throughput"`
	AISuccessRate       float64   `json:"ai_success_rate"`
	CacheHitRate        float64   `json:"cache_hit_rate"`
	BatchQueueSize      int       `json:"batch_queue_size"`
	CircuitBreakerState string    `json:"circuit_breaker_state"`
	CheckpointProgress  float64   `json:"checkpoint_progress"`
}

// SaveMetrics сохраняет снимок метрик производительности в историю
func (db *DB) SaveMetrics(snapshot *PerformanceMetricsSnapshot) error {
	query := `
		INSERT INTO performance_metrics_history (
			timestamp, metric_type, metric_data, uptime_seconds, throughput,
			ai_success_rate, cache_hit_rate, batch_queue_size,
			circuit_breaker_state, checkpoint_progress
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	result, err := db.conn.Exec(
		query,
		snapshot.Timestamp,
		snapshot.MetricType,
		snapshot.MetricData,
		snapshot.UptimeSeconds,
		snapshot.Throughput,
		snapshot.AISuccessRate,
		snapshot.CacheHitRate,
		snapshot.BatchQueueSize,
		snapshot.CircuitBreakerState,
		snapshot.CheckpointProgress,
	)
	if err != nil {
		return fmt.Errorf("failed to save metrics snapshot: %w", err)
	}

	id, err := result.LastInsertId()
	if err == nil {
		snapshot.ID = int(id)
	}

	return nil
}

// GetMetricsHistory возвращает историю метрик за указанный период
// from, to - временные границы (если nil, используются значения по умолчанию)
// metricType - фильтр по типу метрики (если пустая строка, возвращаются все типы)
// limit - максимальное количество записей (0 = без ограничений)
func (db *DB) GetMetricsHistory(from, to *time.Time, metricType string, limit int) ([]PerformanceMetricsSnapshot, error) {
	query := `
		SELECT
			id, timestamp, metric_type, metric_data, uptime_seconds, throughput,
			ai_success_rate, cache_hit_rate, batch_queue_size,
			circuit_breaker_state, checkpoint_progress
		FROM performance_metrics_history
		WHERE 1=1
	`
	args := make([]interface{}, 0)

	// Фильтр по времени начала
	if from != nil {
		query += " AND timestamp >= ?"
		args = append(args, *from)
	}

	// Фильтр по времени окончания
	if to != nil {
		query += " AND timestamp <= ?"
		args = append(args, *to)
	}

	// Фильтр по типу метрики
	if metricType != "" {
		query += " AND metric_type = ?"
		args = append(args, metricType)
	}

	// Сортировка по времени
	query += " ORDER BY timestamp DESC"

	// Ограничение количества записей
	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}

	rows, err := db.conn.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query metrics history: %w", err)
	}
	defer rows.Close()

	var snapshots []PerformanceMetricsSnapshot
	for rows.Next() {
		var snapshot PerformanceMetricsSnapshot
		var timestampStr string

		err := rows.Scan(
			&snapshot.ID,
			&timestampStr,
			&snapshot.MetricType,
			&snapshot.MetricData,
			&snapshot.UptimeSeconds,
			&snapshot.Throughput,
			&snapshot.AISuccessRate,
			&snapshot.CacheHitRate,
			&snapshot.BatchQueueSize,
			&snapshot.CircuitBreakerState,
			&snapshot.CheckpointProgress,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan metrics snapshot: %w", err)
		}

		// Парсим timestamp
		if t, err := time.Parse("2006-01-02 15:04:05", timestampStr); err == nil {
			snapshot.Timestamp = t
		}

		snapshots = append(snapshots, snapshot)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating metrics snapshots: %w", err)
	}

	return snapshots, nil
}

// CleanOldMetrics удаляет старые метрики из истории (retention policy)
// Сохраняет только последние N дней
func (db *DB) CleanOldMetrics(retentionDays int) error {
	query := `
		DELETE FROM performance_metrics_history
		WHERE timestamp < datetime('now', '-' || ? || ' days')
	`

	result, err := db.conn.Exec(query, retentionDays)
	if err != nil {
		return fmt.Errorf("failed to clean old metrics: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected > 0 {
		// Можно добавить логирование
		_ = rowsAffected
	}

	return nil
}
