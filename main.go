//go:build !no_gui
// +build !no_gui

package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"httpserver/database"
	"httpserver/gui"
	"httpserver/server"
)

func main() {
	log.Println("Запуск 1C HTTP Server...")

	// Загружаем конфигурацию
	config, err := server.LoadConfig()
	if err != nil {
		log.Fatalf("Ошибка загрузки конфигурации: %v", err)
	}

	// Определяем путь к основной БД
	// Используем 1c_data.db если существует, иначе data.db
	dbPath := config.DatabasePath
	if _, err := os.Stat("1c_data.db"); err == nil {
		dbPath = "1c_data.db"
		log.Printf("Используется существующая база данных: %s", dbPath)
	}

	// Создаем конфигурацию для БД
	dbConfig := database.DBConfig{
		MaxOpenConns:    config.MaxOpenConns,
		MaxIdleConns:    config.MaxIdleConns,
		ConnMaxLifetime: config.ConnMaxLifetime,
	}

	// Создаем базу данных
	db, err := database.NewDBWithConfig(dbPath, dbConfig)
	if err != nil {
		log.Fatalf("Ошибка создания базы данных: %v", err)
	}
	defer db.Close()

	// Создаем базу данных для нормализованных данных
	normalizedDBPath := config.NormalizedDatabasePath
	normalizedDB, err := database.NewDBWithConfig(normalizedDBPath, dbConfig)
	if err != nil {
		log.Fatalf("Ошибка создания нормализованной базы данных: %v", err)
	}
	defer normalizedDB.Close()
	log.Printf("Используется нормализованная база данных: %s", normalizedDBPath)

	// Создаем сервисную базу данных для системной информации
	serviceDBPath := config.ServiceDatabasePath
	serviceDB, err := database.NewServiceDBWithConfig(serviceDBPath, dbConfig)
	if err != nil {
		log.Fatalf("Ошибка создания сервисной базы данных: %v", err)
	}
	defer serviceDB.Close()
	log.Printf("Используется сервисная база данных: %s", serviceDBPath)

	// Создаем сервер с обеими БД и сервисной БД
	srv := server.NewServerWithConfig(db, normalizedDB, serviceDB, dbPath, normalizedDBPath, config)

	// Проверяем, нужно ли запускать GUI (по умолчанию в контейнере без GUI)
	useGUI := os.Getenv("USE_GUI") == "true"

	var window *gui.Window
	if useGUI {
		// Создаем GUI окно только если явно указано
		window = gui.NewWindow(srv.GetLogChannel())
	}

	// Запускаем сервер в отдельной горутине
	go func() {
		if err := srv.Start(); err != nil {
			log.Fatalf("Ошибка запуска сервера: %v", err)
		}
	}()

	// Фоновое сохранение метрик производительности в БД
	go func() {
		// Ждем 5 минут перед первым сохранением (чтобы накопились данные)
		time.Sleep(5 * time.Minute)

		// Периодически очищаем старые метрики (раз в день)
		cleanupTicker := time.NewTicker(24 * time.Hour)
		defer cleanupTicker.Stop()

		// Сохраняем метрики каждые 60 секунд
		saveTicker := time.NewTicker(60 * time.Second)
		defer saveTicker.Stop()

		for {
			select {
			case <-saveTicker.C:
				// Собираем текущие метрики
				snapshot := srv.CollectMetricsSnapshot()
				if snapshot != nil {
					// Сохраняем в БД
					if err := db.SaveMetrics(snapshot); err != nil {
						log.Printf("⚠ Ошибка сохранения метрик: %v", err)
					}
				}

			case <-cleanupTicker.C:
				// Очищаем метрики старше 7 дней
				if err := db.CleanOldMetrics(7); err != nil {
					log.Printf("⚠ Ошибка очистки старых метрик: %v", err)
				} else {
					log.Printf("✓ Старые метрики очищены (retention: 7 дней)")
				}
			}
		}
	}()

	// Обновляем статистику каждые 5 секунд
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				stats, err := db.GetStats()
				if err != nil {
					log.Printf("Ошибка получения статистики: %v", err)
					continue
				}

				if useGUI && window != nil {
					serverStats := server.ServerStats{
						IsRunning:    true,
						TotalStats:   stats,
						LastActivity: time.Now(),
					}
					// Обновляем статистику в GUI
					window.UpdateStatsFromMain(serverStats)
				} else {
					// Логируем статистику в консоль
					log.Printf("Статистика: Выгрузок: %v, Констант: %v, Справочников: %v, Элементов: %v",
						stats["total_uploads"],
						stats["total_constants"],
						stats["total_catalogs"],
						stats["total_items"])
				}
			}
		}
	}()

	// Обработка сигналов для graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("Получен сигнал завершения...")
		if useGUI && window != nil {
			window.SetStatus("Сервер останавливается...")
		}

		// Graceful shutdown
		if err := srv.Shutdown(ctx); err != nil {
			log.Printf("Ошибка при остановке сервера: %v", err)
		}

		cancel()
		os.Exit(0)
	}()

	log.Printf("Сервер запущен на порту %s", config.Port)
	log.Printf("API доступно по адресу: http://localhost:%s", config.Port)

	if useGUI && window != nil {
		log.Println("Открывается GUI интерфейс...")
		// Показываем GUI и блокируем выполнение
		window.ShowAndRun()
	} else {
		log.Println("Режим без GUI (для контейнера)")
		log.Println("Для остановки нажмите Ctrl+C")
		log.Println("========================================")
		// Блокируем выполнение
		<-ctx.Done()
	}
}
