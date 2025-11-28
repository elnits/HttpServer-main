package server

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"httpserver/database"

	"github.com/google/uuid"
)

const (
	defaultExportTimeout   = 30 * time.Second
	maxExportBatchSize     = 2000
	minExportBatchSize     = 1
	defaultExportBatchSize = 500
)

// ExportRequest описание входящего JSON-запроса на обратную выгрузку.
type ExportRequest struct {
	TargetURL      string   `json:"target_url"`
	Include        []string `json:"include"`
	CatalogNames   []string `json:"catalog_names"`
	BatchSize      int      `json:"batch_size"`
	TimeoutSeconds int      `json:"timeout_seconds"`
}

// ExportOptions нормализованные настройки экспорта.
type ExportOptions struct {
	IncludeMetadata     bool     `json:"include_metadata"`
	IncludeConstants    bool     `json:"include_constants"`
	IncludeCatalogs     bool     `json:"include_catalogs"`
	IncludeNomenclature bool     `json:"include_nomenclature"`
	CatalogNames        []string `json:"catalog_names,omitempty"`
	BatchSize           int      `json:"batch_size"`
}

// ExportProgress отображает текущий прогресс.
type ExportProgress struct {
	Handshake          bool `json:"handshake"`
	Metadata           bool `json:"metadata"`
	ConstantsSent      int  `json:"constants_sent"`
	CatalogsSent       int  `json:"catalogs_sent"`
	CatalogItemsSent   int  `json:"catalog_items_sent"`
	NomenclatureSent   int  `json:"nomenclature_sent"`
	CompleteDispatched bool `json:"complete_dispatched"`
}

// ExportStatus состояние задачи.
type ExportStatus string

const (
	ExportStatusPending  ExportStatus = "pending"
	ExportStatusRunning  ExportStatus = "running"
	ExportStatusFailed   ExportStatus = "failed"
	ExportStatusFinished ExportStatus = "completed"
)

// ExportJob внутренняя структура задачи обратной выгрузки.
type ExportJob struct {
	mu               sync.RWMutex
	ID               string
	UploadUUID       string
	RemoteUploadUUID string
	TargetURL        string
	Status           ExportStatus
	Error            string
	CreatedAt        time.Time
	StartedAt        *time.Time
	FinishedAt       *time.Time
	Progress         ExportProgress
	Options          ExportOptions
	Timeout          time.Duration
}

// ExportJobView DTO для ответа API.
type ExportJobView struct {
	ID               string          `json:"id"`
	UploadUUID       string          `json:"upload_uuid"`
	RemoteUploadUUID string          `json:"remote_upload_uuid,omitempty"`
	TargetURL        string          `json:"target_url"`
	Status           ExportStatus    `json:"status"`
	Error            string          `json:"error,omitempty"`
	CreatedAt        time.Time       `json:"created_at"`
	StartedAt        *time.Time      `json:"started_at,omitempty"`
	FinishedAt       *time.Time      `json:"finished_at,omitempty"`
	Progress         ExportProgress  `json:"progress"`
	Options          ExportOptions   `json:"options"`
}

// xmlSuccessResponse упрощенный ответ на XML-запросы.
type xmlSuccessResponse struct {
	Success bool   `xml:"success"`
	Message string `xml:"message"`
}

// Структуры для исходящих XML.
type constantExport struct {
	XMLName    xml.Name `xml:"constant"`
	UploadUUID string   `xml:"upload_uuid"`
	Name       string   `xml:"name"`
	Synonym    string   `xml:"synonym"`
	Type       string   `xml:"type"`
	Value      string   `xml:"value,innerxml"`
	Timestamp  string   `xml:"timestamp"`
}

type catalogMetaExport struct {
	XMLName    xml.Name `xml:"catalog_meta"`
	UploadUUID string   `xml:"upload_uuid"`
	Name       string   `xml:"name"`
	Synonym    string   `xml:"synonym"`
	Timestamp  string   `xml:"timestamp"`
}

type catalogItemExport struct {
	XMLName     xml.Name `xml:"catalog_item"`
	UploadUUID  string   `xml:"upload_uuid"`
	CatalogName string   `xml:"catalog_name"`
	Reference   string   `xml:"reference"`
	Code        string   `xml:"code"`
	Name        string   `xml:"name"`
	Attributes  string   `xml:"attributes_xml,innerxml"`
	TableParts  string   `xml:"table_parts,innerxml"`
	Timestamp   string   `xml:"timestamp"`
}

type nomenclatureBatchExport struct {
	XMLName    xml.Name                   `xml:"nomenclature_batch"`
	UploadUUID string                     `xml:"upload_uuid"`
	Items      []nomenclatureItemExport   `xml:"items>item"`
	Timestamp  string                     `xml:"timestamp,omitempty"`
}

type nomenclatureItemExport struct {
	NomenclatureReference  string `xml:"nomenclature_reference"`
	NomenclatureCode       string `xml:"nomenclature_code"`
	NomenclatureName       string `xml:"nomenclature_name"`
	CharacteristicReference string `xml:"characteristic_reference,omitempty"`
	CharacteristicName     string `xml:"characteristic_name,omitempty"`
	Attributes             string `xml:"attributes,innerxml"`
	TableParts             string `xml:"table_parts,innerxml"`
	Timestamp              string `xml:"timestamp,omitempty"`
}

type completeExport struct {
	XMLName    xml.Name `xml:"complete"`
	UploadUUID string   `xml:"upload_uuid"`
	Timestamp  string   `xml:"timestamp"`
}

func newExportJob(uploadUUID, targetURL string, options ExportOptions, timeout time.Duration) *ExportJob {
	return &ExportJob{
		ID:         uuid.New().String(),
		UploadUUID: uploadUUID,
		TargetURL:  targetURL,
		Status:     ExportStatusPending,
		CreatedAt:  time.Now(),
		Options:    options,
		Timeout:    timeout,
	}
}

func (job *ExportJob) snapshot() ExportJobView {
	job.mu.RLock()
	defer job.mu.RUnlock()

	view := ExportJobView{
		ID:               job.ID,
		UploadUUID:       job.UploadUUID,
		RemoteUploadUUID: job.RemoteUploadUUID,
		TargetURL:        job.TargetURL,
		Status:           job.Status,
		Error:            job.Error,
		CreatedAt:        job.CreatedAt,
		Progress:         job.Progress,
		Options:          job.Options,
	}

	if job.StartedAt != nil {
		start := *job.StartedAt
		view.StartedAt = &start
	}
	if job.FinishedAt != nil {
		finish := *job.FinishedAt
		view.FinishedAt = &finish
	}

	if len(job.Options.CatalogNames) > 0 {
		names := make([]string, len(job.Options.CatalogNames))
		copy(names, job.Options.CatalogNames)
		view.Options.CatalogNames = names
	}

	return view
}

func (job *ExportJob) markRunning() {
	job.mu.Lock()
	defer job.mu.Unlock()
	now := time.Now()
	job.Status = ExportStatusRunning
	job.StartedAt = &now
	job.Error = ""
}

func (job *ExportJob) markFailed(err error) {
	job.mu.Lock()
	defer job.mu.Unlock()
	now := time.Now()
	job.Status = ExportStatusFailed
	job.FinishedAt = &now
	if err != nil {
		job.Error = err.Error()
	}
}

func (job *ExportJob) markCompleted() {
	job.mu.Lock()
	defer job.mu.Unlock()
	now := time.Now()
	job.Status = ExportStatusFinished
	job.FinishedAt = &now
	job.Error = ""
}

func (job *ExportJob) setRemoteUpload(uuid string) {
	job.mu.Lock()
	defer job.mu.Unlock()
	job.RemoteUploadUUID = uuid
}

func (job *ExportJob) setHandshakeDone() {
	job.mu.Lock()
	defer job.mu.Unlock()
	job.Progress.Handshake = true
}

func (job *ExportJob) setMetadataDone() {
	job.mu.Lock()
	defer job.mu.Unlock()
	job.Progress.Metadata = true
}

func (job *ExportJob) addConstants(delta int) {
	if delta == 0 {
		return
	}
	job.mu.Lock()
	job.Progress.ConstantsSent += delta
	job.mu.Unlock()
}

func (job *ExportJob) addCatalogs(delta int) {
	if delta == 0 {
		return
	}
	job.mu.Lock()
	job.Progress.CatalogsSent += delta
	job.mu.Unlock()
}

func (job *ExportJob) addCatalogItems(delta int) {
	if delta == 0 {
		return
	}
	job.mu.Lock()
	job.Progress.CatalogItemsSent += delta
	job.mu.Unlock()
}

func (job *ExportJob) addNomenclature(delta int) {
	if delta == 0 {
		return
	}
	job.mu.Lock()
	job.Progress.NomenclatureSent += delta
	job.mu.Unlock()
}

func (job *ExportJob) markCompleteDispatched() {
	job.mu.Lock()
	defer job.mu.Unlock()
	job.Progress.CompleteDispatched = true
}

// --- Handlers ---

func (s *Server) handleUploadExport(w http.ResponseWriter, r *http.Request, upload *database.Upload) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var payload ExportRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		s.writeJSONError(w, fmt.Sprintf("Invalid payload: %v", err), http.StatusBadRequest)
		return
	}

	targetURL, err := normalizeTargetURL(payload.TargetURL)
	if err != nil {
		s.writeJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	options, err := normalizeExportOptions(payload)
	if err != nil {
		s.writeJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	timeout := defaultExportTimeout
	if payload.TimeoutSeconds > 0 {
		timeout = time.Duration(payload.TimeoutSeconds) * time.Second
	}

	job := newExportJob(upload.UploadUUID, targetURL, options, timeout)

	s.exportJobsMutex.Lock()
	s.exportJobs[job.ID] = job
	s.exportJobsMutex.Unlock()

	s.log(LogEntry{
		Timestamp: time.Now(),
		Level:     "INFO",
		Message:   fmt.Sprintf("Export job %s created for upload %s -> %s", job.ID, job.UploadUUID, job.TargetURL),
		UploadUUID: upload.UploadUUID,
		Endpoint:  "/api/uploads/{uuid}/export",
	})

	go s.runExportJob(job, upload)

	s.writeJSONResponse(w, job.snapshot(), http.StatusAccepted)
}

func (s *Server) handleUploadExportsList(w http.ResponseWriter, r *http.Request, upload *database.Upload) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	jobs := s.getExportJobsByUpload(upload.UploadUUID)
	response := map[string]interface{}{
		"exports": jobs,
		"total":   len(jobs),
	}
	s.writeJSONResponse(w, response, http.StatusOK)
}

func (s *Server) handleExportsRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/api/exports" {
		http.NotFound(w, r)
		return
	}

	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	jobs := s.getAllExportJobs()
	response := map[string]interface{}{
		"exports": jobs,
		"total":   len(jobs),
	}
	s.writeJSONResponse(w, response, http.StatusOK)
}

func (s *Server) handleExportRoutes(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/exports/")
	if path == "" {
		http.NotFound(w, r)
		return
	}

	job := s.getExportJob(path)
	if job == nil {
		s.writeJSONError(w, "Export job not found", http.StatusNotFound)
		return
	}

	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.writeJSONResponse(w, job.snapshot(), http.StatusOK)
}

// --- Export execution ---

func (s *Server) runExportJob(job *ExportJob, upload *database.Upload) {
	job.markRunning()

	uploadDB, err := s.getUploadDatabase(upload.UploadUUID)
	if err != nil {
		job.markFailed(fmt.Errorf("failed to find upload database: %w", err))
		return
	}

	client := &http.Client{Timeout: job.Timeout}
	baseURL := job.TargetURL

	remoteUUID, err := s.performExportHandshake(client, baseURL, upload)
	if err != nil {
		job.markFailed(err)
		s.logExportError(job, err, "handshake")
		return
	}
	job.setRemoteUpload(remoteUUID)
	job.setHandshakeDone()

	if job.Options.IncludeMetadata {
		if err := s.sendExportMetadata(client, baseURL, remoteUUID, upload); err != nil {
			job.markFailed(err)
			s.logExportError(job, err, "metadata")
			return
		}
		job.setMetadataDone()
	}

	if job.Options.IncludeConstants {
		err = uploadDB.StreamConstantsByUpload(upload.ID, job.Options.BatchSize, func(batch []*database.Constant) error {
			sent := 0
			for _, constant := range batch {
				if err := s.sendExportConstant(client, baseURL, remoteUUID, constant); err != nil {
					return err
				}
				sent++
			}
			job.addConstants(sent)
			return nil
		})
		if err != nil {
			job.markFailed(err)
			s.logExportError(job, err, "constants")
			return
		}
	}

	if job.Options.IncludeCatalogs {
		catalogFilter := make(map[string]struct{})
		for _, name := range job.Options.CatalogNames {
			catalogFilter[name] = struct{}{}
		}

		catalogs, err := uploadDB.GetCatalogsByUpload(upload.ID)
		if err != nil {
			job.markFailed(fmt.Errorf("failed to load catalogs: %w", err))
			s.logExportError(job, err, "catalog_meta")
			return
		}

		metaSent := 0
		for _, catalog := range catalogs {
			if len(catalogFilter) > 0 {
				if _, ok := catalogFilter[catalog.Name]; !ok {
					continue
				}
			}
			if err := s.sendExportCatalogMeta(client, baseURL, remoteUUID, catalog); err != nil {
				job.markFailed(err)
				s.logExportError(job, err, "catalog_meta")
				return
			}
			metaSent++
		}
		job.addCatalogs(metaSent)

		err = uploadDB.StreamCatalogItems(upload.ID, job.Options.CatalogNames, job.Options.BatchSize, func(items []*database.CatalogItem) error {
			sent := 0
			for _, item := range items {
				if err := s.sendExportCatalogItem(client, baseURL, remoteUUID, item); err != nil {
					return err
				}
				sent++
			}
			job.addCatalogItems(sent)
			return nil
		})
		if err != nil {
			job.markFailed(err)
			s.logExportError(job, err, "catalog_items")
			return
		}
	}

	if job.Options.IncludeNomenclature {
		err = uploadDB.StreamNomenclatureItems(upload.ID, job.Options.BatchSize, func(items []*database.NomenclatureItem) error {
			if len(items) == 0 {
				return nil
			}
			if err := s.sendExportNomenclatureBatch(client, baseURL, remoteUUID, items); err != nil {
				return err
			}
			job.addNomenclature(len(items))
			return nil
		})
		if err != nil {
			job.markFailed(err)
			s.logExportError(job, err, "nomenclature")
			return
		}
	}

	if err := s.sendExportComplete(client, baseURL, remoteUUID); err != nil {
		job.markFailed(err)
		s.logExportError(job, err, "complete")
		return
	}
	job.markCompleteDispatched()
	job.markCompleted()

	s.log(LogEntry{
		Timestamp: time.Now(),
		Level:     "INFO",
		Message:   fmt.Sprintf("Export job %s finished", job.ID),
		UploadUUID: job.UploadUUID,
		Endpoint:  "/api/uploads/{uuid}/export",
	})
}

func (s *Server) logExportError(job *ExportJob, err error, stage string) {
	s.log(LogEntry{
		Timestamp:  time.Now(),
		Level:      "ERROR",
		Message:    fmt.Sprintf("Export job %s failed at %s: %v", job.ID, stage, err),
		UploadUUID: job.UploadUUID,
		Endpoint:   "/api/uploads/{uuid}/export",
	})
}

// --- Helpers for options and jobs ---

func normalizeTargetURL(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", errors.New("target_url is required")
	}
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", fmt.Errorf("invalid target_url: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", errors.New("target_url must include scheme and host")
	}
	return strings.TrimRight(parsed.String(), "/"), nil
}

func normalizeExportOptions(req ExportRequest) (ExportOptions, error) {
	opts := ExportOptions{
		IncludeMetadata:     true,
		IncludeConstants:    true,
		IncludeCatalogs:     true,
		IncludeNomenclature: true,
		BatchSize:           defaultExportBatchSize,
	}

	if len(req.Include) > 0 {
		opts.IncludeMetadata = false
		opts.IncludeConstants = false
		opts.IncludeCatalogs = false
		opts.IncludeNomenclature = false

		for _, value := range req.Include {
			switch strings.ToLower(strings.TrimSpace(value)) {
			case "metadata":
				opts.IncludeMetadata = true
			case "constants":
				opts.IncludeConstants = true
			case "catalogs":
				opts.IncludeCatalogs = true
			case "nomenclature":
				opts.IncludeNomenclature = true
			case "":
				continue
			default:
				return opts, fmt.Errorf("unknown include value: %s", value)
			}
		}
	}

	if req.BatchSize > 0 {
		opts.BatchSize = req.BatchSize
	}
	if opts.BatchSize < minExportBatchSize {
		opts.BatchSize = minExportBatchSize
	}
	if opts.BatchSize > maxExportBatchSize {
		opts.BatchSize = maxExportBatchSize
	}

	if len(req.CatalogNames) > 0 {
		names := make([]string, 0, len(req.CatalogNames))
		seen := make(map[string]struct{})
		for _, name := range req.CatalogNames {
			clean := strings.TrimSpace(name)
			if clean == "" {
				continue
			}
			if _, exists := seen[clean]; exists {
				continue
			}
			seen[clean] = struct{}{}
			names = append(names, clean)
		}
		opts.CatalogNames = names
	}

	return opts, nil
}

func (s *Server) getExportJobsByUpload(uploadUUID string) []ExportJobView {
	s.exportJobsMutex.RLock()
	defer s.exportJobsMutex.RUnlock()

	views := make([]ExportJobView, 0)
	for _, job := range s.exportJobs {
		if job.UploadUUID == uploadUUID {
			views = append(views, job.snapshot())
		}
	}

	sort.Slice(views, func(i, j int) bool {
		return views[i].CreatedAt.After(views[j].CreatedAt)
	})

	return views
}

func (s *Server) getAllExportJobs() []ExportJobView {
	s.exportJobsMutex.RLock()
	defer s.exportJobsMutex.RUnlock()

	views := make([]ExportJobView, 0, len(s.exportJobs))
	for _, job := range s.exportJobs {
		views = append(views, job.snapshot())
	}

	sort.Slice(views, func(i, j int) bool {
		return views[i].CreatedAt.After(views[j].CreatedAt)
	})

	return views
}

func (s *Server) getExportJob(id string) *ExportJob {
	s.exportJobsMutex.RLock()
	defer s.exportJobsMutex.RUnlock()
	return s.exportJobs[id]
}

// --- Remote calls ---

func (s *Server) performExportHandshake(client *http.Client, baseURL string, upload *database.Upload) (string, error) {
	req := HandshakeRequest{
		Version1C:      upload.Version1C,
		ConfigName:     upload.ConfigName,
		ConfigVersion:  upload.ConfigVersion,
		ComputerName:   upload.ComputerName,
		UserName:       upload.UserName,
		Timestamp:      time.Now().Format(time.RFC3339),
		UploadType:     "ОбратнаяВыгрузка",
		IterationNumber: upload.IterationNumber,
		IterationLabel: upload.IterationLabel,
		ProgrammerName: upload.ProgrammerName,
		UploadPurpose:  upload.UploadPurpose,
	}
	if upload.DatabaseID != nil {
		req.DatabaseID = fmt.Sprintf("%d", *upload.DatabaseID)
	}
	var resp HandshakeResponse
	if err := s.postXML(client, baseURL+"/handshake", req, &resp); err != nil {
		return "", err
	}
	if !resp.Success {
		return "", fmt.Errorf("remote handshake failed: %s", resp.Message)
	}
	if resp.UploadUUID == "" {
		return "", errors.New("remote handshake response missing upload_uuid")
	}
	return resp.UploadUUID, nil
}

func (s *Server) sendExportMetadata(client *http.Client, baseURL, remoteUUID string, upload *database.Upload) error {
	req := MetadataRequest{
		UploadUUID:    remoteUUID,
		Version1C:     upload.Version1C,
		ConfigName:    upload.ConfigName,
		ConfigVersion: upload.ConfigVersion,
		ComputerName:  upload.ComputerName,
		UserName:      upload.UserName,
		Timestamp:     time.Now().Format(time.RFC3339),
	}
	if upload.DatabaseID != nil {
		req.DatabaseID = fmt.Sprintf("%d", *upload.DatabaseID)
	}
	return s.expectXMLSuccess(client, baseURL+"/metadata", req)
}

func (s *Server) sendExportConstant(client *http.Client, baseURL, remoteUUID string, constant *database.Constant) error {
	req := constantExport{
		UploadUUID: remoteUUID,
		Name:       constant.Name,
		Synonym:    constant.Synonym,
		Type:       constant.Type,
		Value:      constant.Value,
		Timestamp:  constant.CreatedAt.Format(time.RFC3339),
	}
	return s.expectXMLSuccess(client, baseURL+"/constant", req)
}

func (s *Server) sendExportCatalogMeta(client *http.Client, baseURL, remoteUUID string, catalog *database.Catalog) error {
	req := catalogMetaExport{
		UploadUUID: remoteUUID,
		Name:       catalog.Name,
		Synonym:    catalog.Synonym,
		Timestamp:  catalog.CreatedAt.Format(time.RFC3339),
	}
	return s.expectXMLSuccess(client, baseURL+"/catalog/meta", req)
}

func (s *Server) sendExportCatalogItem(client *http.Client, baseURL, remoteUUID string, item *database.CatalogItem) error {
	req := catalogItemExport{
		UploadUUID:  remoteUUID,
		CatalogName: item.CatalogName,
		Reference:   item.Reference,
		Code:        item.Code,
		Name:        item.Name,
		Attributes:  item.Attributes,
		TableParts:  item.TableParts,
		Timestamp:   item.CreatedAt.Format(time.RFC3339),
	}
	return s.expectXMLSuccess(client, baseURL+"/catalog/item", req)
}

func (s *Server) sendExportNomenclatureBatch(client *http.Client, baseURL, remoteUUID string, items []*database.NomenclatureItem) error {
	payload := nomenclatureBatchExport{
		UploadUUID: remoteUUID,
		Timestamp:  time.Now().Format(time.RFC3339),
		Items:      make([]nomenclatureItemExport, len(items)),
	}
	for i, item := range items {
		payload.Items[i] = nomenclatureItemExport{
			NomenclatureReference:  item.NomenclatureReference,
			NomenclatureCode:       item.NomenclatureCode,
			NomenclatureName:       item.NomenclatureName,
			CharacteristicReference: item.CharacteristicReference,
			CharacteristicName:     item.CharacteristicName,
			Attributes:             item.AttributesXML,
			TableParts:             item.TablePartsXML,
		}
	}
	return s.expectXMLSuccess(client, baseURL+"/nomenclature/batch", payload)
}

func (s *Server) sendExportComplete(client *http.Client, baseURL, remoteUUID string) error {
	req := completeExport{
		UploadUUID: remoteUUID,
		Timestamp:  time.Now().Format(time.RFC3339),
	}
	return s.expectXMLSuccess(client, baseURL+"/complete", req)
}

func (s *Server) expectXMLSuccess(client *http.Client, endpoint string, payload interface{}) error {
	var resp xmlSuccessResponse
	if err := s.postXML(client, endpoint, payload, &resp); err != nil {
		return err
	}
	if !resp.Success {
		return fmt.Errorf("remote endpoint %s returned error: %s", endpoint, resp.Message)
	}
	return nil
}

func (s *Server) postXML(client *http.Client, endpoint string, payload interface{}, out interface{}) error {
	var body bytes.Buffer
	body.WriteString(xml.Header)
	encoder := xml.NewEncoder(&body)
	encoder.Indent("", "  ")
	if err := encoder.Encode(payload); err != nil {
		return fmt.Errorf("failed to encode XML: %w", err)
	}
	if err := encoder.Flush(); err != nil {
		return fmt.Errorf("failed to flush XML: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, endpoint, &body)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/xml; charset=utf-8")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request to %s failed: %w", endpoint, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("remote server %s responded with %d: %s", endpoint, resp.StatusCode, strings.TrimSpace(string(message)))
	}

	if out == nil {
		io.Copy(io.Discard, resp.Body)
		return nil
	}

	dec := xml.NewDecoder(resp.Body)
	if err := dec.Decode(out); err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("failed to decode XML response: %w", err)
	}

	return nil
}


