package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
)

// handleGetClassifiers возвращает список всех классификаторов
func (s *Server) handleGetClassifiers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Получаем параметры фильтрации
	query := r.URL.Query()
	activeOnly := query.Get("active_only") == "true"
	clientIDStr := query.Get("client_id")
	projectIDStr := query.Get("project_id")

	var clientID *int
	var projectID *int

	if clientIDStr != "" {
		if id, err := strconv.Atoi(clientIDStr); err == nil {
			clientID = &id
		}
	}

	if projectIDStr != "" {
		if id, err := strconv.Atoi(projectIDStr); err == nil {
			projectID = &id
		}
	}

	// Получаем классификаторы с фильтрацией
	classifiers, err := s.db.GetCategoryClassifiersByFilter(clientID, projectID, activeOnly)
	if err != nil {
		s.writeJSONError(w, fmt.Sprintf("Failed to get classifiers: %v", err), http.StatusInternalServerError)
		return
	}

	// Преобразуем в формат для фронтенда
	type ClassifierResponse struct {
		ID          int    `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
		IsActive    bool   `json:"is_active"`
		MaxDepth    int    `json:"max_depth"`
	}

	response := make([]ClassifierResponse, 0, len(classifiers))
	for _, classifier := range classifiers {
		response = append(response, ClassifierResponse{
			ID:          classifier.ID,
			Name:        classifier.Name,
			Description: classifier.Description,
			IsActive:    classifier.IsActive,
			MaxDepth:    classifier.MaxDepth,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

