package main

import (
	"database/sql"
	"fmt"

	_ "github.com/mattn/go-sqlite3"
)

func main() {
	db, _ := sql.Open("sqlite3", "1c_data.db")
	defer db.Close()

	rows, _ := db.Query(`
		SELECT id, source_name, normalized_name, merged_count
		FROM normalized_data
		WHERE LOWER(source_name) LIKE '%болт%'
		   OR LOWER(source_name) LIKE '%гайка%'
		   OR LOWER(source_name) LIKE '%шайба%'
		ORDER BY id DESC
		LIMIT 20
	`)
	defer rows.Close()

	count := 0
	fmt.Println("Записи содержащие наши тестовые элементы (Болт/Гайка/Шайба):")
	for rows.Next() {
		var id, merged int
		var source, norm string
		rows.Scan(&id, &source, &norm, &merged)
		fmt.Printf("  %d: %s → %s (merged: %d)\n", id, source, norm, merged)
		count++
	}

	if count == 0 {
		fmt.Println("  (не найдено)")
	}
}
