package server

import (
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"httpserver/database"
)

// DatabaseInfo информация о БД для ответа
type DatabaseInfo struct {
	XMLName         xml.Name `xml:"database"`
	FileName        string   `xml:"file_name"`
	UploadUUID      string   `xml:"upload_uuid"`
	ConfigName      string   `xml:"config_name"`
	StartedAt       string   `xml:"started_at"`
	TotalCatalogs   int      `xml:"total_catalogs"`
	TotalConstants  int      `xml:"total_constants"`
	TotalItems      int      `xml:"total_items"`
	DatabaseID      int      `xml:"database_id,omitempty"`
	ClientID        int      `xml:"client_id,omitempty"`
	ProjectID       int      `xml:"project_id,omitempty"`
	ComputerName    string   `xml:"computer_name,omitempty"`
	UserName        string   `xml:"user_name,omitempty"`
	ConfigVersion   string   `xml:"config_version,omitempty"`
}

// DatabasesListResponse ответ со списком БД
type DatabasesListResponse struct {
	XMLName   xml.Name       `xml:"databases"`
	Databases []DatabaseInfo `xml:"database"`
	Total     int            `xml:"total"`
}

// ImportHandshakeRequest запрос на начало импорта
type ImportHandshakeRequest struct {
	XMLName    xml.Name   `xml:"import_handshake"`
	DBName     string     `xml:"db_name"`
	ClientInfo ClientInfo `xml:"client_info"`
}

// ClientInfo информация о клиенте
type ClientInfo struct {
	Version1C    string `xml:"version_1c"`
	ComputerName string `xml:"computer_name"`
	UserName     string `xml:"user_name"`
}

// ImportHandshakeResponse ответ на handshake для импорта
type ImportHandshakeResponse struct {
	XMLName        xml.Name                  `xml:"import_handshake_response"`
	Success        bool                      `xml:"success"`
	UploadUUID     string                    `xml:"upload_uuid"`
	Catalogs       []ImportCatalogInfo       `xml:"catalogs>catalog"`
	ConstantsCount int                       `xml:"constants_count"`
	Message        string                    `xml:"message,omitempty"`
	Timestamp      string                    `xml:"timestamp"`
}

// ImportCatalogInfo информация о справочнике для импорта
type ImportCatalogInfo struct {
	Name      string `xml:"name"`
	Synonym   string `xml:"synonym"`
	ItemCount int    `xml:"item_count"`
}

// ImportGetConstantsRequest запрос на получение констант
type ImportGetConstantsRequest struct {
	XMLName xml.Name `xml:"import_get_constants"`
	DBName  string   `xml:"db_name"`
	Offset  int      `xml:"offset,omitempty"`
	Limit   int      `xml:"limit,omitempty"`
}

// ImportGetConstantsResponse ответ с константами
type ImportGetConstantsResponse struct {
	XMLName   xml.Name             `xml:"import_constants_response"`
	Success   bool                 `xml:"success"`
	Constants []ConstantForImport  `xml:"constants>constant"`
	Total     int                  `xml:"total"`
	Offset    int                  `xml:"offset"`
	Limit     int                  `xml:"limit"`
	Message   string               `xml:"message,omitempty"`
}

// ConstantForImport константа для импорта
type ConstantForImport struct {
	Name      string `xml:"name"`
	Synonym   string `xml:"synonym"`
	Type      string `xml:"type"`
	Value     string `xml:"value"`
	CreatedAt string `xml:"created_at"`
}

// ImportGetCatalogRequest запрос на получение справочника
type ImportGetCatalogRequest struct {
	XMLName     xml.Name `xml:"import_get_catalog"`
	DBName      string   `xml:"db_name"`
	CatalogName string   `xml:"catalog_name"`
	Offset      int      `xml:"offset,omitempty"`
	Limit       int      `xml:"limit,omitempty"`
}

// ImportGetCatalogResponse ответ с элементами справочника
type ImportGetCatalogResponse struct {
	XMLName     xml.Name              `xml:"import_catalog_response"`
	Success     bool                  `xml:"success"`
	CatalogName string                `xml:"catalog_name"`
	Items       []CatalogItemForImport `xml:"items>item"`
	Total       int                   `xml:"total"`
	Offset      int                   `xml:"offset"`
	Limit       int                   `xml:"limit"`
	Message     string                `xml:"message,omitempty"`
}

// CatalogItemForImport элемент справочника для импорта
type CatalogItemForImport struct {
	Reference      string `xml:"reference"`
	Code           string `xml:"code"`
	Name           string `xml:"name"`
	AttributesXML  string `xml:"attributes_xml"`
	TablePartsXML  string `xml:"table_parts_xml"`
	CreatedAt      string `xml:"created_at"`
}

// ImportCompleteRequest запрос на завершение импорта
type ImportCompleteRequest struct {
	XMLName xml.Name `xml:"import_complete"`
	DBName  string   `xml:"db_name"`
}

// ImportCompleteResponse ответ на завершение импорта
type ImportCompleteResponse struct {
	XMLName   xml.Name `xml:"import_complete_response"`
	Success   bool     `xml:"success"`
	Message   string   `xml:"message"`
	Timestamp string   `xml:"timestamp"`
}

// handle1CDatabasesList обрабатывает GET /api/1c/databases
func (s *Server) handle1CDatabasesList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.log(LogEntry{
		Timestamp: time.Now(),
		Level:     "INFO",
		Message:   "Getting databases list for 1C import",
		Endpoint:  "/api/1c/databases",
	})

	// Получаем список всех .db файлов
	databases, err := s.getDatabaseList()
	if err != nil {
		s.log(LogEntry{
			Timestamp: time.Now(),
			Level:     "ERROR",
			Message:   fmt.Sprintf("Failed to get databases list: %v", err),
			Endpoint:  "/api/1c/databases",
		})
		http.Error(w, fmt.Sprintf("Failed to get databases list: %v", err), http.StatusInternalServerError)
		return
	}

	response := DatabasesListResponse{
		Databases: databases,
		Total:     len(databases),
	}

	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	
	xmlData, err := xml.MarshalIndent(response, "", "  ")
	if err != nil {
		s.log(LogEntry{
			Timestamp: time.Now(),
			Level:     "ERROR",
			Message:   fmt.Sprintf("Failed to marshal XML: %v", err),
			Endpoint:  "/api/1c/databases",
		})
		http.Error(w, "Failed to create XML response", http.StatusInternalServerError)
		return
	}

	w.Write([]byte(xml.Header))
	w.Write(xmlData)

	s.log(LogEntry{
		Timestamp: time.Now(),
		Level:     "INFO",
		Message:   fmt.Sprintf("Returned %d databases for 1C import", len(databases)),
		Endpoint:  "/api/1c/databases",
	})
}

// handle1CImportHandshake обрабатывает POST /api/1c/import/handshake
func (s *Server) handle1CImportHandshake(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.writeErrorResponse(w, "Failed to read request body", err)
		return
	}

	var req ImportHandshakeRequest
	if err := xml.Unmarshal(body, &req); err != nil {
		s.writeErrorResponse(w, "Failed to parse XML", err)
		return
	}

	s.log(LogEntry{
		Timestamp: time.Now(),
		Level:     "INFO",
		Message:   fmt.Sprintf("Starting import from database: %s", req.DBName),
		Endpoint:  "/api/1c/import/handshake",
	})

	// Открываем БД по имени файла
	db, uploadUUID, err := s.openDatabaseByName(req.DBName)
	if err != nil {
		s.log(LogEntry{
			Timestamp: time.Now(),
			Level:     "ERROR",
			Message:   fmt.Sprintf("Failed to open database %s: %v", req.DBName, err),
			Endpoint:  "/api/1c/import/handshake",
		})
		response := ImportHandshakeResponse{
			Success:   false,
			Message:   fmt.Sprintf("Failed to open database: %v", err),
			Timestamp: time.Now().Format(time.RFC3339),
		}
		s.writeXMLResponse(w, response)
		return
	}

	// Получаем информацию о БД (справочники и константы)
	catalogs, err := db.GetAllCatalogs()
	if err != nil {
		s.log(LogEntry{
			Timestamp: time.Now(),
			Level:     "ERROR",
			Message:   fmt.Sprintf("Failed to get catalogs: %v", err),
			Endpoint:  "/api/1c/import/handshake",
		})
		response := ImportHandshakeResponse{
			Success:   false,
			Message:   fmt.Sprintf("Failed to get catalogs: %v", err),
			Timestamp: time.Now().Format(time.RFC3339),
		}
		s.writeXMLResponse(w, response)
		return
	}

	// Получаем количество констант
	constantsCount := 0
	upload, err := db.GetUploadByUUID(uploadUUID)
	if err == nil {
		constantsCount = upload.TotalConstants
	}

	// Формируем список справочников с количеством элементов
	var catalogInfos []ImportCatalogInfo
	for _, catalog := range catalogs {
		itemCount, err := db.GetCatalogItemsCount(catalog.ID)
		if err != nil {
			itemCount = 0
		}
		catalogInfos = append(catalogInfos, ImportCatalogInfo{
			Name:      catalog.Name,
			Synonym:   catalog.Synonym,
			ItemCount: itemCount,
		})
	}

	response := ImportHandshakeResponse{
		Success:        true,
		UploadUUID:     uploadUUID,
		Catalogs:       catalogInfos,
		ConstantsCount: constantsCount,
		Message:        "Import handshake successful",
		Timestamp:      time.Now().Format(time.RFC3339),
	}

	s.log(LogEntry{
		Timestamp:  time.Now(),
		Level:      "INFO",
		Message:    fmt.Sprintf("Import handshake successful for database %s, uuid=%s", req.DBName, uploadUUID),
		UploadUUID: uploadUUID,
		Endpoint:   "/api/1c/import/handshake",
	})

	s.writeXMLResponse(w, response)
}

// handle1CImportGetConstants обрабатывает POST /api/1c/import/get-constants
func (s *Server) handle1CImportGetConstants(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.writeErrorResponse(w, "Failed to read request body", err)
		return
	}

	var req ImportGetConstantsRequest
	if err := xml.Unmarshal(body, &req); err != nil {
		s.writeErrorResponse(w, "Failed to parse XML", err)
		return
	}

	// Устанавливаем значения по умолчанию для пагинации
	if req.Limit <= 0 {
		req.Limit = 1000
	}
	if req.Limit > 10000 {
		req.Limit = 10000
	}

	s.log(LogEntry{
		Timestamp: time.Now(),
		Level:     "INFO",
		Message:   fmt.Sprintf("Getting constants from database: %s (offset=%d, limit=%d)", req.DBName, req.Offset, req.Limit),
		Endpoint:  "/api/1c/import/get-constants",
	})

	// Открываем БД
	db, uploadUUID, err := s.openDatabaseByName(req.DBName)
	if err != nil {
		response := ImportGetConstantsResponse{
			Success: false,
			Message: fmt.Sprintf("Failed to open database: %v", err),
		}
		s.writeXMLResponse(w, response)
		return
	}

	// Получаем выгрузку
	upload, err := db.GetUploadByUUID(uploadUUID)
	if err != nil {
		response := ImportGetConstantsResponse{
			Success: false,
			Message: fmt.Sprintf("Failed to get upload: %v", err),
		}
		s.writeXMLResponse(w, response)
		return
	}

	// Получаем константы с пагинацией
	constants, err := db.GetConstantsByUploadWithPagination(upload.ID, req.Limit, req.Offset)
	if err != nil {
		response := ImportGetConstantsResponse{
			Success: false,
			Message: fmt.Sprintf("Failed to get constants: %v", err),
		}
		s.writeXMLResponse(w, response)
		return
	}

	// Форматируем константы для ответа
	var constantsForImport []ConstantForImport
	for _, c := range constants {
		constantsForImport = append(constantsForImport, ConstantForImport{
			Name:      c.Name,
			Synonym:   c.Synonym,
			Type:      c.Type,
			Value:     c.Value,
			CreatedAt: c.CreatedAt.Format(time.RFC3339),
		})
	}

	response := ImportGetConstantsResponse{
		Success:   true,
		Constants: constantsForImport,
		Total:     upload.TotalConstants,
		Offset:    req.Offset,
		Limit:     req.Limit,
	}

	s.log(LogEntry{
		Timestamp:  time.Now(),
		Level:      "INFO",
		Message:    fmt.Sprintf("Returned %d constants from database %s", len(constantsForImport), req.DBName),
		UploadUUID: uploadUUID,
		Endpoint:   "/api/1c/import/get-constants",
	})

	s.writeXMLResponse(w, response)
}

// handle1CImportGetCatalog обрабатывает POST /api/1c/import/get-catalog
func (s *Server) handle1CImportGetCatalog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.writeErrorResponse(w, "Failed to read request body", err)
		return
	}

	var req ImportGetCatalogRequest
	if err := xml.Unmarshal(body, &req); err != nil {
		s.writeErrorResponse(w, "Failed to parse XML", err)
		return
	}

	// Устанавливаем значения по умолчанию для пагинации
	if req.Limit <= 0 {
		req.Limit = 500
	}
	if req.Limit > 10000 {
		req.Limit = 10000
	}

	s.log(LogEntry{
		Timestamp: time.Now(),
		Level:     "INFO",
		Message:   fmt.Sprintf("Getting catalog '%s' from database: %s (offset=%d, limit=%d)", req.CatalogName, req.DBName, req.Offset, req.Limit),
		Endpoint:  "/api/1c/import/get-catalog",
	})

	// Открываем БД
	db, uploadUUID, err := s.openDatabaseByName(req.DBName)
	if err != nil {
		response := ImportGetCatalogResponse{
			Success: false,
			Message: fmt.Sprintf("Failed to open database: %v", err),
		}
		s.writeXMLResponse(w, response)
		return
	}

	// Получаем выгрузку
	upload, err := db.GetUploadByUUID(uploadUUID)
	if err != nil {
		response := ImportGetCatalogResponse{
			Success: false,
			Message: fmt.Sprintf("Failed to get upload: %v", err),
		}
		s.writeXMLResponse(w, response)
		return
	}

	// Находим справочник по имени
	catalog, err := db.GetCatalogByNameAndUpload(req.CatalogName, upload.ID)
	if err != nil {
		response := ImportGetCatalogResponse{
			Success:     false,
			CatalogName: req.CatalogName,
			Message:     fmt.Sprintf("Catalog not found: %v", err),
		}
		s.writeXMLResponse(w, response)
		return
	}

	// Получаем общее количество элементов
	totalItems, err := db.GetCatalogItemsCount(catalog.ID)
	if err != nil {
		totalItems = 0
	}

	// Получаем элементы справочника с пагинацией
	items, err := db.GetCatalogItemsWithPagination(catalog.ID, req.Limit, req.Offset)
	if err != nil {
		response := ImportGetCatalogResponse{
			Success:     false,
			CatalogName: req.CatalogName,
			Message:     fmt.Sprintf("Failed to get catalog items: %v", err),
		}
		s.writeXMLResponse(w, response)
		return
	}

	// Форматируем элементы для ответа
	var itemsForImport []CatalogItemForImport
	for _, item := range items {
		itemsForImport = append(itemsForImport, CatalogItemForImport{
			Reference:     item.Reference,
			Code:          item.Code,
			Name:          item.Name,
			AttributesXML: item.Attributes,
			TablePartsXML: item.TableParts,
			CreatedAt:     item.CreatedAt.Format(time.RFC3339),
		})
	}

	response := ImportGetCatalogResponse{
		Success:     true,
		CatalogName: req.CatalogName,
		Items:       itemsForImport,
		Total:       totalItems,
		Offset:      req.Offset,
		Limit:       req.Limit,
	}

	s.log(LogEntry{
		Timestamp:  time.Now(),
		Level:      "INFO",
		Message:    fmt.Sprintf("Returned %d items of catalog '%s' from database %s", len(itemsForImport), req.CatalogName, req.DBName),
		UploadUUID: uploadUUID,
		Endpoint:   "/api/1c/import/get-catalog",
	})

	s.writeXMLResponse(w, response)
}

// handle1CImportComplete обрабатывает POST /api/1c/import/complete
func (s *Server) handle1CImportComplete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.writeErrorResponse(w, "Failed to read request body", err)
		return
	}

	var req ImportCompleteRequest
	if err := xml.Unmarshal(body, &req); err != nil {
		s.writeErrorResponse(w, "Failed to parse XML", err)
		return
	}

	s.log(LogEntry{
		Timestamp: time.Now(),
		Level:     "INFO",
		Message:   fmt.Sprintf("Import completed for database: %s", req.DBName),
		Endpoint:  "/api/1c/import/complete",
	})

	response := ImportCompleteResponse{
		Success:   true,
		Message:   "Import completed successfully",
		Timestamp: time.Now().Format(time.RFC3339),
	}

	s.writeXMLResponse(w, response)
}

// getDatabaseList получает список всех БД файлов с информацией
func (s *Server) getDatabaseList() ([]DatabaseInfo, error) {
	var databases []DatabaseInfo

	// Сканируем несколько директорий на наличие .db файлов
	var allFiles []string

	// 1. Текущая директория
	files, err := filepath.Glob("*.db")
	if err == nil {
		allFiles = append(allFiles, files...)
	}

	// 2. Директория /app/data (для Docker)
	dataFiles, err := filepath.Glob("data/*.db")
	if err == nil {
		for _, file := range dataFiles {
			allFiles = append(allFiles, file)
		}
	}

	// 3. Директория /app/data (абсолютный путь для Docker)
	absDataFiles, err := filepath.Glob("/app/data/*.db")
	if err == nil {
		for _, file := range absDataFiles {
			allFiles = append(allFiles, file)
		}
	}

	// Убираем дубликаты и служебные БД
	fileMap := make(map[string]string)
	uniqueFiles := []string{}
	for _, file := range allFiles {
		absPath, err := filepath.Abs(file)
		if err != nil {
			absPath = file
		}
		
		baseName := filepath.Base(file)
		// Пропускаем service.db и другие служебные БД
		if baseName == "service.db" || baseName == "data.db" || baseName == "normalized_data.db" {
			continue
		}
		
		if _, exists := fileMap[absPath]; !exists {
			fileMap[absPath] = file
			uniqueFiles = append(uniqueFiles, file)
		}
	}

	// Получаем информацию о каждой БД
	for _, dbPath := range uniqueFiles {
		info, err := s.getDatabaseInfo(dbPath)
		if err != nil {
			log.Printf("Warning: Failed to get info for database %s: %v", dbPath, err)
			continue
		}
		databases = append(databases, info)
	}

	return databases, nil
}

// getDatabaseInfo получает информацию о БД
func (s *Server) getDatabaseInfo(dbPath string) (DatabaseInfo, error) {
	info := DatabaseInfo{
		FileName: filepath.Base(dbPath),
	}

	// Открываем БД для чтения метаданных
	dbConfig := database.DBConfig{
		MaxOpenConns:    10,
		MaxIdleConns:    2,
		ConnMaxLifetime: 5 * time.Minute,
	}

	db, err := database.NewDBWithConfig(dbPath, dbConfig)
	if err != nil {
		return info, fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	// Получаем последнюю выгрузку из этой БД
	uploads, err := db.GetAllUploads()
	if err != nil || len(uploads) == 0 {
		return info, fmt.Errorf("no uploads found in database")
	}

	// Берем первую (последнюю) выгрузку
	upload := uploads[0]
	
	info.UploadUUID = upload.UploadUUID
	info.ConfigName = upload.ConfigName
	info.StartedAt = upload.StartedAt.Format(time.RFC3339)
	info.TotalCatalogs = upload.TotalCatalogs
	info.TotalConstants = upload.TotalConstants
	info.TotalItems = upload.TotalItems
	
	if upload.DatabaseID != nil {
		info.DatabaseID = *upload.DatabaseID
	}
	if upload.ClientID != nil {
		info.ClientID = *upload.ClientID
	}
	if upload.ProjectID != nil {
		info.ProjectID = *upload.ProjectID
	}
	
	info.ComputerName = upload.ComputerName
	info.UserName = upload.UserName
	info.ConfigVersion = upload.ConfigVersion

	return info, nil
}

// openDatabaseByName открывает БД по имени файла и возвращает upload_uuid
func (s *Server) openDatabaseByName(dbName string) (*database.DB, string, error) {
	// Ищем БД файл в разных директориях
	possiblePaths := []string{
		dbName,
		filepath.Join("data", dbName),
		filepath.Join("/app/data", dbName),
		filepath.Join("/app", dbName),
	}

	var dbPath string
	for _, path := range possiblePaths {
		if _, err := os.Stat(path); err == nil {
			dbPath = path
			break
		}
	}

	if dbPath == "" {
		return nil, "", fmt.Errorf("database file not found: %s", dbName)
	}

	// Проверяем кэш uploadDBs - может БД уже открыта
	// Ищем по имени файла
	s.uploadDBsMutex.RLock()
	for uuid, cachedDB := range s.uploadDBs {
		// Проверяем что это та же БД (по пути или имени файла)
		// Для упрощения просто открываем новое подключение
		_ = uuid
		_ = cachedDB
	}
	s.uploadDBsMutex.RUnlock()

	// Открываем БД
	dbConfig := database.DBConfig{
		MaxOpenConns:    s.config.MaxOpenConns,
		MaxIdleConns:    s.config.MaxIdleConns,
		ConnMaxLifetime: s.config.ConnMaxLifetime,
	}

	db, err := database.NewDBWithConfig(dbPath, dbConfig)
	if err != nil {
		return nil, "", fmt.Errorf("failed to open database: %w", err)
	}

	// Получаем последнюю выгрузку для получения UUID
	uploads, err := db.GetAllUploads()
	if err != nil || len(uploads) == 0 {
		db.Close()
		return nil, "", fmt.Errorf("no uploads found in database")
	}

	uploadUUID := uploads[0].UploadUUID

	// Добавляем в кэш
	s.uploadDBsMutex.Lock()
	s.uploadDBs[uploadUUID] = db
	s.uploadDBsMutex.Unlock()

	return db, uploadUUID, nil
}

