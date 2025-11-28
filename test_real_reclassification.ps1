# Полный тест переклассификации с реальными данными

$baseUrl = "http://localhost:9999"
$timeout = 7

Write-Host "╔══════════════════════════════════════════════════════════════╗" -ForegroundColor Cyan
Write-Host "║     ТЕСТ ПЕРЕКЛАССИФИКАЦИИ С РЕАЛЬНЫМИ ДАННЫМИ               ║" -ForegroundColor Cyan
Write-Host "╚══════════════════════════════════════════════════════════════╝" -ForegroundColor Cyan
Write-Host ""

# 1. Проверка доступности сервера
Write-Host "1. Проверка доступности сервера..." -ForegroundColor Yellow
try {
    $healthResponse = Invoke-RestMethod -Uri "$baseUrl/health" -Method GET -TimeoutSec $timeout
    Write-Host "   ✅ Сервер доступен" -ForegroundColor Green
} catch {
    Write-Host "   ❌ Сервер недоступен: $_" -ForegroundColor Red
    Write-Host "   Запустите сервер: .\start_server_with_api_key.ps1" -ForegroundColor Yellow
    exit 1
}
Write-Host ""

# 2. Проверка данных в базе
Write-Host "2. Проверка данных в базе..." -ForegroundColor Yellow
$dbCheck = go run cmd/check_normalized_data/main.go 1c_data.db 2>&1
if ($dbCheck -match "Всего записей: (\d+)") {
    $totalRecords = $matches[1]
    Write-Host "   ✅ Найдено записей: $totalRecords" -ForegroundColor Green
} else {
    Write-Host "   ⚠ Не удалось проверить базу данных" -ForegroundColor Yellow
}
Write-Host ""

# 3. Запуск переклассификации (тест, 5 записей)
Write-Host "3. Запуск переклассификации (тест, 5 записей)..." -ForegroundColor Yellow
$startBody = @{
    classifier_id = 1
    strategy_id = "top_priority"
    limit = 5
} | ConvertTo-Json

try {
    $startResponse = Invoke-RestMethod -Uri "$baseUrl/api/reclassification/start" -Method POST -Body $startBody -ContentType "application/json" -TimeoutSec $timeout
    Write-Host "   ✅ Переклассификация запущена:" -ForegroundColor Green
    Write-Host "      Classifier ID: $($startResponse.classifier_id)" -ForegroundColor Gray
    Write-Host "      Strategy: $($startResponse.strategy_id)" -ForegroundColor Gray
    Write-Host "      Limit: $($startResponse.limit)" -ForegroundColor Gray
} catch {
    Write-Host "   ❌ Ошибка запуска: $_" -ForegroundColor Red
}
Write-Host ""

# 4. Мониторинг прогресса
Write-Host "4. Мониторинг прогресса (максимум 120 секунд)..." -ForegroundColor Yellow
$maxWait = 120
$waited = 0
$checkInterval = 5

while ($waited -lt $maxWait) {
    try {
        $statusResponse = Invoke-RestMethod -Uri "$baseUrl/api/reclassification/status" -Method GET -TimeoutSec $timeout
        
        if (-not $statusResponse.isRunning -and $statusResponse.processed -gt 0) {
            Write-Host "   ✅ Переклассификация завершена!" -ForegroundColor Green
            Write-Host "      Обработано: $($statusResponse.processed)" -ForegroundColor Gray
            Write-Host "      Успешно: $($statusResponse.success)" -ForegroundColor Green
            Write-Host "      Ошибок: $($statusResponse.errors)" -ForegroundColor $(if ($statusResponse.errors -gt 0) { "Red" } else { "Gray" })
            Write-Host "      Скорость: $([math]::Round($statusResponse.rate, 3)) записей/сек" -ForegroundColor Gray
            break
        }
        
        if ($statusResponse.isRunning) {
            $progress = if ($statusResponse.total -gt 0) { 
                [math]::Round(($statusResponse.processed / $statusResponse.total) * 100, 1) 
            } else { 
                0 
            }
            Write-Host "   Прогресс: $progress% ($($statusResponse.processed)/$($statusResponse.total)) | Успешно: $($statusResponse.success) | Ошибок: $($statusResponse.errors)" -ForegroundColor Cyan
        }
    } catch {
        Write-Host "   ❌ Ошибка проверки статуса: $_" -ForegroundColor Red
    }
    
    Start-Sleep -Seconds $checkInterval
    $waited += $checkInterval
}

if ($waited -ge $maxWait) {
    Write-Host "   ⚠ Достигнут лимит ожидания" -ForegroundColor Yellow
}
Write-Host ""

# 5. Финальный статус
Write-Host "5. Финальный статус..." -ForegroundColor Yellow
try {
    $statusResponse = Invoke-RestMethod -Uri "$baseUrl/api/reclassification/status" -Method GET -TimeoutSec $timeout
    Write-Host "   ✅ Финальный статус:" -ForegroundColor Green
    Write-Host "      IsRunning: $($statusResponse.isRunning)" -ForegroundColor Gray
    Write-Host "      Processed: $($statusResponse.processed)/$($statusResponse.total)" -ForegroundColor Gray
    Write-Host "      Success: $($statusResponse.success)" -ForegroundColor Green
    Write-Host "      Errors: $($statusResponse.errors)" -ForegroundColor $(if ($statusResponse.errors -gt 0) { "Red" } else { "Gray" })
    Write-Host "      Rate: $([math]::Round($statusResponse.rate, 3)) записей/сек" -ForegroundColor Gray
    
    if ($statusResponse.logs.Count -gt 0) {
        Write-Host "      Последние 5 логов:" -ForegroundColor Gray
        $statusResponse.logs[-5..-1] | ForEach-Object {
            Write-Host "        $_" -ForegroundColor DarkGray
        }
    }
} catch {
    Write-Host "   ❌ Ошибка получения финального статуса: $_" -ForegroundColor Red
}
Write-Host ""

Write-Host "╔══════════════════════════════════════════════════════════════╗" -ForegroundColor Green
Write-Host "║              ТЕСТИРОВАНИЕ ЗАВЕРШЕНО                          ║" -ForegroundColor Green
Write-Host "╚══════════════════════════════════════════════════════════════╝" -ForegroundColor Green

