package main

import (
	"fmt"
	"html/template"
	"log"
	"os"
	"time"

	"httpserver/database"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Println("Использование: export_normalization_report <путь_к_базе.db> <путь_к_файлу.html>")
		os.Exit(1)
	}

	dbPath := os.Args[1]
	outputFile := os.Args[2]

	db, err := database.NewDB(dbPath)
	if err != nil {
		log.Fatalf("Ошибка подключения: %v", err)
	}
	defer db.Close()

	// Собираем данные
	var totalItems, normalizedCount, kpvedCount, uniqueCategories int
	var changedCount, mergedCount int

	db.QueryRow("SELECT COUNT(*) FROM catalog_items").Scan(&totalItems)
	db.QueryRow("SELECT COUNT(*) FROM normalized_data").Scan(&normalizedCount)
	db.QueryRow("SELECT COUNT(*) FROM normalized_data WHERE kpved_code IS NOT NULL AND kpved_code != ''").Scan(&kpvedCount)
	db.QueryRow("SELECT COUNT(DISTINCT category) FROM normalized_data WHERE category IS NOT NULL AND category != ''").Scan(&uniqueCategories)
	db.QueryRow("SELECT COUNT(*) FROM normalized_data WHERE source_name != normalized_name AND normalized_name IS NOT NULL AND normalized_name != ''").Scan(&changedCount)
	db.QueryRow("SELECT COUNT(*) FROM normalized_data WHERE merged_count > 1").Scan(&mergedCount)

	// Топ категории
	type CategoryStat struct {
		Name      string
		Count     int
		Percent   float64
	}
	var topCategories []CategoryStat
	categoryQuery := `
		SELECT category, COUNT(*) as count
		FROM normalized_data
		WHERE category IS NOT NULL AND category != ''
		GROUP BY category
		ORDER BY count DESC
		LIMIT 20
	`
	rows, _ := db.Query(categoryQuery)
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var cat CategoryStat
			if err := rows.Scan(&cat.Name, &cat.Count); err == nil {
				cat.Percent = float64(cat.Count) / float64(normalizedCount) * 100
				topCategories = append(topCategories, cat)
			}
		}
	}

	// Примеры
	type Example struct {
		Source      string
		Normalized  string
		Category    string
		Code        string
	}
	var examples []Example
	exampleQuery := `
		SELECT source_name, normalized_name, category, code
		FROM normalized_data
		WHERE category IS NOT NULL AND category != ''
		ORDER BY category, id
		LIMIT 30
	`
	exampleRows, _ := db.Query(exampleQuery)
	if exampleRows != nil {
		defer exampleRows.Close()
		for exampleRows.Next() {
			var ex Example
			if err := exampleRows.Scan(&ex.Source, &ex.Normalized, &ex.Category, &ex.Code); err == nil {
				examples = append(examples, ex)
			}
		}
	}

	// Генерируем HTML
	htmlTemplate := `<!DOCTYPE html>
<html lang="ru">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Отчет о нормализации и классификации</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 20px; background: #f5f5f5; }
        .container { max-width: 1200px; margin: 0 auto; background: white; padding: 30px; border-radius: 10px; box-shadow: 0 2px 10px rgba(0,0,0,0.1); }
        h1 { color: #2c3e50; border-bottom: 3px solid #3498db; padding-bottom: 10px; }
        h2 { color: #34495e; margin-top: 30px; }
        .stats { display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: 20px; margin: 20px 0; }
        .stat-card { background: #ecf0f1; padding: 20px; border-radius: 8px; text-align: center; }
        .stat-value { font-size: 2em; font-weight: bold; color: #3498db; }
        .stat-label { color: #7f8c8d; margin-top: 5px; }
        table { width: 100%; border-collapse: collapse; margin: 20px 0; }
        th, td { padding: 12px; text-align: left; border-bottom: 1px solid #ddd; }
        th { background: #3498db; color: white; }
        tr:hover { background: #f5f5f5; }
        .example { background: #fff; padding: 15px; margin: 10px 0; border-left: 4px solid #3498db; border-radius: 4px; }
        .category { color: #27ae60; font-weight: bold; }
        .progress { background: #ecf0f1; border-radius: 10px; height: 30px; margin: 10px 0; overflow: hidden; }
        .progress-bar { background: #3498db; height: 100%; display: flex; align-items: center; justify-content: center; color: white; font-weight: bold; }
        .timestamp { color: #95a5a6; font-size: 0.9em; margin-top: 20px; }
    </style>
</head>
<body>
    <div class="container">
        <h1>📊 Отчет о нормализации и классификации</h1>
        <div class="timestamp">Сгенерировано: {{.Timestamp}}</div>

        <h2>📈 Общая статистика</h2>
        <div class="stats">
            <div class="stat-card">
                <div class="stat-value">{{.TotalItems}}</div>
                <div class="stat-label">Всего элементов</div>
            </div>
            <div class="stat-card">
                <div class="stat-value">{{.NormalizedCount}}</div>
                <div class="stat-label">Нормализовано</div>
            </div>
            <div class="stat-card">
                <div class="stat-value">{{.UniqueCategories}}</div>
                <div class="stat-label">Уникальных категорий</div>
            </div>
            <div class="stat-card">
                <div class="stat-value">{{.KpvedCount}}</div>
                <div class="stat-label">С КПВЭД</div>
            </div>
        </div>

        <h2>📋 Прогресс нормализации</h2>
        <div class="progress">
            <div class="progress-bar" style="width: {{.NormalizedPercent}}%">{{.NormalizedPercent}}%</div>
        </div>

        <h2>🏷️ Прогресс классификации КПВЭД</h2>
        <div class="progress">
            <div class="progress-bar" style="width: {{.KpvedPercent}}%">{{.KpvedPercent}}%</div>
        </div>

        <h2>📊 Топ-20 категорий</h2>
        <table>
            <thead>
                <tr>
                    <th>№</th>
                    <th>Категория</th>
                    <th>Количество</th>
                    <th>Процент</th>
                </tr>
            </thead>
            <tbody>
                {{range $i, $cat := .TopCategories}}
                <tr>
                    <td>{{add $i 1}}</td>
                    <td>{{$cat.Name}}</td>
                    <td>{{$cat.Count}}</td>
                    <td>{{printf "%.1f" $cat.Percent}}%</td>
                </tr>
                {{end}}
            </tbody>
        </table>

        <h2>📦 Примеры нормализованных записей</h2>
        {{range .Examples}}
        <div class="example">
            <strong>Исходное:</strong> {{.Source}}<br>
            <strong>Нормализованное:</strong> {{.Normalized}}<br>
            <span class="category">Категория:</span> {{.Category}} | <strong>Код:</strong> {{.Code}}
        </div>
        {{end}}

        <h2>✨ Дополнительная статистика</h2>
        <ul>
            <li>Изменено названий: {{.ChangedCount}} ({{printf "%.1f" .ChangedPercent}}%)</li>
            <li>Записей с объединением: {{.MergedCount}}</li>
        </ul>
    </div>
</body>
</html>`

	tmpl, err := template.New("report").Funcs(template.FuncMap{
		"add": func(a, b int) int { return a + b },
		"printf": fmt.Sprintf,
	}).Parse(htmlTemplate)
	if err != nil {
		log.Fatalf("Ошибка парсинга шаблона: %v", err)
	}

	data := struct {
		Timestamp         string
		TotalItems        int
		NormalizedCount   int
		NormalizedPercent float64
		KpvedCount        int
		KpvedPercent      float64
		UniqueCategories  int
		ChangedCount      int
		ChangedPercent    float64
		MergedCount       int
		TopCategories     []CategoryStat
		Examples          []Example
	}{
		Timestamp:         time.Now().Format("2006-01-02 15:04:05"),
		TotalItems:        totalItems,
		NormalizedCount:   normalizedCount,
		NormalizedPercent: float64(normalizedCount) / float64(totalItems) * 100,
		KpvedCount:        kpvedCount,
		KpvedPercent:      float64(kpvedCount) / float64(normalizedCount) * 100,
		UniqueCategories:  uniqueCategories,
		ChangedCount:      changedCount,
		ChangedPercent:    float64(changedCount) / float64(normalizedCount) * 100,
		MergedCount:       mergedCount,
		TopCategories:     topCategories,
		Examples:          examples,
	}

	file, err := os.Create(outputFile)
	if err != nil {
		log.Fatalf("Ошибка создания файла: %v", err)
	}
	defer file.Close()

	if err := tmpl.Execute(file, data); err != nil {
		log.Fatalf("Ошибка генерации HTML: %v", err)
	}

	fmt.Printf("✅ Отчет успешно создан: %s\n", outputFile)
	fmt.Printf("   Всего элементов: %d\n", totalItems)
	fmt.Printf("   Нормализовано: %d (%.1f%%)\n", normalizedCount, data.NormalizedPercent)
	fmt.Printf("   Классифицировано по КПВЭД: %d (%.1f%%)\n", kpvedCount, data.KpvedPercent)
}

