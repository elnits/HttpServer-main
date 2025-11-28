package main

import (
	"fmt"
	"log"
	"os"

	"httpserver/database"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Использование: check_nomenclature <путь_к_базе.db>")
		os.Exit(1)
	}

	dbPath := os.Args[1]

	db, err := database.NewDB(dbPath)
	if err != nil {
		log.Fatalf("Ошибка подключения: %v", err)
	}
	defer db.Close()

	// Проверяем количество в nomenclature_items
	count, err := db.GetNomenclatureItemsCount()
	if err != nil {
		log.Printf("Ошибка получения количества: %v", err)
	} else {
		fmt.Printf("Номенклатур в nomenclature_items: %d\n", count)
	}

	// Проверяем через прямой запрос
	query := `SELECT COUNT(*) FROM nomenclature_items WHERE nomenclature_name IS NOT NULL AND nomenclature_name != ''`
	var directCount int
	row := db.QueryRow(query)
	if err := row.Scan(&directCount); err != nil {
		log.Printf("Ошибка прямого запроса: %v", err)
	} else {
		fmt.Printf("Номенклатур (прямой запрос): %d\n", directCount)
	}

	// Проверяем catalog_items
	query2 := `SELECT COUNT(*) FROM catalog_items WHERE name IS NOT NULL AND name != ''`
	var catalogCount int
	row2 := db.QueryRow(query2)
	if err := row2.Scan(&catalogCount); err != nil {
		log.Printf("Ошибка запроса catalog_items: %v", err)
	} else {
		fmt.Printf("Элементов в catalog_items: %d\n", catalogCount)
	}

	// Показываем несколько примеров из nomenclature_items
	fmt.Println("\nПримеры из nomenclature_items:")
	rows, err := db.Query("SELECT id, nomenclature_code, nomenclature_name FROM nomenclature_items LIMIT 5")
	if err != nil {
		log.Printf("Ошибка выборки примеров: %v", err)
	} else {
		defer rows.Close()
		for rows.Next() {
			var id int
			var code, name string
			if err := rows.Scan(&id, &code, &name); err == nil {
				fmt.Printf("  ID: %d, Код: %s, Название: %s\n", id, code, name)
			}
		}
	}

	// Показываем несколько примеров из catalog_items
	fmt.Println("\nПримеры из catalog_items:")
	rows2, err := db.Query("SELECT id, code, name FROM catalog_items LIMIT 5")
	if err != nil {
		log.Printf("Ошибка выборки примеров: %v", err)
	} else {
		defer rows2.Close()
		for rows2.Next() {
			var id int
			var code, name string
			if err := rows2.Scan(&id, &code, &name); err == nil {
				fmt.Printf("  ID: %d, Код: %s, Название: %s\n", id, code, name)
			}
		}
	}

	// Проверяем классифицированные
	query3 := `SELECT COUNT(*) FROM nomenclature_items WHERE category_level1 IS NOT NULL AND category_level1 != ''`
	var classifiedCount int
	row3 := db.QueryRow(query3)
	if err := row3.Scan(&classifiedCount); err != nil {
		// Возможно, поля еще не созданы
		fmt.Println("\nКлассифицированных: 0 (поля категорий еще не созданы)")
	} else {
		fmt.Printf("\nКлассифицированных номенклатур: %d\n", classifiedCount)
	}
}

