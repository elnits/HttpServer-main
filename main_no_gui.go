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
	log.Println("Запуск 1C HTTP Server (без GUI)...")

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
		log.Fatalf("Ошибка создания базы нормализованных данных: %v", err)
	}
	defer normalizedDB.Close()

	// Создаем сервисную базу данных для системной информации
	serviceDBPath := config.ServiceDatabasePath
	serviceDB, err := database.NewServiceDBWithConfig(serviceDBPath, dbConfig)
	if err != nil {
		log.Fatalf("Ошибка создания сервисной базы данных: %v", err)
	}
	defer serviceDB.Close()

	// Создаем сервер
	srv := server.NewServerWithConfig(db, normalizedDB, serviceDB, dbPath, normalizedDBPath, config)

	// Запускаем сервер в горутине
	go func() {
		if err := srv.Start(); err != nil {
			log.Fatalf("Ошибка запуска сервера: %v", err)
		}
	}()

	log.Printf("Сервер запущен на порту %s", config.Port)
	log.Println("API доступно по адресу: http://localhost:9999")
	log.Println("Для остановки нажмите Ctrl+C")

	// Ожидаем сигнал завершения
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	<-sigChan
	log.Println("Получен сигнал завершения, останавливаем сервер...")

	// Останавливаем сервер с таймаутом
	shutdownCtx, shutdownCancel := context.WithTimeout(ctx, 30*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("Ошибка при остановке сервера: %v", err)
	} else {
		log.Println("Сервер успешно остановлен")
	}
}

