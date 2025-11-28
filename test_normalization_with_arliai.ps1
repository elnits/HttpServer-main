# Тест нормализации с ArlAI

$baseUrl = "http://localhost:9999"
$timeout = 7

Write-Host "╔══════════════════════════════════════════════════════════════╗" -ForegroundColor Cyan
Write-Host "║     ТЕСТ НОРМАЛИЗАЦИИ С ARLAI                               ║" -ForegroundColor Cyan
Write-Host "╚══════════════════════════════════════════════════════════════╝" -ForegroundColor Cyan
Write-Host ""

# 1. Проверка доступности сервера
Write-Host "1. Проверка доступности сервера..." -ForegroundColor Yellow
try {
    $healthResponse = Invoke-RestMethod -Uri "$baseUrl/health" -Method GET -TimeoutSec $timeout
    Write-Host "   ✅ Сервер доступен" -ForegroundColor Green
} catch {
    Write-Host "   ❌ Сервер недоступен: $_" -ForegroundColor Red
    exit 1
}
Write-Host ""

# 2. Проверка статуса нормализации
Write-Host "2. Проверка текущего статуса нормализации..." -ForegroundColor Yellow
try {
    $statusResponse = Invoke-RestMethod -Uri "$baseUrl/api/normalization/status" -Method GET -TimeoutSec $timeout
    if ($statusResponse.isRunning) {
        Write-Host "   ⚠ Нормализация уже запущена" -ForegroundColor Yellow
        Write-Host "      Processed: $($statusResponse.processed)/$($statusResponse.total)" -ForegroundColor Gray
    } else {
        Write-Host "   ✅ Нормализация не запущена, можно начать" -ForegroundColor Green
    }
} catch {
    Write-Host "   ⚠ Не удалось получить статус" -ForegroundColor Yellow
}
Write-Host ""

# 3. Запуск нормализации с ArlAI
Write-Host "3. Запуск нормализации с ArlAI..." -ForegroundColor Yellow
$startBody = @{
    use_ai = $true
    min_confidence = 0.7
    rate_limit_delay_ms = 100
    max_retries = 3
} | ConvertTo-Json

try {
    $startResponse = Invoke-RestMethod -Uri "$baseUrl/api/normalize/start" -Method POST -Body $startBody -ContentType "application/json" -TimeoutSec $timeout
    Write-Host "   ✅ Нормализация запущена:" -ForegroundColor Green
    Write-Host "      Success: $($startResponse.success)" -ForegroundColor Gray
    Write-Host "      Message: $($startResponse.message)" -ForegroundColor Gray
    Write-Host "      Timestamp: $($startResponse.timestamp)" -ForegroundColor Gray
} catch {
    Write-Host "   ❌ Ошибка запуска: $_" -ForegroundColor Red
    if ($_.Exception.Response) {
        try {
            $stream = $_.Exception.Response.GetResponseStream()
            $reader = New-Object System.IO.StreamReader($stream)
            $responseBody = $reader.ReadToEnd()
            Write-Host "      Ответ: $responseBody" -ForegroundColor Gray
        } catch {}
    }
    exit 1
}
Write-Host ""

# 4. Мониторинг прогресса
Write-Host "4. Мониторинг прогресса (максимум 300 секунд)..." -ForegroundColor Yellow
$maxWait = 300
$waited = 0
$checkInterval = 5

while ($waited -lt $maxWait) {
    try {
        $statusResponse = Invoke-RestMethod -Uri "$baseUrl/api/normalization/status" -Method GET -TimeoutSec $timeout
        
        if (-not $statusResponse.isRunning -and $statusResponse.processed -gt 0) {
            Write-Host "   ✅ Нормализация завершена!" -ForegroundColor Green
            Write-Host "      Обработано: $($statusResponse.processed)" -ForegroundColor Gray
            Write-Host "      Успешно: $($statusResponse.success)" -ForegroundColor Green
            Write-Host "      Ошибок: $($statusResponse.errors)" -ForegroundColor $(if ($statusResponse.errors -gt 0) { "Red" } else { "Gray" })
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
    $statusResponse = Invoke-RestMethod -Uri "$baseUrl/api/normalization/status" -Method GET -TimeoutSec $timeout
    Write-Host "   ✅ Финальный статус:" -ForegroundColor Green
    Write-Host "      IsRunning: $($statusResponse.isRunning)" -ForegroundColor Gray
    Write-Host "      Processed: $($statusResponse.processed)/$($statusResponse.total)" -ForegroundColor Gray
    Write-Host "      Success: $($statusResponse.success)" -ForegroundColor Green
    Write-Host "      Errors: $($statusResponse.errors)" -ForegroundColor $(if ($statusResponse.errors -gt 0) { "Red" } else { "Gray" })
    
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

# 6. Статистика
Write-Host "6. Статистика нормализации..." -ForegroundColor Yellow
try {
    $statsResponse = Invoke-RestMethod -Uri "$baseUrl/api/normalization/stats" -Method GET -TimeoutSec $timeout
    Write-Host "   ✅ Статистика:" -ForegroundColor Green
    Write-Host "      Всего групп: $($statsResponse.total_groups)" -ForegroundColor Gray
    Write-Host "      Всего записей: $($statsResponse.total_items)" -ForegroundColor Gray
    Write-Host "      Средний размер группы: $([math]::Round($statsResponse.avg_group_size, 2))" -ForegroundColor Gray
} catch {
    Write-Host "   ⚠ Не удалось получить статистику" -ForegroundColor Yellow
}
Write-Host ""

Write-Host "╔══════════════════════════════════════════════════════════════╗" -ForegroundColor Green
Write-Host "║              ТЕСТИРОВАНИЕ ЗАВЕРШЕНО                          ║" -ForegroundColor Green
Write-Host "╚══════════════════════════════════════════════════════════════╝" -ForegroundColor Green

