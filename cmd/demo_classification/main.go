package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	"httpserver/classification"
	"httpserver/database"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Использование: demo_classification <путь_к_базе.db> [limit]")
		fmt.Println("Пример: demo_classification 1c_data.db 10")
		os.Exit(1)
	}

	dbPath := os.Args[1]
	limit := 10
	if len(os.Args) >= 3 {
		fmt.Sscanf(os.Args[2], "%d", &limit)
	}

	db, err := database.NewDB(dbPath)
	if err != nil {
		log.Fatalf("Ошибка подключения: %v", err)
	}
	defer db.Close()

	// Получаем классификатор
	classifier, err := db.GetCategoryClassifier(1)
	if err != nil {
		log.Fatalf("Ошибка получения классификатора: %v", err)
	}

	fmt.Println("=== Демонстрация классификации по КПВЭД ===")
	fmt.Printf("Классификатор: %s\n", classifier.Name)
	fmt.Printf("Максимальная глубина: %d\n\n", classifier.MaxDepth)

	// Получаем примеры элементов
	query := `
		SELECT id, code, name
		FROM catalog_items
		WHERE name IS NOT NULL AND name != ''
		ORDER BY id
		LIMIT ?
	`

	rows, err := db.Query(query, limit)
	if err != nil {
		log.Fatalf("Ошибка запроса: %v", err)
	}
	defer rows.Close()

	type Item struct {
		ID   int
		Code string
		Name string
	}

	var items []Item
	for rows.Next() {
		var item Item
		if err := rows.Scan(&item.ID, &item.Code, &item.Name); err != nil {
			continue
		}
		items = append(items, item)
	}

	fmt.Printf("Примеры элементов для классификации (%d шт.):\n\n", len(items))

	// Парсим дерево классификатора для демонстрации
	var classifierTree classification.CategoryNode
	if err := json.Unmarshal([]byte(classifier.TreeStructure), &classifierTree); err == nil {
		fmt.Println("Структура классификатора КПВЭД:")
		fmt.Printf("  Корневых категорий: %d\n", len(classifierTree.Children))
		if len(classifierTree.Children) > 0 {
			fmt.Println("  Примеры корневых категорий:")
			for i, child := range classifierTree.Children {
				if i >= 5 {
					break
				}
				fmt.Printf("    - %s (ID: %s)\n", child.Name, child.ID)
			}
		}
		fmt.Println()
	}

	// Показываем примеры элементов
	for i, item := range items {
		fmt.Printf("--- Пример #%d ---\n", i+1)
		fmt.Printf("ID: %d\n", item.ID)
		fmt.Printf("Код: %s\n", item.Code)
		fmt.Printf("Название: %s\n", item.Name)

		// Показываем примерную классификацию на основе ключевых слов
		suggestedCategory := suggestCategory(item.Name)
		if suggestedCategory != nil {
			fmt.Printf("Предполагаемая категория КПВЭД:\n")
			for j, level := range suggestedCategory {
				fmt.Printf("  Уровень %d: %s\n", j+1, level)
			}
		} else {
			fmt.Printf("Категория: (требуется AI классификация)\n")
		}
		fmt.Println()
	}

	fmt.Println("=== Информация ===")
	fmt.Println("Для реальной классификации с использованием AI:")
	fmt.Println("  1. Убедитесь, что установлен ARLIAI_API_KEY")
	fmt.Println("  2. Запустите: go run cmd/classify_catalog_items/main.go 1c_data.db 1 top_priority")
	fmt.Println("\nПримечание: Классификация 15973 элементов займет значительное время")
	fmt.Println("Рекомендуется запускать в фоновом режиме или использовать лимит:")
	fmt.Println("  go run cmd/classify_catalog_items/main.go 1c_data.db 1 top_priority 100")
}

// suggestCategory предлагает категорию на основе ключевых слов (для демонстрации)
func suggestCategory(name string) []string {
	nameLower := name
	
	// Простые правила для демонстрации
	if contains(nameLower, "строитель", "материал", "обои", "клей") {
		return []string{"C", "ПРОДУКЦИЯ ОБРАБАТЫВАЮЩЕЙ ПРОМЫШЛЕННОСТИ", "Строительные материалы"}
	}
	if contains(nameLower, "ламп", "свет", "люстр", "освещени") {
		return []string{"C", "ПРОДУКЦИЯ ОБРАБАТЫВАЮЩЕЙ ПРОМЫШЛЕННОСТИ", "Осветительное оборудование"}
	}
	if contains(nameLower, "бумаг", "блок", "ежедневник", "визитниц", "папк") {
		return []string{"C", "ПРОДУКЦИЯ ОБРАБАТЫВАЮЩЕЙ ПРОМЫШЛЕННОСТИ", "Бумажная продукция"}
	}
	if contains(nameLower, "ванн", "сантехник") {
		return []string{"C", "ПРОДУКЦИЯ ОБРАБАТЫВАЮЩЕЙ ПРОМЫШЛЕННОСТИ", "Сантехническое оборудование"}
	}
	if contains(nameLower, "программ", "autodesk", "software") {
		return []string{"J", "ИНФОРМАЦИЯ И СВЯЗЬ", "Программное обеспечение"}
	}
	if contains(nameLower, "ручк", "линейк", "канцеляр") {
		return []string{"C", "ПРОДУКЦИЯ ОБРАБАТЫВАЮЩЕЙ ПРОМЫШЛЕННОСТИ", "Канцелярские товары"}
	}

	return nil
}

func contains(s string, substrs ...string) bool {
	for _, substr := range substrs {
		if len(s) >= len(substr) {
			for i := 0; i <= len(s)-len(substr); i++ {
				if i+len(substr) <= len(s) && s[i:i+len(substr)] == substr {
					return true
				}
			}
		}
	}
	return false
}

