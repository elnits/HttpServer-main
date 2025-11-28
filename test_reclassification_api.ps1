# Тестирование API переклассификации

$baseUrl = "http://localhost:9999"
$timeout = 7

Write-Host "╔══════════════════════════════════════════════════════════════╗" -ForegroundColor Cyan
Write-Host "║     ТЕСТИРОВАНИЕ API ПЕРЕКЛАССИФИКАЦИИ                       ║" -ForegroundColor Cyan
Write-Host "╚══════════════════════════════════════════════════════════════╝" -ForegroundColor Cyan
Write-Host ""

# Проверка доступности сервера
Write-Host "1. Проверка доступности сервера..." -ForegroundColor Yellow
try {
    $healthResponse = Invoke-RestMethod -Uri "$baseUrl/health" -Method GET -TimeoutSec $timeout
    Write-Host "   ✅ Сервер доступен" -ForegroundColor Green
} catch {
    Write-Host "   ❌ Сервер недоступен: $_" -ForegroundColor Red
    Write-Host "   Запустите сервер: go run ." -ForegroundColor Yellow
    exit 1
}
Write-Host ""

# Проверка статуса переклассификации (должен быть пустым)
Write-Host "2. Проверка начального статуса..." -ForegroundColor Yellow
try {
    $statusResponse = Invoke-RestMethod -Uri "$baseUrl/api/reclassification/status" -Method GET -TimeoutSec $timeout
    Write-Host "   ✅ Статус получен:" -ForegroundColor Green
    Write-Host "      IsRunning: $($statusResponse.isRunning)" -ForegroundColor Gray
    Write-Host "      Progress: $($statusResponse.progress)%" -ForegroundColor Gray
    Write-Host "      Processed: $($statusResponse.processed)/$($statusResponse.total)" -ForegroundColor Gray
} catch {
    Write-Host "   ❌ Ошибка получения статуса: $_" -ForegroundColor Red
}
Write-Host ""

# Запуск переклассификации (тест с лимитом 10 записей)
Write-Host "3. Запуск переклассификации (тест, 10 записей)..." -ForegroundColor Yellow
$startBody = @{
    classifier_id = 1
    strategy_id = "top_priority"
    limit = 10
} | ConvertTo-Json

try {
    $startResponse = Invoke-RestMethod -Uri "$baseUrl/api/reclassification/start" -Method POST -Body $startBody -ContentType "application/json" -TimeoutSec $timeout
    Write-Host "   ✅ Переклассификация запущена:" -ForegroundColor Green
    Write-Host "      Classifier ID: $($startResponse.classifier_id)" -ForegroundColor Gray
    Write-Host "      Strategy: $($startResponse.strategy_id)" -ForegroundColor Gray
    Write-Host "      Limit: $($startResponse.limit)" -ForegroundColor Gray
} catch {
    Write-Host "   ❌ Ошибка запуска: $_" -ForegroundColor Red
    if ($_.Exception.Response) {
        try {
            $stream = $_.Exception.Response.GetResponseStream()
            $reader = New-Object System.IO.StreamReader($stream)
            $responseBody = $reader.ReadToEnd()
            Write-Host "      Ответ сервера: $responseBody" -ForegroundColor Gray
        } catch {
            Write-Host "      Не удалось прочитать ответ сервера" -ForegroundColor Gray
        }
    }
}
Write-Host ""

# Ожидание немного
Write-Host "4. Ожидание 3 секунды..." -ForegroundColor Yellow
Start-Sleep -Seconds 3
Write-Host ""

# Проверка статуса после запуска
Write-Host "5. Проверка статуса после запуска..." -ForegroundColor Yellow
try {
    $statusResponse = Invoke-RestMethod -Uri "$baseUrl/api/reclassification/status" -Method GET -TimeoutSec $timeout
    Write-Host "   ✅ Статус:" -ForegroundColor Green
    Write-Host "      IsRunning: $($statusResponse.isRunning)" -ForegroundColor Gray
    Write-Host "      Progress: $($statusResponse.progress)%" -ForegroundColor Gray
    Write-Host "      Processed: $($statusResponse.processed)/$($statusResponse.total)" -ForegroundColor Gray
    Write-Host "      Success: $($statusResponse.success)" -ForegroundColor Gray
    Write-Host "      Errors: $($statusResponse.errors)" -ForegroundColor Gray
    Write-Host "      Rate: $($statusResponse.rate) записей/сек" -ForegroundColor Gray
    Write-Host "      Current Step: $($statusResponse.currentStep)" -ForegroundColor Gray
    
    if ($statusResponse.logs.Count -gt 0) {
        Write-Host "      Последние логи:" -ForegroundColor Gray
        $statusResponse.logs[-5..-1] | ForEach-Object {
            Write-Host "        - $_" -ForegroundColor DarkGray
        }
    }
} catch {
    Write-Host "   ❌ Ошибка получения статуса: $_" -ForegroundColor Red
}
Write-Host ""

# Ожидание завершения (максимум 60 секунд)
Write-Host "6. Ожидание завершения (максимум 60 секунд)..." -ForegroundColor Yellow
$maxWait = 60
$waited = 0
$checkInterval = 2

while ($waited -lt $maxWait) {
    try {
        $statusResponse = Invoke-RestMethod -Uri "$baseUrl/api/reclassification/status" -Method GET -TimeoutSec $timeout
        if (-not $statusResponse.isRunning) {
            Write-Host "   ✅ Переклассификация завершена!" -ForegroundColor Green
            Write-Host "      Обработано: $($statusResponse.processed)" -ForegroundColor Gray
            Write-Host "      Успешно: $($statusResponse.success)" -ForegroundColor Gray
            Write-Host "      Ошибок: $($statusResponse.errors)" -ForegroundColor Gray
            break
        }
        Write-Host "   Прогресс: $($statusResponse.progress.ToString('F1'))% ($($statusResponse.processed)/$($statusResponse.total))" -ForegroundColor Cyan
    } catch {
        Write-Host "   ❌ Ошибка проверки статуса: $_" -ForegroundColor Red
    }
    Start-Sleep -Seconds $checkInterval
    $waited += $checkInterval
}

if ($waited -ge $maxWait) {
    Write-Host "   ⚠ Достигнут лимит ожидания, останавливаем..." -ForegroundColor Yellow
    try {
        $stopResponse = Invoke-RestMethod -Uri "$baseUrl/api/reclassification/stop" -Method POST -TimeoutSec $timeout
        Write-Host "   ✅ Процесс остановлен" -ForegroundColor Green
    } catch {
        Write-Host "   ❌ Ошибка остановки: $_" -ForegroundColor Red
    }
}
Write-Host ""

# Финальная проверка статуса
Write-Host "7. Финальная проверка статуса..." -ForegroundColor Yellow
try {
    $statusResponse = Invoke-RestMethod -Uri "$baseUrl/api/reclassification/status" -Method GET -TimeoutSec $timeout
    Write-Host "   ✅ Финальный статус:" -ForegroundColor Green
    Write-Host "      IsRunning: $($statusResponse.isRunning)" -ForegroundColor Gray
    Write-Host "      Progress: $($statusResponse.progress)%" -ForegroundColor Gray
    Write-Host "      Processed: $($statusResponse.processed)/$($statusResponse.total)" -ForegroundColor Gray
    Write-Host "      Success: $($statusResponse.success)" -ForegroundColor Gray
    Write-Host "      Errors: $($statusResponse.errors)" -ForegroundColor Gray
    Write-Host "      Rate: $($statusResponse.rate) записей/сек" -ForegroundColor Gray
} catch {
    Write-Host "   ❌ Ошибка получения финального статуса: $_" -ForegroundColor Red
}
Write-Host ""

Write-Host "╔══════════════════════════════════════════════════════════════╗" -ForegroundColor Green
Write-Host "║              ТЕСТИРОВАНИЕ ЗАВЕРШЕНО                          ║" -ForegroundColor Green
Write-Host "╚══════════════════════════════════════════════════════════════╝" -ForegroundColor Green


