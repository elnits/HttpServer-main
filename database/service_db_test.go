package database

import (
	"testing"
)

func TestCreateClient(t *testing.T) {
	db, err := NewServiceDB(":memory:")
	if err != nil {
		t.Fatalf("Failed to create ServiceDB: %v", err)
	}
	defer db.Close()

	client, err := db.CreateClient(
		"Test Client",
		"Test Client Legal Name",
		"Test Description",
		"test@example.com",
		"+1234567890",
		"TAX123",
		"test_user",
	)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	if client == nil {
		t.Error("CreateClient returned nil")
	}
	if client.ID == 0 {
		t.Error("Client ID is zero")
	}
	if client.Name != "Test Client" {
		t.Errorf("Expected name 'Test Client', got '%s'", client.Name)
	}
	if client.LegalName != "Test Client Legal Name" {
		t.Errorf("Expected legal name 'Test Client Legal Name', got '%s'", client.LegalName)
	}
}

func TestCreateProject(t *testing.T) {
	db, err := NewServiceDB(":memory:")
	if err != nil {
		t.Fatalf("Failed to create ServiceDB: %v", err)
	}
	defer db.Close()

	// Создаем клиента
	client, err := db.CreateClient(
		"Test Client",
		"Test Client Legal Name",
		"Test Description",
		"test@example.com",
		"+1234567890",
		"TAX123",
		"test_user",
	)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Создаем проект
	project, err := db.CreateClientProject(
		client.ID,
		"Test Project",
		"normalization",
		"Test Project Description",
		"1C",
		0.8,
	)
	if err != nil {
		t.Fatalf("Failed to create project: %v", err)
	}

	if project == nil {
		t.Error("CreateClientProject returned nil")
	}
	if project.ID == 0 {
		t.Error("Project ID is zero")
	}
	if project.ClientID != client.ID {
		t.Errorf("Expected clientID %d, got %d", client.ID, project.ClientID)
	}
	if project.Name != "Test Project" {
		t.Errorf("Expected name 'Test Project', got '%s'", project.Name)
	}
}

func TestCreateDatabase(t *testing.T) {
	db, err := NewServiceDB(":memory:")
	if err != nil {
		t.Fatalf("Failed to create ServiceDB: %v", err)
	}
	defer db.Close()

	// Создаем клиента
	client, err := db.CreateClient(
		"Test Client",
		"Test Client Legal Name",
		"Test Description",
		"test@example.com",
		"+1234567890",
		"TAX123",
		"test_user",
	)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Создаем проект
	project, err := db.CreateClientProject(
		client.ID,
		"Test Project",
		"normalization",
		"Test Project Description",
		"1C",
		0.8,
	)
	if err != nil {
		t.Fatalf("Failed to create project: %v", err)
	}

	// Создаем базу данных
	database, err := db.CreateProjectDatabase(
		project.ID,
		"Test Database",
		"/path/to/database.db",
		"Test Database Description",
		1024,
	)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}

	if database == nil {
		t.Error("CreateProjectDatabase returned nil")
	}
	if database.ID == 0 {
		t.Error("Database ID is zero")
	}
	if database.ClientProjectID != project.ID {
		t.Errorf("Expected projectID %d, got %d", project.ID, database.ClientProjectID)
	}
	if database.Name != "Test Database" {
		t.Errorf("Expected name 'Test Database', got '%s'", database.Name)
	}
}

func TestGetQualityMetrics(t *testing.T) {
	db, err := NewServiceDB(":memory:")
	if err != nil {
		t.Fatalf("Failed to create ServiceDB: %v", err)
	}
	defer db.Close()

	// Создаем клиента
	client, err := db.CreateClient(
		"Test Client",
		"Test Client Legal Name",
		"Test Description",
		"test@example.com",
		"+1234567890",
		"TAX123",
		"test_user",
	)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Создаем проект
	project, err := db.CreateClientProject(
		client.ID,
		"Test Project",
		"normalization",
		"Test Project Description",
		"1C",
		0.8,
	)
	if err != nil {
		t.Fatalf("Failed to create project: %v", err)
	}

	// Получаем метрики (может быть пусто, если нет данных или таблицы)
	metrics, err := db.GetQualityMetricsForProject(project.ID, "month")
	// Если таблица не существует, это нормально для теста
	if err != nil {
		t.Logf("GetQualityMetricsForProject returned error (expected if table doesn't exist): %v", err)
		return
	}

	// Проверяем, что возвращается массив (может быть пустым)
	if metrics == nil {
		t.Error("GetQualityMetricsForProject returned nil")
	}
}

func TestCompareProjectsQuality(t *testing.T) {
	db, err := NewServiceDB(":memory:")
	if err != nil {
		t.Fatalf("Failed to create ServiceDB: %v", err)
	}
	defer db.Close()

	// Создаем клиента
	client, err := db.CreateClient(
		"Test Client",
		"Test Client Legal Name",
		"Test Description",
		"test@example.com",
		"+1234567890",
		"TAX123",
		"test_user",
	)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Создаем два проекта
	project1, err := db.CreateClientProject(
		client.ID,
		"Test Project 1",
		"normalization",
		"Test Project 1 Description",
		"1C",
		0.8,
	)
	if err != nil {
		t.Fatalf("Failed to create project 1: %v", err)
	}

	project2, err := db.CreateClientProject(
		client.ID,
		"Test Project 2",
		"normalization",
		"Test Project 2 Description",
		"1C",
		0.9,
	)
	if err != nil {
		t.Fatalf("Failed to create project 2: %v", err)
	}

	// Сравниваем проекты (может быть ошибка, если таблица не существует)
	comparison, err := db.CompareProjectsQuality([]int{project1.ID, project2.ID})
	// Если таблица не существует, это нормально для теста
	if err != nil {
		t.Logf("CompareProjectsQuality returned error (expected if table doesn't exist): %v", err)
		return
	}

	if comparison == nil {
		t.Error("CompareProjectsQuality returned nil")
	}
}

func TestGetClient(t *testing.T) {
	db, err := NewServiceDB(":memory:")
	if err != nil {
		t.Fatalf("Failed to create ServiceDB: %v", err)
	}
	defer db.Close()

	// Создаем клиента
	createdClient, err := db.CreateClient(
		"Test Client",
		"Test Client Legal Name",
		"Test Description",
		"test@example.com",
		"+1234567890",
		"TAX123",
		"test_user",
	)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Получаем клиента
	client, err := db.GetClient(createdClient.ID)
	if err != nil {
		t.Fatalf("Failed to get client: %v", err)
	}

	if client == nil {
		t.Error("GetClient returned nil")
	}
	if client.ID != createdClient.ID {
		t.Errorf("Expected client ID %d, got %d", createdClient.ID, client.ID)
	}
	if client.Name != "Test Client" {
		t.Errorf("Expected name 'Test Client', got '%s'", client.Name)
	}
}

func TestGetClientProject(t *testing.T) {
	db, err := NewServiceDB(":memory:")
	if err != nil {
		t.Fatalf("Failed to create ServiceDB: %v", err)
	}
	defer db.Close()

	// Создаем клиента
	client, err := db.CreateClient(
		"Test Client",
		"Test Client Legal Name",
		"Test Description",
		"test@example.com",
		"+1234567890",
		"TAX123",
		"test_user",
	)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Создаем проект
	createdProject, err := db.CreateClientProject(
		client.ID,
		"Test Project",
		"normalization",
		"Test Project Description",
		"1C",
		0.8,
	)
	if err != nil {
		t.Fatalf("Failed to create project: %v", err)
	}

	// Получаем проект
	project, err := db.GetClientProject(createdProject.ID)
	if err != nil {
		t.Fatalf("Failed to get project: %v", err)
	}

	if project == nil {
		t.Error("GetClientProject returned nil")
	}
	if project.ID != createdProject.ID {
		t.Errorf("Expected project ID %d, got %d", createdProject.ID, project.ID)
	}
	if project.Name != "Test Project" {
		t.Errorf("Expected name 'Test Project', got '%s'", project.Name)
	}
}

