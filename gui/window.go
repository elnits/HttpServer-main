package gui

import (
	"fmt"
	"log"
	"time"

	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/widget"
	"fyne.io/fyne/v2"
	"httpserver/server"
)

// Window представляет главное окно приложения
type Window struct {
	app        fyne.App
	window     fyne.Window
	logList    *widget.List
	statsLabel *widget.Label
	statusLabel *widget.Label
	
	// Данные
	logData    binding.StringList
	statsData  binding.String
	statusData binding.String
	
	// Каналы
	logChan    <-chan server.LogEntry
}

// NewWindow создает новое окно
func NewWindow(logChan <-chan server.LogEntry) *Window {
	myApp := app.New()
	myWindow := myApp.NewWindow("1C HTTP Server")
	myWindow.Resize(fyne.NewSize(800, 600))
	
	w := &Window{
		app:        myApp,
		window:     myWindow,
		logData:    binding.NewStringList(),
		statsData:  binding.NewString(),
		statusData: binding.NewString(),
		logChan:    logChan,
	}
	
	w.setupUI()
	w.startListeners()
	
	return w
}

// setupUI настраивает пользовательский интерфейс
func (w *Window) setupUI() {
	// Заголовок
	title := widget.NewLabel("1C HTTP Server")
	title.TextStyle.Bold = true
	
	// Статус сервера
	w.statusLabel = widget.NewLabelWithData(w.statusData)
	w.statusData.Set("Сервер запущен")
	
	// Статистика
	w.statsLabel = widget.NewLabelWithData(w.statsData)
	w.statsData.Set("Статистика загружается...")
	
	// Лог запросов
	logTitle := widget.NewLabel("Лог запросов:")
	logTitle.TextStyle.Bold = true
	
	w.logList = widget.NewListWithData(
		w.logData,
		func() fyne.CanvasObject {
			return widget.NewLabel("")
		},
		func(item binding.DataItem, obj fyne.CanvasObject) {
			label := obj.(*widget.Label)
			label.Bind(item.(binding.String))
		},
	)
	
	// Кнопки управления
	refreshBtn := widget.NewButton("Обновить статистику", w.refreshStats)
	clearLogBtn := widget.NewButton("Очистить лог", w.clearLog)
	
	// Компоновка
	statusContainer := container.NewVBox(
		widget.NewLabel("Статус:"),
		w.statusLabel,
		widget.NewSeparator(),
		widget.NewLabel("Статистика:"),
		w.statsLabel,
		widget.NewSeparator(),
		container.NewHBox(refreshBtn, clearLogBtn),
	)
	
	logContainer := container.NewVBox(
		logTitle,
		w.logList,
	)
	
	content := container.NewHSplit(
		statusContainer,
		logContainer,
	)
	content.SetOffset(0.3) // 30% для статуса, 70% для лога
	
	w.window.SetContent(content)
}

// startListeners запускает слушатели каналов
func (w *Window) startListeners() {
	// Слушатель логов
	go func() {
		for entry := range w.logChan {
			logText := fmt.Sprintf("[%s] %s: %s", 
				entry.Timestamp.Format("15:04:05"), 
				entry.Level, 
				entry.Message)
			
			// Добавляем в начало списка
			current, _ := w.logData.Get()
			newLogs := append([]string{logText}, current...)
			
			// Ограничиваем количество записей
			if len(newLogs) > 1000 {
				newLogs = newLogs[:1000]
			}
			
			w.logData.Set(newLogs)
		}
	}()
	
	// Статистика обновляется из main.go
}

// updateStats обновляет статистику
func (w *Window) updateStats(stats server.ServerStats) {
	statsText := fmt.Sprintf(`Статус: %s
Последняя активность: %s

Общая статистика:
• Всего выгрузок: %v
• Активных выгрузок: %v
• Всего констант: %v
• Всего справочников: %v
• Всего элементов: %v`,
		map[bool]string{true: "Работает", false: "Остановлен"}[stats.IsRunning],
		stats.LastActivity.Format("15:04:05"),
		stats.TotalStats["total_uploads"],
		stats.TotalStats["active_uploads"],
		stats.TotalStats["total_constants"],
		stats.TotalStats["total_catalogs"],
		stats.TotalStats["total_items"])
	
	if stats.CurrentUpload != nil {
		statsText += fmt.Sprintf(`

Текущая выгрузка:
• UUID: %s
• Статус: %s
• Версия 1С: %s
• Конфигурация: %s
• Константы: %d
• Справочники: %d
• Элементы: %d`,
			stats.CurrentUpload.UploadUUID,
			stats.CurrentUpload.Status,
			stats.CurrentUpload.Version1C,
			stats.CurrentUpload.ConfigName,
			stats.CurrentUpload.TotalConstants,
			stats.CurrentUpload.TotalCatalogs,
			stats.CurrentUpload.TotalItems)
	}
	
	w.statsData.Set(statsText)
}

// refreshStats обновляет статистику
func (w *Window) refreshStats() {
	// Здесь можно добавить запрос к серверу для получения актуальной статистики
	log.Println("Обновление статистики...")
}

// clearLog очищает лог
func (w *Window) clearLog() {
	w.logData.Set([]string{})
	log.Println("Лог очищен")
}

// ShowAndRun показывает окно и запускает приложение
func (w *Window) ShowAndRun() {
	w.window.ShowAndRun()
}

// SetStatus устанавливает статус сервера
func (w *Window) SetStatus(status string) {
	w.statusData.Set(status)
}

// AddLog добавляет запись в лог
func (w *Window) AddLog(message string) {
	logText := fmt.Sprintf("[%s] %s", time.Now().Format("15:04:05"), message)
	current, _ := w.logData.Get()
	newLogs := append([]string{logText}, current...)
	
	if len(newLogs) > 1000 {
		newLogs = newLogs[:1000]
	}
	
	w.logData.Set(newLogs)
}

// UpdateStatsFromMain обновляет статистику из main.go
func (w *Window) UpdateStatsFromMain(stats server.ServerStats) {
	w.updateStats(stats)
}
