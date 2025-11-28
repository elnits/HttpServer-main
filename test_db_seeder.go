package main

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	"httpserver/database"

	_ "github.com/mattn/go-sqlite3"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	fmt.Println("================================================================================")
	fmt.Println("СЕЯЛКА ТЕСТОВЫХ ДАННЫХ ДЛЯ ДЕДУПЛИКАЦИИ")
	fmt.Println("================================================================================")

	// Открываем базу данных напрямую через SQL
	db, err := sql.Open("sqlite3", "1c_data.db")
	if err != nil {
		log.Fatalf("Ошибка открытия БД: %v", err)
	}
	defer db.Close()

	fmt.Println("\n[1/4] Инициализация схемы БД...")

	// Инициализируем схему
	if err := database.InitSchema(db); err != nil {
		log.Fatalf("Ошибка инициализации схемы: %v", err)
	}
	fmt.Println("✓ Схема инициализирована")

	fmt.Println("\n[2/4] Очистка старых данных...")

	// Очищаем таблицы
	_, err = db.Exec("DELETE FROM catalog_items")
	if err != nil {
		log.Fatalf("Ошибка очистки catalog_items: %v", err)
	}

	_, err = db.Exec("DELETE FROM catalogs")
	if err != nil {
		log.Fatalf("Ошибка очистки catalogs: %v", err)
	}

	_, err = db.Exec("DELETE FROM uploads")
	if err != nil {
		log.Fatalf("Ошибка очистки uploads: %v", err)
	}

	fmt.Println("✓ Старые данные удалены")

	fmt.Println("\n[3/4] Создание upload и справочника...")

	// Создаем upload
	result, err := db.Exec(`
		INSERT INTO uploads (upload_uuid, version_1c, config_name, config_version, computer_name, user_name, total_catalogs, total_constants, total_items, status)
		VALUES (?, ?, ?, ?, ?, ?, 0, 0, 0, 'in_progress')
	`, "test-dedup-"+time.Now().Format("20060102150405"), "8.3", "TestConfig", "1.0", "TestPC", "TestUser")

	if err != nil {
		log.Fatalf("Ошибка создания upload: %v", err)
	}

	uploadID, err := result.LastInsertId()
	if err != nil {
		log.Fatalf("Ошибка получения uploadID: %v", err)
	}

	// Создаем справочник
	result, err = db.Exec(`
		INSERT INTO catalogs (upload_id, name, synonym)
		VALUES (?, ?, ?)
	`, uploadID, "Номенклатура", "Номенклатура")

	if err != nil {
		log.Fatalf("Ошибка создания справочника: %v", err)
	}

	catalogID, err := result.LastInsertId()
	if err != nil {
		log.Fatalf("Ошибка получения catalogID: %v", err)
	}

	fmt.Printf("✓ Upload ID=%d, Справочник ID=%d созданы\n", uploadID, catalogID)

	fmt.Println("\n[4/4] Создание тестовых данных с дубликатами...")

	// Тестовые элементы с дубликатами
	testItems := []struct {
		Reference string
		Code      string
		Name      string
	}{
		// Оригинальные записи
		{"ref001", "001", "Болт М10x50"},
		{"ref002", "002", "Гайка М10"},
		{"ref003", "003", "Шайба М10"},

		// Дубликаты с вариациями (должны определяться как дубли)
		{"ref004", "001", "Болт М10х50"},     // Дубликат #1: тот же код, х вместо x
		{"ref005", "004", "болт м10x50"},     // Дубликат #2: тот же текст, разный регистр
		{"ref006", "005", "БОЛТ М10X50"},     // Дубликат #3: всё заглавными
		{"ref007", "006", "Болт М10х50 мм"},  // Дубликат #4: с добавлением "мм"
	}

	for _, item := range testItems {
		_, err := db.Exec(`
			INSERT INTO catalog_items (catalog_id, reference, code, name, created_at)
			VALUES (?, ?, ?, ?, ?)
		`, catalogID, item.Reference, item.Code, item.Name, time.Now().Format(time.RFC3339))

		if err != nil {
			log.Fatalf("Ошибка создания элемента %s: %v", item.Name, err)
		}
	}

	// Обновляем счетчики в upload
	_, err = db.Exec(`
		UPDATE uploads
		SET total_catalogs = 1, total_items = ?, status = 'completed'
		WHERE id = ?
	`, len(testItems), uploadID)

	if err != nil {
		log.Fatalf("Ошибка обновления счетчиков: %v", err)
	}

	fmt.Printf("✓ Создано %d элементов в справочнике 'Номенклатура'\n", len(testItems))

	// Проверяем результат
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM catalog_items").Scan(&count)
	if err != nil {
		log.Fatalf("Ошибка подсчета: %v", err)
	}

	fmt.Println("\n================================================================================")
	fmt.Printf("БАЗА ДАННЫХ ГОТОВА К ТЕСТИРОВАНИЮ\n")
	fmt.Printf("Upload ID: %d\n", uploadID)
	fmt.Printf("Catalog ID: %d\n", catalogID)
	fmt.Printf("Всего записей в БД: %d\n", count)
	fmt.Println("================================================================================")
	fmt.Println("\nТеперь запустите: ./test_dedup.exe")
}
