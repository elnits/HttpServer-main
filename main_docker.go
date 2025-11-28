//go:build no_gui
// +build no_gui

package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"httpserver/database"
	"httpserver/server"
)

func main() {
	log.Println("Запуск 1C HTTP Server (Docker режим без GUI)...")
	
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
	
	// Запускаем сервер в отдельной горутине
	go func() {
		if err := srv.Start(); err != nil {
			log.Fatalf("Ошибка запуска сервера: %v", err)
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
				
				// Логируем статистику в консоль
				log.Printf("Статистика: Выгрузок: %v, Констант: %v, Справочников: %v, Элементов: %v",
					stats["total_uploads"],
					stats["total_constants"],
					stats["total_catalogs"],
					stats["total_items"])
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
		
		// Graceful shutdown
		if err := srv.Shutdown(ctx); err != nil {
			log.Printf("Ошибка при остановке сервера: %v", err)
		}
		
		cancel()
		os.Exit(0)
	}()
	
	log.Printf("Сервер запущен на порту %s", config.Port)
	log.Printf("API доступно по адресу: http://localhost:%s", config.Port)
	log.Println("Режим без GUI (Docker контейнер)")
	log.Println("Для остановки нажмите Ctrl+C")
	log.Println("========================================")
	
	// Блокируем выполнение
	<-ctx.Done()
}

