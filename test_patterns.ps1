# Скрипт для тестирования паттернов на реальных данных из базы

param(
    [string]$DatabasePath = "1c_data.db",
    [int]$Limit = 50,
    [switch]$UseAI
)

Write-Host "=== Тестирование системы паттернов ===" -ForegroundColor Cyan
Write-Host "База данных: $DatabasePath" -ForegroundColor Yellow
Write-Host "Лимит записей: $Limit" -ForegroundColor Yellow
Write-Host "Использование AI: $UseAI" -ForegroundColor Yellow
Write-Host ""

# Проверяем наличие базы данных
if (-not (Test-Path $DatabasePath)) {
    Write-Host "Ошибка: База данных '$DatabasePath' не найдена!" -ForegroundColor Red
    exit 1
}

# Формируем URL для API
$baseUrl = "http://localhost:8080"
$testBatchUrl = "$baseUrl/api/patterns/test-batch"

# Формируем тело запроса
$body = @{
    limit = $Limit
    use_ai = $UseAI.IsPresent
    table = "catalog_items"
    column = "name"
} | ConvertTo-Json

Write-Host "Отправка запроса к API..." -ForegroundColor Green

try {
    $response = Invoke-RestMethod -Uri $testBatchUrl -Method Post -Body $body -ContentType "application/json" -TimeoutSec 30
    
    Write-Host "`n=== РЕЗУЛЬТАТЫ ===" -ForegroundColor Cyan
    Write-Host "Всего проанализировано: $($response.total_analyzed)" -ForegroundColor Green
    
    if ($response.statistics) {
        $stats = $response.statistics
        Write-Host "`nСтатистика:" -ForegroundColor Yellow
        Write-Host "  Всего паттернов найдено: $($stats.total_patterns)"
        Write-Host "  Записей с паттернами: $($stats.items_with_patterns)"
        Write-Host "  Требуют проверки: $($stats.items_requiring_review)"
        Write-Host "  Автоприменяемых паттернов: $($stats.auto_fixable_patterns)"
        
        if ($stats.avg_patterns_per_item) {
            Write-Host "  Среднее паттернов на запись: $([math]::Round($stats.avg_patterns_per_item, 2))"
        }
        
        if ($stats.patterns_by_type) {
            Write-Host "`nПаттерны по типам:" -ForegroundColor Yellow
            $stats.patterns_by_type.PSObject.Properties | ForEach-Object {
                Write-Host "  $($_.Name): $($_.Value)"
            }
        }
        
        if ($stats.patterns_by_severity) {
            Write-Host "`nПаттерны по серьезности:" -ForegroundColor Yellow
            $stats.patterns_by_severity.PSObject.Properties | ForEach-Object {
                Write-Host "  $($_.Name): $($_.Value)"
            }
        }
    }
    
    # Показываем примеры с паттернами
    Write-Host "`n=== ПРИМЕРЫ НАЙДЕННЫХ ПАТТЕРНОВ ===" -ForegroundColor Cyan
    
    $examplesShown = 0
    $maxExamples = 10
    
    foreach ($result in $response.results) {
        if ($result.patterns_found -gt 0 -and $examplesShown -lt $maxExamples) {
            Write-Host "`nОригинал: $($result.original_name)" -ForegroundColor White
            Write-Host "  Найдено паттернов: $($result.patterns_found)" -ForegroundColor Yellow
            
            if ($result.patterns) {
                foreach ($pattern in $result.patterns) {
                    Write-Host "    - [$($pattern.severity)] $($pattern.description): '$($pattern.matched_text)'" -ForegroundColor Gray
                }
            }
            
            Write-Host "  Алгоритмическое исправление: $($result.algorithmic_fix)" -ForegroundColor Green
            
            if ($result.final_suggestion) {
                Write-Host "  Финальное предложение: $($result.final_suggestion)" -ForegroundColor Cyan
            }
            
            if ($result.confidence) {
                Write-Host "  Уверенность: $([math]::Round($result.confidence * 100, 1))%" -ForegroundColor Magenta
            }
            
            $examplesShown++
        }
    }
    
    # Сохраняем результаты в файл
    $outputFile = "pattern_test_results_$(Get-Date -Format 'yyyyMMdd_HHmmss').json"
    $response | ConvertTo-Json -Depth 10 | Out-File -FilePath $outputFile -Encoding UTF8
    Write-Host "`nРезультаты сохранены в: $outputFile" -ForegroundColor Green
    
} catch {
    Write-Host "`nОшибка при выполнении запроса:" -ForegroundColor Red
    Write-Host $_.Exception.Message -ForegroundColor Red
    
    if ($_.Exception.Response) {
        $reader = New-Object System.IO.StreamReader($_.Exception.Response.GetResponseStream())
        $responseBody = $reader.ReadToEnd()
        Write-Host "Ответ сервера: $responseBody" -ForegroundColor Red
    }
    
    Write-Host "`nУбедитесь, что:" -ForegroundColor Yellow
    Write-Host "  1. Сервер запущен на порту 8080" -ForegroundColor Yellow
    Write-Host "  2. База данных доступна" -ForegroundColor Yellow
    Write-Host "  3. API endpoint /api/patterns/test-batch доступен" -ForegroundColor Yellow
    
    exit 1
}

Write-Host "`n=== ТЕСТИРОВАНИЕ ЗАВЕРШЕНО ===" -ForegroundColor Cyan

