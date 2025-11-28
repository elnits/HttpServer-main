package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"httpserver/database"
	"httpserver/normalization"
	"httpserver/nomenclature"
	"httpserver/server"
)

func main() {
	// Параметры командной строки
	dbPath := flag.String("db", "1c_data.db", "Путь к базе данных (по умолчанию: 1c_data.db)")
	useAI := flag.Bool("ai", false, "Использовать AI для нормализации")
	logFile := flag.String("log", "", "Путь к файлу лога (опционально)")
	flag.Parse()

	// Настройка логирования в файл, если указан
	if *logFile != "" {
		logDir := "logs"
		if err := os.MkdirAll(logDir, 0755); err != nil {
			log.Printf("⚠ Не удалось создать директорию логов: %v", err)
		} else {
			fullLogPath := *logFile
			if !strings.Contains(fullLogPath, string(os.PathSeparator)) {
				fullLogPath = filepath.Join(logDir, *logFile)
			}
			logFileHandle, err := os.OpenFile(fullLogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
			if err != nil {
				log.Printf("⚠ Не удалось открыть файл лога: %v, продолжаем без файлового логирования", err)
			} else {
				defer logFileHandle.Close()
				log.SetOutput(io.MultiWriter(os.Stdout, logFileHandle))
				log.Printf("📝 Логирование в файл: %s", fullLogPath)
			}
		}
	}

	// Проверяем существование файла БД
	if _, err := os.Stat(*dbPath); os.IsNotExist(err) {
		log.Fatalf("База данных не найдена: %s", *dbPath)
	}

	log.Printf("Подключение к базе данных: %s", *dbPath)

	// Подключаемся к базе данных
	db, err := database.NewDB(*dbPath)
	if err != nil {
		log.Fatalf("Ошибка подключения к базе данных: %v", err)
	}
	defer db.Close()

	log.Println("База данных успешно подключена")

	// Проверяем наличие таблицы kpved_classifier
	var kpvedCount int
	err = db.QueryRow("SELECT COUNT(*) FROM kpved_classifier").Scan(&kpvedCount)
	if err != nil {
		log.Printf("⚠ Предупреждение: таблица kpved_classifier не найдена или пуста: %v", err)
		log.Println("⚠ КПВЭД классификация будет пропущена")
	} else {
		log.Printf("✓ Найдено %d записей в классификаторе КПВЭД", kpvedCount)
	}

	// Создаем конфигурацию AI
	var aiConfig *normalization.AIConfig
	if *useAI {
		apiKey := os.Getenv("ARLIAI_API_KEY")
		if apiKey == "" {
			log.Println("⚠ ARLIAI_API_KEY не установлен, AI отключен")
			*useAI = false
		} else {
			aiConfig = &normalization.AIConfig{
				Enabled:        true,
				MinConfidence:  0.7,
				RateLimitDelay: 100 * time.Millisecond,
				MaxRetries:     3,
			}
			log.Println("✓ AI нормализация включена")
		}
	}

	// Создаем нормализатор
	normalizer := normalization.NewNormalizer(db, nil, aiConfig)

	// Если AI не включен, но есть КПВЭД классификатор, инициализируем его вручную
	if !*useAI && kpvedCount > 0 {
		// Создаем менеджер конфигурации для получения модели и API ключа
		configManager := server.NewWorkerConfigManager()
		
		// Получаем API ключ и модель из конфигурации
		apiKey, model, err := configManager.GetModelAndAPIKey()
		if err != nil {
			// Fallback на переменные окружения
			apiKey = os.Getenv("ARLIAI_API_KEY")
			if apiKey == "" {
				log.Println("⚠ ARLIAI_API_KEY не установлен, КПВЭД классификация будет пропущена")
			} else {
				model = os.Getenv("ARLIAI_MODEL")
				if model == "" {
					model = "GLM-4.5-Air"
				}
			}
		}
		
		if apiKey != "" {
			// Создаем AI клиент для классификатора
			aiClient := nomenclature.NewAIClient(apiKey, model)
			
			// Инициализируем иерархический классификатор
			hierarchicalClassifier, err := normalization.NewHierarchicalClassifier(db, aiClient)
			if err != nil {
				log.Printf("⚠ Предупреждение: не удалось инициализировать КПВЭД классификатор: %v", err)
			} else {
				normalizer.SetHierarchicalClassifier(hierarchicalClassifier)
				log.Println("✓ Иерархический КПВЭД классификатор инициализирован")
			}
		} else {
			log.Println("⚠ ARLIAI_API_KEY не установлен, КПВЭД классификация будет пропущена")
		}
	}

	// Проверяем количество элементов для обработки
	items, err := db.GetCatalogItemsFromTable("catalog_items", "reference", "code", "name")
	if err != nil {
		log.Printf("⚠ Предупреждение: не удалось получить количество элементов: %v", err)
	} else {
		log.Printf("📊 Найдено %d элементов для обработки", len(items))
	}

	// Запускаем нормализацию
	log.Println("\n🚀 Запуск процесса нормализации и классификации...")
	startTime := time.Now()
	
	if err := normalizer.ProcessNormalization(); err != nil {
		log.Fatalf("❌ Ошибка нормализации: %v", err)
	}

	duration := time.Since(startTime)
	
	// Проверяем результаты
	var normalizedCount, kpvedClassifiedCount int
	db.QueryRow("SELECT COUNT(*) FROM normalized_data").Scan(&normalizedCount)
	db.QueryRow("SELECT COUNT(*) FROM normalized_data WHERE kpved_code IS NOT NULL AND kpved_code != ''").Scan(&kpvedClassifiedCount)

	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("✓ Нормализация успешно завершена!")
	fmt.Printf("⏱  Время выполнения: %v\n", duration.Round(time.Second))
	fmt.Printf("📊 Обработано записей: %d\n", normalizedCount)
	fmt.Printf("🏷️  Классифицировано по КПВЭД: %d (%.1f%%)\n", 
		kpvedClassifiedCount, 
		float64(kpvedClassifiedCount)/float64(normalizedCount)*100)
	fmt.Println(strings.Repeat("=", 60))
}

