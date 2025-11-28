package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"

	_ "github.com/mattn/go-sqlite3"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("Использование: check_normalized_data <путь_к_базе.db>")
	}

	dbPath := os.Args[1]

	// Открываем базу данных
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		log.Fatalf("Ошибка открытия базы данных: %v", err)
	}
	defer db.Close()

	// Проверяем существование таблицы normalized_data
	var tableExists bool
	err = db.QueryRow(`
		SELECT COUNT(*) > 0 
		FROM sqlite_master 
		WHERE type='table' AND name='normalized_data'
	`).Scan(&tableExists)
	if err != nil {
		log.Fatalf("Ошибка проверки таблицы: %v", err)
	}

	if !tableExists {
		fmt.Printf("❌ Таблица normalized_data не найдена в %s\n", dbPath)
		return
	}

	fmt.Printf("✅ Таблица normalized_data найдена в %s\n\n", dbPath)

	// Подсчитываем количество записей
	var total int
	err = db.QueryRow("SELECT COUNT(*) FROM normalized_data").Scan(&total)
	if err != nil {
		log.Fatalf("Ошибка подсчета записей: %v", err)
	}
	fmt.Printf("📊 Всего записей: %d\n\n", total)

	// Показываем примеры записей
	rows, err := db.Query(`
		SELECT id, source_name, normalized_name, category, kpved_code, kpved_name
		FROM normalized_data
		WHERE source_name IS NOT NULL AND source_name != ''
		LIMIT 10
	`)
	if err != nil {
		log.Fatalf("Ошибка запроса: %v", err)
	}
	defer rows.Close()

	fmt.Println("📋 Примеры записей:")
	fmt.Println("─────────────────────────────────────────────────────────────")
	for rows.Next() {
		var id int
		var sourceName, normalizedName, category sql.NullString
		var kpvedCode, kpvedName sql.NullString

		err := rows.Scan(&id, &sourceName, &normalizedName, &category, &kpvedCode, &kpvedName)
		if err != nil {
			log.Printf("Ошибка сканирования: %v", err)
			continue
		}

		fmt.Printf("ID: %d\n", id)
		if sourceName.Valid {
			fmt.Printf("  Source: %s\n", sourceName.String)
		}
		if normalizedName.Valid {
			fmt.Printf("  Normalized: %s\n", normalizedName.String)
		}
		if category.Valid {
			fmt.Printf("  Category: %s\n", category.String)
		}
		if kpvedCode.Valid {
			fmt.Printf("  KPVED Code: %s\n", kpvedCode.String)
		}
		if kpvedName.Valid {
			fmt.Printf("  KPVED Name: %s\n", kpvedName.String)
		}
		fmt.Println()
	}

	// Проверяем наличие классификаторов
	var classifierCount int
	err = db.QueryRow(`
		SELECT COUNT(*) 
		FROM sqlite_master 
		WHERE type='table' AND name='category_classifiers'
	`).Scan(&classifierCount)
	if err == nil && classifierCount > 0 {
		var activeClassifiers int
		err = db.QueryRow("SELECT COUNT(*) FROM category_classifiers WHERE is_active = 1").Scan(&activeClassifiers)
		if err == nil {
			fmt.Printf("📚 Активных классификаторов: %d\n", activeClassifiers)
		}
	}
}

