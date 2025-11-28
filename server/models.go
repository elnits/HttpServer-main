package server

import (
	"encoding/xml"
	"io"
	"strings"
	"time"

	"httpserver/database"
)

// HandshakeRequest запрос рукопожатия
type HandshakeRequest struct {
	XMLName         xml.Name `xml:"handshake"`
	DatabaseID      string   `xml:"database_id,omitempty"`
	Version1C       string   `xml:"version_1c"`
	ConfigName      string   `xml:"config_name"`
	ConfigVersion   string   `xml:"config_version,omitempty"`
	ComputerName    string   `xml:"computer_name,omitempty"`
	UserName        string   `xml:"user_name,omitempty"`
	Timestamp       string   `xml:"timestamp"`
	UploadType      string   `xml:"upload_type,omitempty"` // Тип выгружаемых данных (Номенклатура, Контрагенты, ПолнаяВыгрузка и т.д.)
	// Поля для итераций
	IterationNumber int    `xml:"iteration_number,omitempty"`
	IterationLabel  string `xml:"iteration_label,omitempty"`
	ProgrammerName  string `xml:"programmer_name,omitempty"`
	UploadPurpose   string `xml:"upload_purpose,omitempty"`
	ParentUploadID  string `xml:"parent_upload_id,omitempty"` // UUID родительской выгрузки
}

// HandshakeResponse ответ на рукопожатие
type HandshakeResponse struct {
	XMLName      xml.Name `xml:"handshake_response"`
	Success      bool     `xml:"success"`
	UploadUUID   string   `xml:"upload_uuid"`
	ClientName   string   `xml:"client_name,omitempty"`
	ProjectName  string   `xml:"project_name,omitempty"`
	DatabaseName string   `xml:"database_name,omitempty"`
	DatabasePath string   `xml:"database_path,omitempty"` // Путь к созданной БД
	DatabaseID   int      `xml:"database_id,omitempty"`   // ID в service.db (если зарегистрирована)
	Message      string   `xml:"message"`
	Timestamp    string   `xml:"timestamp"`
}

// MetadataRequest запрос метаинформации
type MetadataRequest struct {
	XMLName       xml.Name `xml:"metadata"`
	UploadUUID    string   `xml:"upload_uuid"`
	DatabaseID    string   `xml:"database_id,omitempty"`
	Version1C     string   `xml:"version_1c"`
	ConfigName    string   `xml:"config_name"`
	ConfigVersion string   `xml:"config_version,omitempty"`
	ComputerName  string   `xml:"computer_name,omitempty"`
	UserName      string   `xml:"user_name,omitempty"`
	Timestamp     string   `xml:"timestamp"`
}

// MetadataResponse ответ на метаинформацию
type MetadataResponse struct {
	XMLName     xml.Name `xml:"metadata_response"`
	Success     bool     `xml:"success"`
	Message     string   `xml:"message"`
	Timestamp   string   `xml:"timestamp"`
}

// ConstantValue обертка для значения константы с поддержкой вложенного XML
type ConstantValue struct {
	Content string
}

// UnmarshalXML кастомный парсер для получения всего содержимого тега value
func (cv *ConstantValue) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	var content strings.Builder
	depth := 0
	
	for {
		token, err := d.Token()
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		
		switch t := token.(type) {
		case xml.StartElement:
			depth++
			// Записываем открывающий тег
			content.WriteString("<" + t.Name.Local)
			for _, attr := range t.Attr {
				escaped := strings.ReplaceAll(attr.Value, "&", "&amp;")
				escaped = strings.ReplaceAll(escaped, "<", "&lt;")
				escaped = strings.ReplaceAll(escaped, ">", "&gt;")
				escaped = strings.ReplaceAll(escaped, "\"", "&quot;")
				content.WriteString(" " + attr.Name.Local + "=\"" + escaped + "\"")
			}
			content.WriteString(">")
		case xml.EndElement:
			if t.Name == start.Name && depth == 0 {
				// Это закрывающий тег value - завершаем
				cv.Content = content.String()
				return nil
			}
			depth--
			content.WriteString("</" + t.Name.Local + ">")
		case xml.CharData:
			// Записываем текстовое содержимое (уже экранировано парсером)
			content.Write(t)
		}
	}
	
	cv.Content = content.String()
	return nil
}

// XMLContent обертка для XML содержимого (для attributes_xml и table_parts)
type XMLContent struct {
	Content string
}

// UnmarshalXML кастомный парсер для получения всего содержимого XML тега
func (xc *XMLContent) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	var content strings.Builder
	depth := 0
	
	for {
		token, err := d.Token()
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		
		switch t := token.(type) {
		case xml.StartElement:
			depth++
			// Записываем открывающий тег
			content.WriteString("<" + t.Name.Local)
			for _, attr := range t.Attr {
				// Атрибуты уже экранированы в XML, просто записываем их
				content.WriteString(" " + attr.Name.Local + "=\"" + attr.Value + "\"")
			}
			content.WriteString(">")
		case xml.EndElement:
			if t.Name == start.Name && depth == 0 {
				// Это закрывающий тег - завершаем
				xc.Content = content.String()
				return nil
			}
			depth--
			content.WriteString("</" + t.Name.Local + ">")
		case xml.CharData:
			// Записываем текстовое содержимое
			content.Write(t)
		}
	}
	
	xc.Content = content.String()
	return nil
}

// String возвращает содержимое как строку
func (xc *XMLContent) String() string {
	return xc.Content
}

// ConstantRequest запрос константы
type ConstantRequest struct {
	XMLName     xml.Name      `xml:"constant"`
	UploadUUID  string        `xml:"upload_uuid"`
	Name        string        `xml:"name"`
	Synonym     string        `xml:"synonym"`
	Type        string        `xml:"type"`
	Value       ConstantValue  `xml:"value"` // Используем структуру с кастомным UnmarshalXML для получения всего содержимого тега value
	Timestamp   string        `xml:"timestamp"`
}

// ConstantResponse ответ на константу
type ConstantResponse struct {
	XMLName     xml.Name `xml:"constant_response"`
	Success     bool     `xml:"success"`
	Message     string   `xml:"message"`
	Timestamp   string   `xml:"timestamp"`
}

// CatalogMetaRequest запрос метаданных справочника
type CatalogMetaRequest struct {
	XMLName     xml.Name `xml:"catalog_meta"`
	UploadUUID  string   `xml:"upload_uuid"`
	Name        string   `xml:"name"`
	Synonym     string   `xml:"synonym"`
	Timestamp   string   `xml:"timestamp"`
}

// CatalogMetaResponse ответ на метаданные справочника
type CatalogMetaResponse struct {
	XMLName     xml.Name `xml:"catalog_meta_response"`
	Success     bool     `xml:"success"`
	CatalogID   int      `xml:"catalog_id"`
	Message     string   `xml:"message"`
	Timestamp   string   `xml:"timestamp"`
}

// CatalogItemRequest запрос элемента справочника
type CatalogItemRequest struct {
	XMLName     xml.Name   `xml:"catalog_item"`
	UploadUUID  string     `xml:"upload_uuid"`
	CatalogName string     `xml:"catalog_name"`
	Reference   string     `xml:"reference"`
	Code        string     `xml:"code"`
	Name        string     `xml:"name"`
	Attributes  XMLContent `xml:"attributes_xml"`  // XML строка - используем кастомный парсер
	TableParts  XMLContent `xml:"table_parts"`     // XML строка - используем кастомный парсер
	Timestamp   string     `xml:"timestamp"`
}

// CatalogItemResponse ответ на элемент справочника
type CatalogItemResponse struct {
	XMLName     xml.Name `xml:"catalog_item_response"`
	Success     bool     `xml:"success"`
	Message     string   `xml:"message"`
	Timestamp   string   `xml:"timestamp"`
}

// CatalogItem элемент справочника для пакетной выгрузки
type CatalogItem struct {
	XMLName     xml.Name   `xml:"item"`
	Reference   string     `xml:"reference"`
	Code        string     `xml:"code"`
	Name        string     `xml:"name"`
	Attributes  XMLContent `xml:"attributes_xml"`  // XML строка - используем кастомный парсер
	TableParts  XMLContent `xml:"table_parts"`     // XML строка - используем кастомный парсер
	Timestamp   string     `xml:"timestamp"`
}

// CatalogItemsRequest запрос пакетной загрузки элементов справочника
type CatalogItemsRequest struct {
	XMLName     xml.Name      `xml:"catalog_items"`
	UploadUUID  string        `xml:"upload_uuid"`
	CatalogName string        `xml:"catalog_name"`
	Items       []CatalogItem `xml:"items>item"`
}

// CatalogItemsResponse ответ на пакетную загрузку элементов справочника
type CatalogItemsResponse struct {
	XMLName        xml.Name `xml:"catalog_items_response"`
	Success        bool     `xml:"success"`
	ProcessedCount int      `xml:"processed_count"`
	FailedCount    int      `xml:"failed_count"`
	Message        string   `xml:"message"`
	Timestamp      string   `xml:"timestamp"`
}

// NomenclatureItem элемент номенклатуры с характеристикой для пакетной загрузки
type NomenclatureItem struct {
	XMLName                xml.Name `xml:"item"`
	NomenclatureReference  string   `xml:"nomenclature_reference"`
	NomenclatureCode       string   `xml:"nomenclature_code"`
	NomenclatureName       string   `xml:"nomenclature_name"`
	CharacteristicReference string  `xml:"characteristic_reference,omitempty"`
	CharacteristicName     string   `xml:"characteristic_name,omitempty"`
	Attributes             string   `xml:"attributes"`
	TableParts             string   `xml:"table_parts"`
	Timestamp              string   `xml:"timestamp,omitempty"`
}

// NomenclatureBatchRequest запрос пакетной загрузки номенклатуры с характеристиками
type NomenclatureBatchRequest struct {
	XMLName     xml.Name          `xml:"nomenclature_batch"`
	UploadUUID  string            `xml:"upload_uuid"`
	DatabaseID  string            `xml:"database_id,omitempty"`
	Items       []NomenclatureItem `xml:"items>item"`
	Timestamp   string            `xml:"timestamp,omitempty"`
}

// NomenclatureBatchResponse ответ на пакетную загрузку номенклатуры
type NomenclatureBatchResponse struct {
	XMLName        xml.Name `xml:"nomenclature_batch_response"`
	Success        bool     `xml:"success"`
	ProcessedCount int      `xml:"processed_count"`
	FailedCount    int      `xml:"failed_count"`
	Message        string   `xml:"message"`
	Timestamp      string   `xml:"timestamp"`
}

// CompleteRequest запрос завершения выгрузки
type CompleteRequest struct {
	XMLName     xml.Name `xml:"complete"`
	UploadUUID  string   `xml:"upload_uuid"`
	Timestamp   string   `xml:"timestamp"`
}

// CompleteResponse ответ на завершение выгрузки
type CompleteResponse struct {
	XMLName     xml.Name `xml:"complete_response"`
	Success     bool     `xml:"success"`
	Message     string   `xml:"message"`
	Timestamp   string   `xml:"timestamp"`
}

// ErrorResponse общий ответ об ошибке
type ErrorResponse struct {
	XMLName     xml.Name `xml:"error_response"`
	Success     bool     `xml:"success"`
	Error       string   `xml:"error"`
	Message     string   `xml:"message"`
	Timestamp   string   `xml:"timestamp"`
}

// ServerStats статистика сервера
type ServerStats struct {
	IsRunning       bool                   `json:"is_running"`
	CurrentUpload   *CurrentUploadInfo     `json:"current_upload"`
	TotalStats      map[string]interface{} `json:"total_stats"`
	LastActivity    time.Time              `json:"last_activity"`
}

// CurrentUploadInfo информация о текущей выгрузке
type CurrentUploadInfo struct {
	UploadUUID      string    `json:"upload_uuid"`
	StartedAt       time.Time `json:"started_at"`
	Status          string    `json:"status"`
	Version1C       string    `json:"version_1c"`
	ConfigName      string    `json:"config_name"`
	TotalConstants  int       `json:"total_constants"`
	TotalCatalogs   int       `json:"total_catalogs"`
	TotalItems      int       `json:"total_items"`
}

// LogEntry запись лога
type LogEntry struct {
	Timestamp   time.Time `json:"timestamp"`
	Level       string    `json:"level"`
	Message     string    `json:"message"`
	UploadUUID  string    `json:"upload_uuid,omitempty"`
	Endpoint    string    `json:"endpoint,omitempty"`
}

// API Models для получения данных

// UploadListItem краткая информация о выгрузке для списка
type UploadListItem struct {
	UploadUUID     string     `json:"upload_uuid"`
	StartedAt      time.Time  `json:"started_at"`
	CompletedAt    *time.Time `json:"completed_at,omitempty"`
	Status         string     `json:"status"`
	Version1C      string     `json:"version_1c"`
	ConfigName     string     `json:"config_name"`
	TotalConstants int        `json:"total_constants"`
	TotalCatalogs  int        `json:"total_catalogs"`
	TotalItems     int        `json:"total_items"`
}

// CatalogInfo информация о справочнике с количеством элементов
type CatalogInfo struct {
	ID        int       `json:"id"`
	Name      string    `json:"name"`
	Synonym   string    `json:"synonym"`
	ItemCount int       `json:"item_count"`
	CreatedAt time.Time `json:"created_at"`
}

// UploadDetails детальная информация о выгрузке
type UploadDetails struct {
	UploadUUID     string        `json:"upload_uuid"`
	StartedAt      time.Time     `json:"started_at"`
	CompletedAt    *time.Time    `json:"completed_at,omitempty"`
	Status         string        `json:"status"`
	Version1C      string        `json:"version_1c"`
	ConfigName     string        `json:"config_name"`
	TotalConstants int           `json:"total_constants"`
	TotalCatalogs  int           `json:"total_catalogs"`
	TotalItems     int           `json:"total_items"`
	Catalogs       []CatalogInfo `json:"catalogs"`
	Constants      []interface{} `json:"constants"`
}

// DataItem элемент данных для API ответа
type DataItem struct {
	XMLName   xml.Name   `xml:"item"`
	Type      string     `xml:"type,attr"` // "constant" или "catalog_item"
	ID        int        `xml:"id,attr"`
	Data      string     `xml:",innerxml"` // XML данные как строка
	CreatedAt time.Time  `xml:"created_at,attr"`
}

// DataResponse ответ с данными выгрузки
type DataResponse struct {
	XMLName   xml.Name   `xml:"data_response"`
	UploadUUID string     `xml:"upload_uuid"`
	Type       string     `xml:"type"` // "all", "constants", "catalogs"
	Page       int        `xml:"page"`
	Limit      int        `xml:"limit"`
	Total      int        `xml:"total"`
	Items      []DataItem `xml:"items>item"`
}

// VerifyRequest запрос на проверку передачи
type VerifyRequest struct {
	ReceivedIDs []int `json:"received_ids"`
}

// VerifyResponse ответ на проверку передачи
type VerifyResponse struct {
	UploadUUID      string   `json:"upload_uuid"`
	ExpectedTotal   int      `json:"expected_total"`
	ReceivedCount   int      `json:"received_count"`
	MissingIDs      []int    `json:"missing_ids,omitempty"`
	IsComplete      bool     `json:"is_complete"`
	Message         string   `json:"message"`
}

// NomenclatureProcessingResponse ответ на запрос запуска обработки номенклатуры
type NomenclatureProcessingResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

// DBStatsResponse статистика базы данных
type DBStatsResponse struct {
	Total    int `json:"total"`
	Completed int `json:"completed"`
	Errors   int `json:"errors"`
	Pending  int `json:"pending"`
}

// ProcessingStatsResponse статистика обработки из процессора
type ProcessingStatsResponse struct {
	Total      int64     `json:"total"`
	Processed  int64     `json:"processed"`
	Successful int64     `json:"successful"`
	Failed     int64     `json:"failed"`
	StartTime  time.Time `json:"start_time"`
	MaxWorkers int       `json:"max_workers"`
}

// NomenclatureStatusResponse полный ответ со статусом обработки
type NomenclatureStatusResponse struct {
	Processing   bool                   `json:"processing"`
	CurrentStats *ProcessingStatsResponse `json:"current_stats,omitempty"`
	DBStats      DBStatsResponse         `json:"db_stats"`
}

// RecentRecord последняя обработанная запись
type RecentRecord struct {
	ID             int       `json:"id"`
	OriginalName   string    `json:"original_name"`
	NormalizedName string    `json:"normalized_name"`
	KpvedCode      string    `json:"kpved_code"`
	KpvedName      string    `json:"kpved_name"`
	Status         string    `json:"status"`
	ProcessedAt    *time.Time `json:"processed_at,omitempty"`
}

// RecentRecordsResponse ответ с последними записями
type RecentRecordsResponse struct {
	Records []RecentRecord `json:"records"`
	Total   int            `json:"total"`
}

// PendingRecord необработанная запись
type PendingRecord struct {
	ID           int       `json:"id"`
	OriginalName string    `json:"original_name"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
}

// PendingRecordsResponse ответ с необработанными записями
type PendingRecordsResponse struct {
	Records []PendingRecord `json:"records"`
	Total   int             `json:"total"`
}

// Client модели для работы с клиентами
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

// ClientProject модель проекта клиента
type ClientProject struct {
	ID                int       `json:"id"`
	ClientID          int       `json:"client_id"`
	Name              string    `json:"name"`
	ProjectType       string    `json:"project_type"`
	Description       string    `json:"description"`
	SourceSystem      string    `json:"source_system"`
	Status            string    `json:"status"`
	TargetQualityScore float64  `json:"target_quality_score"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// ClientBenchmark модель эталонной записи клиента
type ClientBenchmark struct {
	ID               int        `json:"id"`
	ClientProjectID  int        `json:"client_project_id"`
	OriginalName     string     `json:"original_name"`
	NormalizedName   string     `json:"normalized_name"`
	Category         string     `json:"category"`
	Subcategory      string     `json:"subcategory"`
	Attributes       string     `json:"attributes"` // JSON строка
	QualityScore     float64    `json:"quality_score"`
	IsApproved       bool       `json:"is_approved"`
	ApprovedBy       string     `json:"approved_by"`
	ApprovedAt       *time.Time `json:"approved_at"`
	SourceDatabase   string     `json:"source_database"`
	UsageCount       int        `json:"usage_count"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

// ClientListResponse ответ со списком клиентов
type ClientListResponse struct {
	Clients []ClientListItem `json:"clients"`
	Total   int              `json:"total"`
}

// ClientListItem краткая информация о клиенте для списка
type ClientListItem struct {
	ID            int       `json:"id"`
	Name          string    `json:"name"`
	LegalName     string    `json:"legal_name"`
	Description   string    `json:"description"`
	Status        string    `json:"status"`
	ProjectCount  int       `json:"project_count"`
	BenchmarkCount int      `json:"benchmark_count"`
	LastActivity  time.Time `json:"last_activity"`
}

// ClientDetailResponse детальная информация о клиенте
type ClientDetailResponse struct {
	Client     Client           `json:"client"`
	Projects   []ClientProject  `json:"projects"`
	Statistics ClientStatistics `json:"statistics"`
}

// ClientStatistics статистика клиента
type ClientStatistics struct {
	TotalProjects    int     `json:"total_projects"`
	TotalBenchmarks  int     `json:"total_benchmarks"`
	ActiveSessions   int     `json:"active_sessions"`
	AvgQualityScore  float64 `json:"avg_quality_score"`
}

// ClientProjectResponse ответ с информацией о проекте
type ClientProjectResponse struct {
	Project   ClientProject        `json:"project"`
	Benchmarks []ClientBenchmark   `json:"benchmarks"`
	Statistics ProjectStatistics   `json:"statistics"`
}

// ProjectStatistics статистика проекта
type ProjectStatistics struct {
	TotalBenchmarks    int     `json:"total_benchmarks"`
	ApprovedBenchmarks int     `json:"approved_benchmarks"`
	AvgQualityScore    float64 `json:"avg_quality_score"`
	LastActivity       time.Time `json:"last_activity"`
}

// ClientBenchmarkResponse ответ со списком эталонов
type ClientBenchmarkResponse struct {
	Benchmarks []ClientBenchmark `json:"benchmarks"`
	Total      int               `json:"total"`
}

// ============================================================================
// Quality Models
// ============================================================================

// QualityReport отчет о качестве выгрузки
type QualityReport struct {
	UploadUUID   string                    `json:"upload_uuid"`
	DatabaseID   int                       `json:"database_id"`
	AnalyzedAt   *time.Time                `json:"analyzed_at,omitempty"`
	OverallScore float64                   `json:"overall_score"`
	Metrics      []database.DataQualityMetric `json:"metrics"`
	Issues       []database.DataQualityIssue  `json:"issues"`
	Summary      QualitySummary            `json:"summary"`
}

// QualitySummary сводка по качеству
type QualitySummary struct {
	TotalIssues       int            `json:"total_issues"`
	CriticalIssues    int            `json:"critical_issues"`
	HighIssues        int            `json:"high_issues"`
	MediumIssues      int            `json:"medium_issues"`
	LowIssues         int            `json:"low_issues"`
	MetricsByCategory map[string]float64 `json:"metrics_by_category"`
}

// QualityDashboard дашборд качества базы данных
type QualityDashboard struct {
	DatabaseID      int                       `json:"database_id"`
	CurrentScore    float64                   `json:"current_score"`
	Trends          []database.QualityTrend   `json:"trends"`
	TopIssues       []database.DataQualityIssue `json:"top_issues"`
	MetricsByEntity map[string]EntityMetrics  `json:"metrics_by_entity"`
	Recommendations []Recommendation          `json:"recommendations,omitempty"`
}

// EntityMetrics метрики по типу сущности
type EntityMetrics struct {
	Completeness float64 `json:"completeness"`
	Consistency  float64 `json:"consistency"`
	Uniqueness   float64 `json:"uniqueness"`
	Validity     float64 `json:"validity"`
	OverallScore float64 `json:"overall_score"`
}

// Recommendation рекомендация по улучшению качества
type Recommendation struct {
	ID          int    `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Impact      string `json:"impact"` // HIGH, MEDIUM, LOW
	Effort      string `json:"effort"` // HIGH, MEDIUM, LOW
	EntityType  string `json:"entity_type"`
	IssueType   string `json:"issue_type"`
	ActionPlan  string `json:"action_plan,omitempty"`
}

// ============================================================================
// Snapshot Models
// ============================================================================

// SnapshotRequest запрос создания среза
type SnapshotRequest struct {
	Name            string                  `json:"name"`
	Description     string                  `json:"description"`
	SnapshotType    string                  `json:"snapshot_type"` // manual, auto_latest, auto_period
	ProjectID       *int                    `json:"project_id,omitempty"`
	ClientID        *int                    `json:"client_id,omitempty"`
	IncludedUploads []SnapshotUploadRequest `json:"included_uploads"`
}

// SnapshotUploadRequest запрос на добавление выгрузки в срез
type SnapshotUploadRequest struct {
	UploadID       int    `json:"upload_id"`
	IterationLabel string `json:"iteration_label,omitempty"`
	UploadOrder    int    `json:"upload_order"`
}

// SnapshotResponse ответ со срезом
type SnapshotResponse struct {
	ID           int       `json:"id"`
	Name         string    `json:"name"`
	Description  string    `json:"description"`
	CreatedBy    *int      `json:"created_by,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	SnapshotType string    `json:"snapshot_type"`
	ProjectID    *int      `json:"project_id,omitempty"`
	ClientID     *int      `json:"client_id,omitempty"`
	Uploads      []UploadListItem `json:"uploads,omitempty"`
	UploadCount  int       `json:"upload_count"`
}

// AutoSnapshotRequest запрос автоматического создания среза
type AutoSnapshotRequest struct {
	Type              string `json:"type"` // "latest_per_database"
	ProjectID         int    `json:"project_id"`
	UploadsPerDatabase int   `json:"uploads_per_database"`
	Name              string `json:"name,omitempty"`
	Description       string `json:"description,omitempty"`
}

// SnapshotNormalizationRequest запрос нормализации среза
type SnapshotNormalizationRequest struct {
	UseAI              bool    `json:"use_ai,omitempty"`
	MinConfidence       float64 `json:"min_confidence,omitempty"`
	RateLimitDelayMS    int     `json:"rate_limit_delay_ms,omitempty"`
	MaxRetries          int     `json:"max_retries,omitempty"`
}

// SnapshotNormalizationResult результат нормализации среза
type SnapshotNormalizationResult struct {
	SnapshotID      int                        `json:"snapshot_id"`
	MasterReference map[string]string           `json:"master_reference,omitempty"`
	UploadResults   map[int]*UploadNormalizationResult `json:"upload_results"`
	TotalProcessed  int                        `json:"total_processed"`
	TotalGroups     int                        `json:"total_groups"`
	CompletedAt     time.Time                  `json:"completed_at"`
}

// UploadNormalizationResult результат нормализации для одной выгрузки
type UploadNormalizationResult struct {
	UploadID       int                      `json:"upload_id"`
	ProcessedCount int                      `json:"processed_count"`
	GroupCount     int                      `json:"group_count"`
	Error          string                   `json:"error,omitempty"`
	Changes        *NormalizationChanges    `json:"changes,omitempty"`
}

// NormalizationChanges представляет изменения после нормализации (дубликат из normalization для удобства)
type NormalizationChanges struct {
	Added   int `json:"added"`
	Updated int `json:"updated"`
	Deleted int `json:"deleted"`
}

// SnapshotListResponse ответ со списком срезов
type SnapshotListResponse struct {
	Snapshots []SnapshotResponse `json:"snapshots"`
	Total     int                `json:"total"`
}

// SnapshotComparisonResponse ответ со сравнением итераций
type SnapshotComparisonResponse struct {
	SnapshotID      int                        `json:"snapshot_id"`
	Iterations      []IterationComparison      `json:"iterations"`
	TotalItems      map[int]int                `json:"total_items"` // upload_id -> count
	Changes         []ItemChange               `json:"changes,omitempty"`
}

// IterationComparison сравнение итерации
type IterationComparison struct {
	UploadID         int       `json:"upload_id"`
	IterationNumber  int       `json:"iteration_number"`
	IterationLabel   string    `json:"iteration_label"`
	StartedAt         time.Time `json:"started_at"`
	TotalItems        int       `json:"total_items"`
	TotalCatalogs     int       `json:"total_catalogs"`
	DatabaseID        *int      `json:"database_id,omitempty"`
}

// ItemChange изменение элемента между итерациями
type ItemChange struct {
	Reference        string    `json:"reference"`
	Field            string    `json:"field"`
	OldValue         string    `json:"old_value"`
	NewValue         string    `json:"new_value"`
	FromUploadID     int       `json:"from_upload_id"`
	ToUploadID       int       `json:"to_upload_id"`
}

// SnapshotMetricsResponse ответ с метриками улучшения данных
type SnapshotMetricsResponse struct {
	SnapshotID       int                        `json:"snapshot_id"`
	QualityScores    map[int]float64            `json:"quality_scores"` // upload_id -> score
	Improvements     []QualityImprovement       `json:"improvements"`
	OverallTrend     string                     `json:"overall_trend"` // "improving", "stable", "degrading"
}

// QualityImprovement улучшение качества
type QualityImprovement struct {
	Metric           string    `json:"metric"`
	FromValue        float64   `json:"from_value"`
	ToValue          float64   `json:"to_value"`
	Improvement      float64   `json:"improvement"`
	FromUploadID     int       `json:"from_upload_id"`
	ToUploadID       int       `json:"to_upload_id"`
}

// SnapshotEvolutionResponse ответ с эволюцией номенклатуры
type SnapshotEvolutionResponse struct {
	SnapshotID       int                        `json:"snapshot_id"`
	Evolution        []NomenclatureEvolution   `json:"evolution"`
	TotalTracked    int                        `json:"total_tracked"`
}

// NomenclatureEvolution эволюция номенклатуры
type NomenclatureEvolution struct {
	Reference        string                     `json:"reference"`
	Name             string                     `json:"name"`
	History          []NomenclatureHistoryItem  `json:"history"`
	Status           string                     `json:"status"` // "new", "modified", "removed", "stable"
}

// NomenclatureHistoryItem элемент истории номенклатуры
type NomenclatureHistoryItem struct {
	UploadID         int       `json:"upload_id"`
	IterationNumber  int       `json:"iteration_number"`
	Name             string    `json:"name"`
	Category         string    `json:"category,omitempty"`
	ChangedAt        time.Time `json:"changed_at"`
}


