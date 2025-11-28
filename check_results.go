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
		log.Fatal(err)
	}
	defer db.Close()
	
	var count int
	db.QueryRow("SELECT COUNT(*) FROM normalized_data").Scan(&count)
	fmt.Printf("Всего записей в normalized_data: %d\n", count)
	
	var withMerged int
	db.QueryRow("SELECT COUNT(*) FROM normalized_data WHERE merged_count > 0").Scan(&withMerged)
	fmt.Printf("Записей с merged_count > 0: %d\n\n", withMerged)
	
	fmt.Println("Топ 10 записей с наибольшим merged_count:")
	rows, _ := db.Query(`
		SELECT normalized_name, merged_count, created_at 
		FROM normalized_data 
		WHERE merged_count > 0
		ORDER BY merged_count DESC 
		LIMIT 10
	`)
	for rows.Next() {
		var name, created string
		var merged int
		rows.Scan(&name, &merged, &created)
		fmt.Printf("  %s: merged_count=%d (%s)\n", name, merged, created)
	}
	rows.Close()
}
