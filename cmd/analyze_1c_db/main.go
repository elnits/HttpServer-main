package main

import (
	"database/sql"
	"encoding/xml"
	"fmt"
	"log"
	"os"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

type UploadInfo struct {
	ID             int
	UploadUUID     string
	StartedAt      string
	CompletedAt    sql.NullString
	Status         string
	Version1C      sql.NullString
	ConfigName     sql.NullString
	TotalConstants int
	TotalCatalogs  int
	TotalItems     int
}

type ConstantInfo struct {
	Name    string
	Synonym sql.NullString
	Type    sql.NullString
	Value   sql.NullString
}

type CatalogInfo struct {
	Name    string
	Synonym sql.NullString
}

type CatalogItemInfo struct {
	Reference      string
	Code           sql.NullString
	Name           sql.NullString
	AttributesXML  sql.NullString
	TablePartsXML  sql.NullString
	CatalogName    string
}

type XMLAttributes struct {
	XMLName xml.Name `xml:"Attributes"`
	Attrs   []Attr   `xml:"Attr"`
}

type Attr struct {
	Name  string `xml:"Name,attr"`
	Value string `xml:"Value,attr"`
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Использование: analyze_1c_db <путь_к_базе.db>")
		os.Exit(1)
	}

	dbPath := os.Args[1]
	
	// Проверяем существование файла
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		log.Fatalf("Файл базы данных не найден: %s", dbPath)
	}

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		log.Fatalf("Ошибка открытия базы данных: %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatalf("Ошибка подключения к базе данных: %v", err)
	}

	fmt.Println("=" + strings.Repeat("=", 80))
	fmt.Println("АНАЛИЗ БАЗЫ ДАННЫХ 1С НА НАЛИЧИЕ СИСТЕМНОЙ ИНФОРМАЦИИ")
	fmt.Println("=" + strings.Repeat("=", 80))
	fmt.Printf("База данных: %s\n\n", dbPath)

	// Анализ таблицы uploads
	analyzeUploads(db)

	// Анализ констант
	analyzeConstants(db)

	// Анализ справочников
	analyzeCatalogs(db)

	// Анализ элементов справочников на наличие системной информации в XML
	analyzeCatalogItems(db)

	fmt.Println("\n" + strings.Repeat("=", 80))
	fmt.Println("АНАЛИЗ ЗАВЕРШЕН")
	fmt.Println(strings.Repeat("=", 80))
}

func analyzeUploads(db *sql.DB) {
	fmt.Println("\n[1] АНАЛИЗ ТАБЛИЦЫ UPLOADS (Выгрузки из 1С)")
	fmt.Println(strings.Repeat("-", 80))

	query := `
		SELECT 
			id, upload_uuid, started_at, completed_at, status,
			version_1c, config_name, total_constants, total_catalogs, total_items
		FROM uploads
		ORDER BY started_at DESC
	`

	rows, err := db.Query(query)
	if err != nil {
		log.Printf("Ошибка запроса uploads: %v", err)
		return
	}
	defer rows.Close()

	var uploads []UploadInfo
	for rows.Next() {
		var u UploadInfo
		err := rows.Scan(
			&u.ID, &u.UploadUUID, &u.StartedAt, &u.CompletedAt, &u.Status,
			&u.Version1C, &u.ConfigName, &u.TotalConstants, &u.TotalCatalogs, &u.TotalItems,
		)
		if err != nil {
			log.Printf("Ошибка сканирования upload: %v", err)
			continue
		}
		uploads = append(uploads, u)
	}

	if len(uploads) == 0 {
		fmt.Println("  Таблица uploads пуста")
		return
	}

	fmt.Printf("  Найдено выгрузок: %d\n\n", len(uploads))

	for i, u := range uploads {
		fmt.Printf("  Выгрузка #%d (ID: %d)\n", i+1, u.ID)
		fmt.Printf("    UUID: %s\n", u.UploadUUID)
		fmt.Printf("    Начало: %s\n", u.StartedAt)
		if u.CompletedAt.Valid {
			fmt.Printf("    Завершение: %s\n", u.CompletedAt.String)
		}
		fmt.Printf("    Статус: %s\n", u.Status)
		
		// Системная информация
		if u.Version1C.Valid && u.Version1C.String != "" {
			fmt.Printf("    ⚙️  Версия 1С: %s\n", u.Version1C.String)
		}
		if u.ConfigName.Valid && u.ConfigName.String != "" {
			fmt.Printf("    ⚙️  Имя конфигурации: %s\n", u.ConfigName.String)
		}
		
		fmt.Printf("    Статистика: Константы=%d, Справочники=%d, Элементы=%d\n", 
			u.TotalConstants, u.TotalCatalogs, u.TotalItems)
		fmt.Println()
	}
}

func analyzeConstants(db *sql.DB) {
	fmt.Println("\n[2] АНАЛИЗ КОНСТАНТ (Возможные системные параметры)")
	fmt.Println(strings.Repeat("-", 80))

	// Сначала проверим, есть ли вообще константы
	var totalConstants int
	err := db.QueryRow("SELECT COUNT(*) FROM constants").Scan(&totalConstants)
	if err != nil {
		log.Printf("Ошибка подсчета констант: %v", err)
		return
	}

	if totalConstants == 0 {
		fmt.Println("  Константы в базе данных отсутствуют")
		return
	}

	fmt.Printf("  Всего констант в базе: %d\n\n", totalConstants)

	// Поиск системных констант по ключевым словам
	query := `
		SELECT DISTINCT c.name, c.synonym, c.type, c.value
		FROM constants c
		JOIN uploads u ON c.upload_id = u.id
		WHERE LOWER(c.name) LIKE '%версия%' 
		   OR LOWER(c.name) LIKE '%конфигурация%'
		   OR LOWER(c.name) LIKE '%база%'
		   OR LOWER(c.name) LIKE '%путь%'
		   OR LOWER(c.name) LIKE '%сервер%'
		   OR LOWER(c.name) LIKE '%информационная%'
		   OR LOWER(c.name) LIKE '%расположение%'
		   OR LOWER(c.name) LIKE '%адрес%'
		   OR LOWER(c.name) LIKE '%путькбазе%'
		   OR LOWER(c.name) LIKE '%путькфайлу%'
		   OR LOWER(c.synonym) LIKE '%версия%'
		   OR LOWER(c.synonym) LIKE '%конфигурация%'
		   OR LOWER(c.synonym) LIKE '%база%'
		   OR LOWER(c.synonym) LIKE '%путь%'
		   OR LOWER(c.synonym) LIKE '%сервер%'
		ORDER BY c.name
	`

	rows, err := db.Query(query)
	if err != nil {
		log.Printf("Ошибка запроса constants: %v", err)
		return
	}
	defer rows.Close()

	var constants []ConstantInfo
	for rows.Next() {
		var c ConstantInfo
		err := rows.Scan(&c.Name, &c.Synonym, &c.Type, &c.Value)
		if err != nil {
			log.Printf("Ошибка сканирования constant: %v", err)
			continue
		}
		constants = append(constants, c)
	}

	if len(constants) == 0 {
		fmt.Println("  Системные константы по ключевым словам не найдены")
		fmt.Println("\n  [2.1] Показываем все константы (первые 50)...")
		
		// Показываем все константы для анализа
		allQuery := `
			SELECT DISTINCT c.name, c.synonym, c.type, c.value
			FROM constants c
			ORDER BY c.name
			LIMIT 50
		`
		allRows, err := db.Query(allQuery)
		if err == nil {
			defer allRows.Close()
			count := 0
			for allRows.Next() {
				var c ConstantInfo
				if err := allRows.Scan(&c.Name, &c.Synonym, &c.Type, &c.Value); err == nil {
					count++
					fmt.Printf("  %d. %s", count, c.Name)
					if c.Synonym.Valid && c.Synonym.String != "" {
						fmt.Printf(" (%s)", c.Synonym.String)
					}
					if c.Value.Valid && c.Value.String != "" {
						value := c.Value.String
						if len(value) > 100 {
							value = value[:100] + "..."
						}
						fmt.Printf(" = %s", value)
					}
					fmt.Println()
				}
			}
			if count == 0 {
				fmt.Println("  Константы не найдены")
			}
		}
		return
	}

	fmt.Printf("  Найдено потенциально системных констант: %d\n\n", len(constants))

	for _, c := range constants {
		fmt.Printf("  Константа: %s\n", c.Name)
		if c.Synonym.Valid && c.Synonym.String != "" {
			fmt.Printf("    Синоним: %s\n", c.Synonym.String)
		}
		if c.Type.Valid && c.Type.String != "" {
			fmt.Printf("    Тип: %s\n", c.Type.String)
		}
		if c.Value.Valid && c.Value.String != "" {
			fmt.Printf("    ⚙️  Значение: %s\n", c.Value.String)
		}
		fmt.Println()
	}
}

func analyzeCatalogs(db *sql.DB) {
	fmt.Println("\n[3] АНАЛИЗ СПРАВОЧНИКОВ (Метаданные)")
	fmt.Println(strings.Repeat("-", 80))

	query := `
		SELECT DISTINCT c.name, c.synonym
		FROM catalogs c
		ORDER BY c.name
	`

	rows, err := db.Query(query)
	if err != nil {
		log.Printf("Ошибка запроса catalogs: %v", err)
		return
	}
	defer rows.Close()

	var catalogs []CatalogInfo
	for rows.Next() {
		var cat CatalogInfo
		err := rows.Scan(&cat.Name, &cat.Synonym)
		if err != nil {
			log.Printf("Ошибка сканирования catalog: %v", err)
			continue
		}
		catalogs = append(catalogs, cat)
	}

	if len(catalogs) == 0 {
		fmt.Println("  Справочники не найдены")
		return
	}

	fmt.Printf("  Найдено справочников: %d\n\n", len(catalogs))

	for _, cat := range catalogs {
		fmt.Printf("  Справочник: %s\n", cat.Name)
		if cat.Synonym.Valid && cat.Synonym.String != "" {
			fmt.Printf("    Синоним: %s\n", cat.Synonym.String)
		}
	}
}

func analyzeCatalogItems(db *sql.DB) {
	fmt.Println("\n[4] АНАЛИЗ ЭЛЕМЕНТОВ СПРАВОЧНИКОВ (Поиск системной информации в XML)")
	fmt.Println(strings.Repeat("-", 80))

	query := `
		SELECT 
			ci.reference, ci.code, ci.name, ci.attributes_xml, ci.table_parts_xml,
			c.name as catalog_name
		FROM catalog_items ci
		JOIN catalogs c ON ci.catalog_id = c.id
		WHERE ci.attributes_xml IS NOT NULL 
		   OR ci.table_parts_xml IS NOT NULL
		LIMIT 100
	`

	rows, err := db.Query(query)
	if err != nil {
		log.Printf("Ошибка запроса catalog_items: %v", err)
		return
	}
	defer rows.Close()

	var items []CatalogItemInfo
	for rows.Next() {
		var item CatalogItemInfo
		err := rows.Scan(
			&item.Reference, &item.Code, &item.Name, 
			&item.AttributesXML, &item.TablePartsXML, &item.CatalogName,
		)
		if err != nil {
			log.Printf("Ошибка сканирования catalog_item: %v", err)
			continue
		}
		items = append(items, item)
	}

	if len(items) == 0 {
		fmt.Println("  Элементы с XML данными не найдены")
		return
	}

	fmt.Printf("  Найдено элементов с XML: %d (показано до 100)\n\n", len(items))

	systemKeywords := []string{
		"версия", "конфигурация", "база", "путь", "сервер", 
		"информационная", "расположение", "адрес", "путькбазе",
		"путькфайлу", "путькфайлубазы", "путькбазеданных",
	}

	foundSystemInfo := false

	for i, item := range items {
		if i >= 10 { // Показываем только первые 10 для примера
			break
		}

		hasSystemInfo := false
		var systemAttrs []string

		// Анализ attributes_xml
		if item.AttributesXML.Valid && item.AttributesXML.String != "" {
			xmlStr := item.AttributesXML.String
			for _, keyword := range systemKeywords {
				if strings.Contains(strings.ToLower(xmlStr), keyword) {
					hasSystemInfo = true
					systemAttrs = append(systemAttrs, keyword)
					break
				}
			}
		}

		// Анализ table_parts_xml
		if item.TablePartsXML.Valid && item.TablePartsXML.String != "" {
			xmlStr := item.TablePartsXML.String
			for _, keyword := range systemKeywords {
				if strings.Contains(strings.ToLower(xmlStr), keyword) {
					hasSystemInfo = true
					systemAttrs = append(systemAttrs, keyword)
					break
				}
			}
		}

		if hasSystemInfo {
			foundSystemInfo = true
			fmt.Printf("  Элемент: %s (Справочник: %s)\n", 
				item.Name.String, item.CatalogName)
			fmt.Printf("    Ссылка: %s\n", item.Reference)
			if item.Code.Valid {
				fmt.Printf("    Код: %s\n", item.Code.String)
			}
			fmt.Printf("    ⚙️  Найдены системные атрибуты: %s\n", strings.Join(systemAttrs, ", "))
			fmt.Println()
		}
	}

	if !foundSystemInfo {
		fmt.Println("  Системная информация в XML атрибутах не обнаружена")
	}

	// Дополнительный анализ: поиск всех уникальных имен атрибутов
	fmt.Println("\n  [4.1] Анализ уникальных имен атрибутов в XML...")
	analyzeXMLAttributes(db)
}

func analyzeXMLAttributes(db *sql.DB) {
	query := `
		SELECT DISTINCT ci.attributes_xml
		FROM catalog_items ci
		WHERE ci.attributes_xml IS NOT NULL 
		  AND ci.attributes_xml != ''
		LIMIT 50
	`

	rows, err := db.Query(query)
	if err != nil {
		log.Printf("Ошибка запроса для анализа XML: %v", err)
		return
	}
	defer rows.Close()

	attributeNames := make(map[string]int)

	for rows.Next() {
		var xmlStr sql.NullString
		if err := rows.Scan(&xmlStr); err != nil {
			continue
		}

		if !xmlStr.Valid {
			continue
		}

		// Простой парсинг XML для поиска имен атрибутов
		// Ищем паттерны Name="..."
		xmlContent := xmlStr.String
		lines := strings.Split(xmlContent, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.Contains(line, `Name="`) {
				start := strings.Index(line, `Name="`)
				if start != -1 {
					start += 6 // Длина `Name="`
					end := strings.Index(line[start:], `"`)
					if end != -1 {
						attrName := line[start : start+end]
						attrNameLower := strings.ToLower(attrName)
						
						// Проверяем на системные ключевые слова
						systemKeywords := []string{
							"версия", "конфигурация", "база", "путь", "сервер",
							"информационная", "расположение", "адрес", "путькбазе",
							"путькфайлу", "путькфайлубазы", "путькбазеданных",
						}
						
						for _, keyword := range systemKeywords {
							if strings.Contains(attrNameLower, keyword) {
								attributeNames[attrName]++
								break
							}
						}
					}
				}
			}
		}
	}

	if len(attributeNames) > 0 {
		fmt.Printf("  Найдено системных атрибутов: %d\n", len(attributeNames))
		for attrName, count := range attributeNames {
			fmt.Printf("    ⚙️  %s (встречается %d раз)\n", attrName, count)
		}
	} else {
		fmt.Println("  Системные атрибуты в XML не найдены")
	}
}

