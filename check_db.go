package main

import (
	"database/sql"
	"fmt"
	"log"

	_ "github.com/mattn/go-sqlite3"
)

func main() {
	db, err := sql.Open("sqlite3", "1c_data.db")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	fmt.Println("=== ПРОВЕРКА БАЗЫ ДАННЫХ ===\n")

	// Проверка catalog_items
	var catalogItemsCount int
	db.QueryRow("SELECT COUNT(*) FROM catalog_items").Scan(&catalogItemsCount)
	fmt.Printf("Элементов в catalog_items: %d\n", catalogItemsCount)

	// Проверка normalized_data
	var normalizedCount int
	db.QueryRow("SELECT COUNT(*) FROM normalized_data").Scan(&normalizedCount)
	fmt.Printf("Элементов в normalized_data: %d\n", normalizedCount)

	// Показать несколько элементов из catalog_items
	rows, err := db.Query("SELECT id, code, name FROM catalog_items LIMIT 10")
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	fmt.Println("\nПервые 10 записей из catalog_items:")
	for rows.Next() {
		var id int
		var code, name string
		rows.Scan(&id, &code, &name)
		fmt.Printf("  %d: [%s] %s\n", id, code, name)
	}

	// Показать несколько элементов из normalized_data
	rows2, err := db.Query("SELECT id, source_name, normalized_name, merged_count FROM normalized_data LIMIT 10")
	if err != nil {
		log.Fatal(err)
	}
	defer rows2.Close()

	fmt.Println("\nПервые 10 записей из normalized_data:")
	for rows2.Next() {
		var id, mergedCount int
		var sourceName, normalizedName string
		rows2.Scan(&id, &sourceName, &normalizedName, &mergedCount)
		fmt.Printf("  %d: %s → %s (merged: %d)\n", id, sourceName, normalizedName, mergedCount)
	}
}
