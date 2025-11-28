package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

// NotifierConfig конфигурация уведомлений
type NotifierConfig struct {
	Enabled     bool   `json:"enabled"`
	Type        string `json:"type"` // "telegram", "email", "webhook"
	Telegram    TelegramConfig `json:"telegram,omitempty"`
	Email       EmailConfig    `json:"email,omitempty"`
	Webhook     WebhookConfig  `json:"webhook,omitempty"`
	MinSeverity string `json:"min_severity"` // "info", "warning", "error", "critical"
}

// TelegramConfig конфигурация Telegram уведомлений
type TelegramConfig struct {
	BotToken string `json:"bot_token"`
	ChatID   string `json:"chat_id"`
}

// EmailConfig конфигурация Email уведомлений
type EmailConfig struct {
	SMTPHost     string   `json:"smtp_host"`
	SMTPPort     int      `json:"smtp_port"`
	Username     string   `json:"username"`
	Password     string   `json:"password"`
	From         string   `json:"from"`
	To           []string `json:"to"`
	Subject      string   `json:"subject"`
}

// WebhookConfig конфигурация Webhook уведомлений
type WebhookConfig struct {
	URL     string            `json:"url"`
	Method  string            `json:"method"`
	Headers map[string]string `json:"headers"`
}

// sendNotification отправляет уведомление о результатах проверки
func sendNotification(report *Report, config *NotifierConfig) error {
	if !config.Enabled {
		return nil
	}

	// Определяем уровень серьезности
	severity := determineSeverity(report)
	if !shouldNotify(severity, config.MinSeverity) {
		return nil
	}

	switch config.Type {
	case "telegram":
		return sendTelegramNotification(report, config.Telegram, severity)
	case "email":
		return sendEmailNotification(report, config.Email, severity)
	case "webhook":
		return sendWebhookNotification(report, config.Webhook, severity)
	default:
		return fmt.Errorf("неизвестный тип уведомления: %s", config.Type)
	}
}

func determineSeverity(report *Report) string {
	if report.Summary.ServerErrors > 0 || report.Summary.Timeouts > 0 {
		return "critical"
	}
	if report.Summary.ClientErrors > 0 {
		return "error"
	}
	if report.Summary.Invalid > 0 {
		return "warning"
	}
	return "info"
}

func shouldNotify(severity, minSeverity string) bool {
	levels := map[string]int{
		"info":     0,
		"warning":  1,
		"error":    2,
		"critical": 3,
	}

	return levels[severity] >= levels[minSeverity]
}

func sendTelegramNotification(report *Report, config TelegramConfig, severity string) error {
	if config.BotToken == "" || config.ChatID == "" {
		return fmt.Errorf("Telegram конфигурация неполная")
	}

	emoji := map[string]string{
		"info":     "ℹ️",
		"warning":  "⚠️",
		"error":    "❌",
		"critical": "🔴",
	}

	message := fmt.Sprintf(
		"%s *HTTP Check Report*\n\n"+
			"*Статус:* %s\n"+
			"*Проверок:* %d\n"+
			"*Успешных:* %d\n"+
			"*Ошибок:* %d\n"+
			"*Время:* %.2f сек\n\n",
		emoji[severity],
		strings.ToUpper(severity),
		report.TotalChecks,
		report.Summary.Success,
		report.Summary.TotalErrors,
		report.Duration.Seconds(),
	)

	if report.Summary.TotalErrors > 0 {
		message += "*Проблемные URL:*\n"
		count := 0
		for _, result := range report.Results {
			if !result.IsValid || result.Error != "" {
				if count < 5 { // Ограничиваем количество в сообщении
					statusInfo := fmt.Sprintf("%d %s", result.Status, result.StatusText)
					if result.Error != "" {
						statusInfo = result.Error
					}
					message += fmt.Sprintf("• %s - %s\n", result.URL, statusInfo)
					count++
				}
			}
		}
		if len(report.Results)-count > 0 {
			message += fmt.Sprintf("... и еще %d\n", len(report.Results)-count)
		}
	}

	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", config.BotToken)
	payload := map[string]interface{}{
		"chat_id":    config.ChatID,
		"text":       message,
		"parse_mode": "Markdown",
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Telegram API вернул статус %d", resp.StatusCode)
	}

	// Уведомление отправлено в Telegram
	return nil
}

func sendEmailNotification(report *Report, config EmailConfig, severity string) error {
	// Простая реализация через SMTP
	// Для полной реализации можно использовать библиотеку типа go-smtp
	// Email уведомления требуют дополнительной настройки SMTP
	return fmt.Errorf("Email уведомления не реализованы (требуется SMTP библиотека)")
}

func sendWebhookNotification(report *Report, config WebhookConfig, severity string) error {
	if config.URL == "" {
		return fmt.Errorf("Webhook URL не указан")
	}

	method := config.Method
	if method == "" {
		method = "POST"
	}

	payload := map[string]interface{}{
		"timestamp": time.Now().Format(time.RFC3339),
		"severity":  severity,
		"report":    report,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(method, config.URL, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	for key, value := range config.Headers {
		req.Header.Set(key, value)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Webhook вернул статус %d", resp.StatusCode)
	}

	// Уведомление отправлено на webhook
	return nil
}

// loadNotifierConfig загружает конфигурацию уведомлений из переменных окружения или файла
func loadNotifierConfig() *NotifierConfig {
	config := &NotifierConfig{
		Enabled:     false,
		MinSeverity: "error",
	}

	// Проверяем переменные окружения для Telegram
	botToken := os.Getenv("TELEGRAM_BOT_TOKEN")
	chatID := os.Getenv("TELEGRAM_CHAT_ID")
	if botToken != "" && chatID != "" {
		config.Enabled = true
		config.Type = "telegram"
		config.Telegram = TelegramConfig{
			BotToken: botToken,
			ChatID:   chatID,
		}
	}

	// Проверяем переменные окружения для Webhook
	webhookURL := os.Getenv("WEBHOOK_URL")
	if webhookURL != "" && !config.Enabled {
		config.Enabled = true
		config.Type = "webhook"
		config.Webhook = WebhookConfig{
			URL:    webhookURL,
			Method: "POST",
		}
	}

	return config
}

