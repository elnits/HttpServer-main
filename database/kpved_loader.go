package database

import (
	"bufio"
	"database/sql"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
)

// KpvedEntry представляет одну запись классификатора КПВЭД
type KpvedEntry struct {
	Code       string
	Name       string
	ParentCode string
	Level      int
}

// ParseKpvedFile парсит файл КПВЭД.txt и возвращает список записей
func ParseKpvedFile(filePath string) ([]KpvedEntry, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open KPVED file: %w", err)
	}
	defer file.Close()

	var entries []KpvedEntry
	scanner := bufio.NewScanner(file)

	// Регулярное выражение для кода КПВЭД
	// Matches: A, 01, 01.1, 01.11, 01.11.1, 01.11.11
	codeRegex := regexp.MustCompile(`^([A-Z]|\d{2}(?:\.\d{1,2}){0,3})\s*\t`)

	lineNum := 0
	var currentEntry *KpvedEntry
	var currentSection string // Отслеживаем текущую секцию (A, B, C...)

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		// Пропускаем первые 4 строки (заголовки и пустые строки)
		if lineNum <= 4 {
			continue
		}

		// Проверяем, начинается ли строка с кода
		if codeRegex.MatchString(line) {
			// Если есть незавершенная запись, добавляем её
			if currentEntry != nil {
				currentEntry.Name = strings.TrimSpace(currentEntry.Name)
				entries = append(entries, *currentEntry)
			}

			// Разделяем по табуляции
			parts := strings.Split(line, "\t")
			if len(parts) < 2 {
				log.Printf("Warning: line %d has invalid format: %s", lineNum, line)
				continue
			}

			code := strings.TrimSpace(parts[0])
			name := strings.TrimSpace(parts[1])

			// Определяем уровень
			level := determineLevel(code)
			
			// Определяем родительский код
			var parentCode string
			
			// Если это секция (A-Z), обновляем currentSection
			if len(code) == 1 && code[0] >= 'A' && code[0] <= 'Z' {
				currentSection = code
				parentCode = "" // Секции не имеют родителя
			} else if len(code) == 2 {
				// Это класс (01, 02...), используем текущую секцию как родителя
				if currentSection != "" {
					parentCode = currentSection
				} else {
					parentCode = "" // Если секция еще не встречена, оставляем пустым
				}
			} else {
				// Для остальных уровней используем существующую логику
				parentCode = determineParentCode(code)
			}

			currentEntry = &KpvedEntry{
				Code:       code,
				Name:       name,
				ParentCode: parentCode,
				Level:      level,
			}
		} else if currentEntry != nil {
			// Это продолжение названия с предыдущей строки
			line = strings.TrimSpace(line)
			if line != "" {
				// Добавляем пробел если название не пустое
				if currentEntry.Name != "" {
					currentEntry.Name += " "
				}
				currentEntry.Name += line
			}
		}
	}

	// Добавляем последнюю запись
	if currentEntry != nil {
		currentEntry.Name = strings.TrimSpace(currentEntry.Name)
		entries = append(entries, *currentEntry)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read KPVED file: %w", err)
	}

	log.Printf("Parsed %d KPVED entries from file", len(entries))
	return entries, nil
}

// determineLevel определяет уровень вложенности кода КПВЭД
func determineLevel(code string) int {
	// A-Z = level 0 (секция)
	if len(code) == 1 && code[0] >= 'A' && code[0] <= 'Z' {
		return 0
	}

	// 01 = level 1 (класс)
	if len(code) == 2 {
		return 1
	}

	// Считаем количество точек для определения уровня
	// 01.1 или 01.11 = level 2
	// 01.11.1 или 01.11.11 = level 3
	// 01.11.11.1 = level 4
	dotCount := strings.Count(code, ".")
	return 1 + dotCount
}

// determineParentCode определяет родительский код
func determineParentCode(code string) string {
	// Секции (A-Z) не имеют родителя
	if len(code) == 1 {
		return ""
	}

	// Классы (01, 02, ...) имеют родителя - секцию
	// Но мы не знаем какую секцию, поэтому оставляем пустым
	// В реальном классификаторе связь есть, но она неявная
	if len(code) == 2 {
		return ""
	}

	// Для остальных - удаляем последний сегмент после точки
	// 01.11.12 -> 01.11.1
	// 01.11.1 -> 01.11
	// 01.11 -> 01.1 или 01

	lastDotIndex := strings.LastIndex(code, ".")
	if lastDotIndex == -1 {
		return ""
	}

	parentCode := code[:lastDotIndex]

	// Специальный случай: 01.11.1 -> родитель 01.11 (не 01.1)
	// Проверяем длину последнего сегмента
	lastSegment := code[lastDotIndex+1:]
	if len(lastSegment) == 1 {
		// Это код вида XX.YY.Z
		// Родитель - XX.YY
		return parentCode
	}

	// Это код вида XX.YY.ZZ
	// Родитель - XX.YY.Z (берем первую цифру последнего сегмента)
	if len(lastSegment) == 2 {
		return parentCode + "." + string(lastSegment[0])
	}

	return parentCode
}

// LoadKpvedToDatabase загружает записи КПВЭД в базу данных
func LoadKpvedToDatabase(db *sql.DB, entries []KpvedEntry) error {
	// Начинаем транзакцию для batch insert
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Очищаем таблицу перед загрузкой
	_, err = tx.Exec("DELETE FROM kpved_classifier")
	if err != nil {
		return fmt.Errorf("failed to clear kpved_classifier table: %w", err)
	}

	// Подготавливаем statement для вставки
	stmt, err := tx.Prepare(`
		INSERT INTO kpved_classifier (code, name, parent_code, level)
		VALUES (?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	// Вставляем записи
	for _, entry := range entries {
		parentCode := sql.NullString{
			String: entry.ParentCode,
			Valid:  entry.ParentCode != "",
		}

		_, err = stmt.Exec(entry.Code, entry.Name, parentCode, entry.Level)
		if err != nil {
			return fmt.Errorf("failed to insert KPVED entry %s: %w", entry.Code, err)
		}
	}

	// Коммитим транзакцию
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	log.Printf("Successfully loaded %d KPVED entries to database", len(entries))
	return nil
}

// LoadKpvedFromFile - вспомогательная функция для загрузки КПВЭД из файла в БД
func LoadKpvedFromFile(db *sql.DB, filePath string) error {
	log.Printf("Loading KPVED classifier from file: %s", filePath)

	// Парсим файл
	entries, err := ParseKpvedFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to parse KPVED file: %w", err)
	}

	// Загружаем в БД
	if err := LoadKpvedToDatabase(db, entries); err != nil {
		return fmt.Errorf("failed to load KPVED to database: %w", err)
	}

	log.Printf("KPVED classifier loaded successfully")
	return nil
}
