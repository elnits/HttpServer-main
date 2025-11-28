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
		log.Fatalf("Error: %v", err)
	}
	defer db.Close()
	
	fmt.Println("Проверка структуры data/1c_data.db:")
	fmt.Println("\nТаблицы:")
	rows, _ := db.Query("SELECT name FROM sqlite_master WHERE type='table'")
	for rows.Next() {
		var name string
		rows.Scan(&name)
		fmt.Printf("  - %s\n", name)
	}
	rows.Close()
	
	fmt.Println("\nКолонки normalized_data:")
	rows2, err := db.Query("PRAGMA table_info(normalized_data)")
	if err != nil {
		fmt.Printf("  Таблица не существует: %v\n", err)
		return
	}
	for rows2.Next() {
		var cid int
		var name, dtype string
		var notnull, pk int
		var dflt interface{}
		rows2.Scan(&cid, &name, &dtype, &notnull, &dflt, &pk)
		fmt.Printf("  - %s (%s)\n", name, dtype)
	}
	rows2.Close()
}
