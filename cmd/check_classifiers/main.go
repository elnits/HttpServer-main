package main

import (
	"fmt"
	"log"
	"os"

	"httpserver/database"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Использование: check_classifiers <путь_к_базе.db> [client_id] [project_id]")
		fmt.Println("Пример: check_classifiers 1c_data.db")
		fmt.Println("Пример: check_classifiers 1c_data.db 1 1")
		os.Exit(1)
	}

	dbPath := os.Args[1]

	var clientID *int
	var projectID *int

	if len(os.Args) >= 3 {
		id := 0
		fmt.Sscanf(os.Args[2], "%d", &id)
		if id > 0 {
			clientID = &id
		}
	}

	if len(os.Args) >= 4 {
		id := 0
		fmt.Sscanf(os.Args[3], "%d", &id)
		if id > 0 {
			projectID = &id
		}
	}

	fmt.Printf("Проверка классификаторов в базе: %s\n", dbPath)
	if clientID != nil {
		fmt.Printf("Client ID: %d\n", *clientID)
	}
	if projectID != nil {
		fmt.Printf("Project ID: %d\n", *projectID)
	}
	fmt.Println()

	// Подключаемся к базе
	db, err := database.NewDB(dbPath)
	if err != nil {
		log.Fatalf("Ошибка подключения к базе: %v", err)
	}
	defer db.Close()

	// Получаем классификаторы
	classifiers, err := db.GetCategoryClassifiersByFilter(clientID, projectID, false)
	if err != nil {
		log.Fatalf("Ошибка получения классификаторов: %v", err)
	}

	fmt.Printf("Найдено классификаторов: %d\n\n", len(classifiers))

	if len(classifiers) == 0 {
		fmt.Println("Классификаторы не найдены!")
		if clientID != nil || projectID != nil {
			fmt.Println("\nПопробуйте загрузить классификатор КПВЭД:")
			fmt.Printf("  load_kpved КПВЭД.txt %s", dbPath)
			if clientID != nil {
				fmt.Printf(" %d", *clientID)
			} else {
				fmt.Printf(" 0")
			}
			if projectID != nil {
				fmt.Printf(" %d", *projectID)
			} else {
				fmt.Printf(" 0")
			}
			fmt.Println()
		}
		os.Exit(1)
	}

	// Выводим информацию о каждом классификаторе
	for i, classifier := range classifiers {
		fmt.Printf("=== Классификатор #%d ===\n", i+1)
		fmt.Printf("ID: %d\n", classifier.ID)
		fmt.Printf("Название: %s\n", classifier.Name)
		fmt.Printf("Описание: %s\n", classifier.Description)
		fmt.Printf("Максимальная глубина: %d\n", classifier.MaxDepth)
		fmt.Printf("Активен: %v\n", classifier.IsActive)
		if classifier.ClientID != nil {
			fmt.Printf("Client ID: %d\n", *classifier.ClientID)
		} else {
			fmt.Printf("Client ID: (глобальный)\n")
		}
		if classifier.ProjectID != nil {
			fmt.Printf("Project ID: %d\n", *classifier.ProjectID)
		} else {
			fmt.Printf("Project ID: (глобальный)\n")
		}
		fmt.Printf("Создан: %s\n", classifier.CreatedAt.Format("2006-01-02 15:04:05"))
		fmt.Printf("Обновлен: %s\n", classifier.UpdatedAt.Format("2006-01-02 15:04:05"))
		
		// Подсчитываем размер дерева
		treeSize := len(classifier.TreeStructure)
		fmt.Printf("Размер дерева: %d байт (%.2f KB)\n", treeSize, float64(treeSize)/1024)
		fmt.Println()
	}

	// Проверяем наличие КПВЭД
	kpvedFound := false
	for _, classifier := range classifiers {
		if classifier.Name == "КПВЭД" {
			kpvedFound = true
			fmt.Println("✓ Классификатор КПВЭД найден!")
			break
		}
	}

	if !kpvedFound {
		fmt.Println("⚠ Классификатор КПВЭД не найден!")
		fmt.Println("\nДля загрузки КПВЭД выполните:")
		fmt.Printf("  load_kpved КПВЭД.txt %s", dbPath)
		if clientID != nil {
			fmt.Printf(" %d", *clientID)
		}
		if projectID != nil {
			fmt.Printf(" %d", *projectID)
		}
		fmt.Println()
	}
}

