package main

import (
	"database/sql"
	"fmt"
	"log"
	_ "github.com/mattn/go-sqlite3"
)

func main() {
	db, err := sql.Open("sqlite3", "data/1c_data.db")
	if err != nil {
		log.Fatalf("Ошибка открытия БД: %v", err)
	}
	defer db.Close()

	fmt.Println("Добавление недостающих колонок в normalized_data...")

	// Проверяем текущие колонки
	rows, err := db.Query("PRAGMA table_info(normalized_data)")
	if err != nil {
		log.Fatalf("Ошибка получения структуры: %v", err)
	}
	
	columns := make(map[string]bool)
	for rows.Next() {
		var cid int
		var name, dtype string
		var notnull, pk int
		var dflt interface{}
		rows.Scan(&cid, &name, &dtype, &notnull, &dflt, &pk)
		columns[name] = true
		fmt.Printf("  Найдена колонка: %s (%s)\n", name, dtype)
	}
	rows.Close()

	// Добавляем недостающие колонки
	if !columns["quality_score"] {
		fmt.Println("\nДобавление quality_score...")
		_, err = db.Exec("ALTER TABLE normalized_data ADD COLUMN quality_score REAL DEFAULT 0.0")
		if err != nil {
			log.Fatalf("Ошибка добавления quality_score: %v", err)
		}
		fmt.Println("✓ quality_score добавлена")
	} else {
		fmt.Println("\n✓ quality_score уже существует")
	}

	if !columns["validation_status"] {
		fmt.Println("\nДобавление validation_status...")
		_, err = db.Exec("ALTER TABLE normalized_data ADD COLUMN validation_status TEXT DEFAULT ''")
		if err != nil {
			log.Fatalf("Ошибка добавления validation_status: %v", err)
		}
		fmt.Println("✓ validation_status добавлена")
	} else {
		fmt.Println("\n✓ validation_status уже существует")
	}

	fmt.Println("\n✓ Схема БД обновлена!")
}
