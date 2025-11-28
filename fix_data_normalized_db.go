package main
import (
	"database/sql"
	"fmt"
	"log"
	_ "github.com/mattn/go-sqlite3"
)
func main() {
	db, err := sql.Open("sqlite3", "data/normalized_data.db")
	if err != nil {
		log.Fatalf("Ошибка: %v", err)
	}
	defer db.Close()

	fmt.Println("Обновление data/normalized_data.db...")

	rows, err := db.Query("PRAGMA table_info(normalized_data)")
	if err != nil {
		log.Fatalf("Ошибка: %v", err)
	}
	
	columns := make(map[string]bool)
	for rows.Next() {
		var cid int
		var name, dtype string
		var notnull, pk int
		var dflt interface{}
		rows.Scan(&cid, &name, &dtype, &notnull, &dflt, &pk)
		columns[name] = true
	}
	rows.Close()

	if !columns["quality_score"] {
		fmt.Println("Добавление quality_score...")
		_, err = db.Exec("ALTER TABLE normalized_data ADD COLUMN quality_score REAL DEFAULT 0.0")
		if err != nil {
			log.Fatalf("Ошибка: %v", err)
		}
		fmt.Println("✓ quality_score добавлена")
	} else {
		fmt.Println("✓ quality_score уже существует")
	}

	if !columns["validation_status"] {
		fmt.Println("Добавление validation_status...")
		_, err = db.Exec("ALTER TABLE normalized_data ADD COLUMN validation_status TEXT DEFAULT ''")
		if err != nil {
			log.Fatalf("Ошибка: %v", err)
		}
		fmt.Println("✓ validation_status добавлена")
	} else {
		fmt.Println("✓ validation_status уже существует")
	}

	fmt.Println("\n✓ Схема data/normalized_data.db обновлена!")
}
