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
		log.Fatalf("Error: %v", err)
	}
	defer db.Close()
	
	fmt.Println("Проверка data/normalized_data.db:")
	rows, err := db.Query("PRAGMA table_info(normalized_data)")
	if err != nil {
		fmt.Printf("  Ошибка: %v\n", err)
		return
	}
	fmt.Println("\nКолонки normalized_data:")
	for rows.Next() {
		var cid int
		var name, dtype string
		var notnull, pk int
		var dflt interface{}
		rows.Scan(&cid, &name, &dtype, &notnull, &dflt, &pk)
		fmt.Printf("  - %s (%s)\n", name, dtype)
	}
	rows.Close()
}
