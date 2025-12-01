package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"httpserver/database"

	_ "github.com/mattn/go-sqlite3"
)

func main() {
	// Параметры командной строки
	sourceDir := flag.String("source", ".", "Директория с исходными файлами БД")
	targetDB := flag.String("target", "unified_catalogs.db", "Путь к целевой единой БД")
	createBackup := flag.Bool("backup", true, "Создать резервную копию старых БД")
	dryRun := flag.Bool("dry-run", false, "Только показать что будет сделано, без изменений")
	flag.Parse()

	log.Println("========================================")
	log.Println("Миграция данных в единую БД справочников")
	log.Println("========================================")
	log.Printf("Исходная директория: %s", *sourceDir)
	log.Printf("Целевая БД: %s", *targetDB)
	log.Printf("Создать резервную копию: %v", *createBackup)
	log.Printf("Режим dry-run: %v", *dryRun)
	log.Println("========================================")

	// Находим все файлы БД
	dbFiles, err := findDatabaseFiles(*sourceDir)
	if err != nil {
		log.Fatalf("Ошибка поиска файлов БД: %v", err)
	}

	log.Printf("Найдено файлов БД: %d", len(dbFiles))
	for i, file := range dbFiles {
		log.Printf("  %d. %s", i+1, file)
	}

	if len(dbFiles) == 0 {
		log.Println("Нет файлов для миграции")
		return
	}

	if *dryRun {
		log.Println("\n[DRY-RUN] Миграция не будет выполнена")
		return
	}

	// Создаём резервную копию если требуется
	if *createBackup {
		backupDir := fmt.Sprintf("backup_%s", time.Now().Format("2006-01-02_15-04-05"))
		if err := os.MkdirAll(backupDir, 0755); err != nil {
			log.Fatalf("Ошибка создания директории резервных копий: %v", err)
		}
		log.Printf("\nСоздание резервных копий в: %s", backupDir)
		
		for _, file := range dbFiles {
			backupPath := filepath.Join(backupDir, filepath.Base(file))
			if err := copyFile(file, backupPath); err != nil {
				log.Printf("  ✗ Ошибка копирования %s: %v", file, err)
			} else {
				log.Printf("  ✓ %s", filepath.Base(file))
			}
		}
	}

	// Открываем или создаём целевую БД
	log.Printf("\nИнициализация целевой БД: %s", *targetDB)
	
	dbConfig := database.DBConfig{
		MaxOpenConns:    25,
		MaxIdleConns:    5,
		ConnMaxLifetime: 5 * time.Minute,
	}
	
	targetDatabase, err := database.NewUnifiedDBWithConfig(*targetDB, dbConfig)
	if err != nil {
		log.Fatalf("Ошибка создания целевой БД: %v", err)
	}
	defer targetDatabase.Close()
	
	log.Println("✓ Целевая БД инициализирована")

	// Статистика миграции
	stats := MigrationStats{}

	// Мигрируем каждый файл
	log.Println("\n========================================")
	log.Println("Начало миграции")
	log.Println("========================================")

	for i, file := range dbFiles {
		log.Printf("\n[%d/%d] Миграция: %s", i+1, len(dbFiles), filepath.Base(file))
		
		if err := migrateDatabase(file, targetDatabase, &stats); err != nil {
			log.Printf("  ✗ ОШИБКА: %v", err)
			stats.FailedFiles++
		} else {
			log.Printf("  ✓ Успешно")
			stats.SuccessFiles++
		}
	}

	// Выводим итоговую статистику
	log.Println("\n========================================")
	log.Println("ИТОГИ МИГРАЦИИ")
	log.Println("========================================")
	log.Printf("Всего файлов: %d", len(dbFiles))
	log.Printf("Успешно: %d", stats.SuccessFiles)
	log.Printf("Ошибок: %d", stats.FailedFiles)
	log.Printf("Выгрузок перенесено: %d", stats.TotalUploads)
	log.Printf("Констант перенесено: %d", stats.TotalConstants)
	log.Printf("Справочников перенесено: %d", stats.TotalCatalogs)
	log.Printf("Элементов перенесено: %d", stats.TotalItems)
	log.Println("========================================")
	
	if stats.FailedFiles > 0 {
		log.Println("\n⚠ Некоторые файлы не были перенесены. Проверьте логи выше.")
	} else {
		log.Println("\n✓ Все файлы успешно перенесены!")
	}
}

// MigrationStats статистика миграции
type MigrationStats struct {
	SuccessFiles   int
	FailedFiles    int
	TotalUploads   int
	TotalConstants int
	TotalCatalogs  int
	TotalItems     int
}

// findDatabaseFiles находит все файлы БД в директории
func findDatabaseFiles(dir string) ([]string, error) {
	var files []string
	
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		
		// Пропускаем директории
		if info.IsDir() {
			return nil
		}
		
		// Ищем файлы с названием "Выгрузка_*.db"
		if strings.HasPrefix(info.Name(), "Выгрузка_") && strings.HasSuffix(info.Name(), ".db") {
			files = append(files, path)
		}
		
		return nil
	})
	
	return files, err
}

// copyFile копирует файл
func copyFile(src, dst string) error {
	input, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	
	return os.WriteFile(dst, input, 0644)
}

// migrateDatabase мигрирует одну БД
func migrateDatabase(sourceFile string, targetDB *database.DB, stats *MigrationStats) error {
	// Открываем исходную БД
	sourceConn, err := sql.Open("sqlite3", sourceFile)
	if err != nil {
		return fmt.Errorf("не удалось открыть исходную БД: %w", err)
	}
	defer sourceConn.Close()

	// Читаем данные из исходной БД
	// 1. Читаем uploads
	uploads, err := readUploads(sourceConn)
	if err != nil {
		return fmt.Errorf("ошибка чтения uploads: %w", err)
	}

	if len(uploads) == 0 {
		log.Printf("  ⚠ В БД нет выгрузок, пропускаем")
		return nil
	}

	log.Printf("  Найдено выгрузок: %d", len(uploads))

	// Для каждой выгрузки
	for _, upload := range uploads {
		log.Printf("    Миграция выгрузки: %s", upload.UploadUUID)

		// Создаём выгрузку в целевой БД
		_, err := targetDB.CreateUploadWithDatabase(
			upload.UploadUUID,
			upload.Version1C,
			upload.ConfigName,
			upload.DatabaseID,
			upload.ComputerName,
			upload.UserName,
			upload.ConfigVersion,
			upload.IterationNumber,
			upload.IterationLabel,
			upload.ProgrammerName,
			upload.UploadPurpose,
			upload.ParentUploadID,
		)
		if err != nil {
			// Если выгрузка уже существует, пропускаем
			if strings.Contains(err.Error(), "UNIQUE constraint failed") {
				log.Printf("    ⚠ Выгрузка %s уже существует, пропускаем", upload.UploadUUID)
				continue
			}
			return fmt.Errorf("ошибка создания выгрузки: %w", err)
		}

		stats.TotalUploads++

		// 2. Мигрируем константы
		constants, err := readConstants(sourceConn, upload.ID)
		if err != nil {
			return fmt.Errorf("ошибка чтения констант: %w", err)
		}

		log.Printf("      Констант: %d", len(constants))
		
		for _, constant := range constants {
			if err := addConstantToTarget(targetDB, upload.UploadUUID, constant); err != nil {
				log.Printf("      ✗ Ошибка добавления константы: %v", err)
			} else {
				stats.TotalConstants++
			}
		}

		// 3. Мигрируем справочники
		catalogs, err := readCatalogs(sourceConn, upload.ID)
		if err != nil {
			return fmt.Errorf("ошибка чтения справочников: %w", err)
		}

		log.Printf("      Справочников: %d", len(catalogs))

		for _, catalog := range catalogs {
			// Получаем или создаём таблицу для справочника
			tableName, err := database.GetOrCreateCatalogTable(targetDB.GetDB(), catalog.Name)
			if err != nil {
				log.Printf("      ✗ Ошибка создания таблицы для '%s': %v", catalog.Name, err)
				continue
			}

			stats.TotalCatalogs++
			log.Printf("        Справочник: %s -> %s", catalog.Name, tableName)

			// Читаем элементы справочника
			items, err := readCatalogItems(sourceConn, catalog.ID)
			if err != nil {
				log.Printf("        ✗ Ошибка чтения элементов: %v", err)
				continue
			}

			log.Printf("          Элементов: %d", len(items))

			// Мигрируем элементы
			targetUpload, err := targetDB.GetUploadByUUID(upload.UploadUUID)
			if err != nil {
				log.Printf("        ✗ Ошибка получения upload: %v", err)
				continue
			}

			for _, item := range items {
				err := targetDB.AddCatalogItemToTable(
					tableName,
					targetUpload.ID,
					item.Reference,
					item.Code,
					item.Name,
					item.Attributes,
					item.TableParts,
				)
				if err != nil {
					log.Printf("          ✗ Ошибка добавления элемента: %v", err)
				} else {
					stats.TotalItems++
				}
			}
		}
	}

	return nil
}

// UploadData структура для хранения данных выгрузки
type UploadData struct {
	ID              int
	UploadUUID      string
	Version1C       string
	ConfigName      string
	DatabaseID      *int
	ComputerName    string
	UserName        string
	ConfigVersion   string
	IterationNumber int
	IterationLabel  string
	ProgrammerName  string
	UploadPurpose   string
	ParentUploadID  *int
}

// ConstantData структура для хранения данных константы
type ConstantData struct {
	Name    string
	Synonym string
	Type    string
	Value   string
}

// CatalogData структура для хранения данных справочника
type CatalogData struct {
	ID      int
	Name    string
	Synonym string
}

// CatalogItemData структура для хранения данных элемента справочника
type CatalogItemData struct {
	Reference  string
	Code       string
	Name       string
	Attributes string
	TableParts string
}

// readUploads читает выгрузки из БД
func readUploads(db *sql.DB) ([]UploadData, error) {
	rows, err := db.Query(`
		SELECT id, upload_uuid, COALESCE(version_1c, ''), COALESCE(config_name, ''),
		       database_id, COALESCE(computer_name, ''), COALESCE(user_name, ''),
		       COALESCE(config_version, ''), COALESCE(iteration_number, 1),
		       COALESCE(iteration_label, ''), COALESCE(programmer_name, ''),
		       COALESCE(upload_purpose, ''), parent_upload_id
		FROM uploads
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var uploads []UploadData
	for rows.Next() {
		var u UploadData
		err := rows.Scan(
			&u.ID, &u.UploadUUID, &u.Version1C, &u.ConfigName,
			&u.DatabaseID, &u.ComputerName, &u.UserName,
			&u.ConfigVersion, &u.IterationNumber, &u.IterationLabel,
			&u.ProgrammerName, &u.UploadPurpose, &u.ParentUploadID,
		)
		if err != nil {
			return nil, err
		}
		uploads = append(uploads, u)
	}

	return uploads, rows.Err()
}

// readConstants читает константы из БД
func readConstants(db *sql.DB, uploadID int) ([]ConstantData, error) {
	rows, err := db.Query(`
		SELECT name, COALESCE(synonym, ''), COALESCE(type, ''), COALESCE(value, '')
		FROM constants
		WHERE upload_id = ?
	`, uploadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var constants []ConstantData
	for rows.Next() {
		var c ConstantData
		if err := rows.Scan(&c.Name, &c.Synonym, &c.Type, &c.Value); err != nil {
			return nil, err
		}
		constants = append(constants, c)
	}

	return constants, rows.Err()
}

// readCatalogs читает справочники из БД
func readCatalogs(db *sql.DB, uploadID int) ([]CatalogData, error) {
	rows, err := db.Query(`
		SELECT id, name, COALESCE(synonym, '')
		FROM catalogs
		WHERE upload_id = ?
	`, uploadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var catalogs []CatalogData
	for rows.Next() {
		var c CatalogData
		if err := rows.Scan(&c.ID, &c.Name, &c.Synonym); err != nil {
			return nil, err
		}
		catalogs = append(catalogs, c)
	}

	return catalogs, rows.Err()
}

// readCatalogItems читает элементы справочника из БД
func readCatalogItems(db *sql.DB, catalogID int) ([]CatalogItemData, error) {
	rows, err := db.Query(`
		SELECT reference, COALESCE(code, ''), COALESCE(name, ''),
		       COALESCE(attributes_xml, ''), COALESCE(table_parts_xml, '')
		FROM catalog_items
		WHERE catalog_id = ?
	`, catalogID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []CatalogItemData
	for rows.Next() {
		var item CatalogItemData
		if err := rows.Scan(&item.Reference, &item.Code, &item.Name, &item.Attributes, &item.TableParts); err != nil {
			return nil, err
		}
		items = append(items, item)
	}

	return items, rows.Err()
}

// addConstantToTarget добавляет константу в целевую БД
func addConstantToTarget(db *database.DB, uploadUUID string, constant ConstantData) error {
	upload, err := db.GetUploadByUUID(uploadUUID)
	if err != nil {
		return err
	}

	return db.AddConstant(upload.ID, constant.Name, constant.Synonym, constant.Type, constant.Value)
}


