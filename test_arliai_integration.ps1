# Тест интеграции с Arliai для процесса нормализации

Write-Host "=== Тест интеграции Arliai для нормализации ===" -ForegroundColor Cyan
Write-Host ""

# Проверка наличия API ключа
$apiKey = $env:ARLIAI_API_KEY
if (-not $apiKey) {
    Write-Host "⚠ ARLIAI_API_KEY не установлен" -ForegroundColor Yellow
    Write-Host "Тестирование будет проводиться только в режиме без AI (fallback на правила)" -ForegroundColor Yellow
    Write-Host ""
    $useAI = $false
} else {
    Write-Host "✓ ARLIAI_API_KEY найден" -ForegroundColor Green
    Write-Host "Тестирование будет проводиться с AI и без AI" -ForegroundColor Green
    Write-Host ""
    $useAI = $true
}

# URL сервера
$baseUrl = "http://localhost:9999"

# Функция для отправки запроса
function Send-Request {
    param(
        [string]$Method,
        [string]$Endpoint,
        [object]$Body = $null,
        [int]$Timeout = 7
    )
    
    $url = "$baseUrl$Endpoint"
    $headers = @{
        "Content-Type" = "application/json"
    }
    
    try {
        if ($Body) {
            $jsonBody = $Body | ConvertTo-Json -Depth 10
            $response = Invoke-RestMethod -Uri $url -Method $Method -Headers $headers -Body $jsonBody -TimeoutSec $Timeout
        } else {
            $response = Invoke-RestMethod -Uri $url -Method $Method -Headers $headers -TimeoutSec $Timeout
        }
        return @{ Success = $true; Data = $response }
    } catch {
        return @{ Success = $false; Error = $_.Exception.Message }
    }
}

# Тест 1: Проверка статуса сервера
Write-Host "Тест 1: Проверка статуса сервера" -ForegroundColor Cyan
$result = Send-Request -Method "GET" -Endpoint "/api/normalize/status"
if ($result.Success) {
    Write-Host "✓ Сервер доступен" -ForegroundColor Green
    Write-Host "  Статус: $($result.Data.status)" -ForegroundColor Gray
} else {
    Write-Host "✗ Сервер недоступен: $($result.Error)" -ForegroundColor Red
    Write-Host "Убедитесь, что сервер запущен на $baseUrl" -ForegroundColor Yellow
    exit 1
}
Write-Host ""

# Тест 2: Нормализация БЕЗ AI (fallback на правила)
Write-Host "Тест 2: Нормализация БЕЗ AI (fallback на правила)" -ForegroundColor Cyan
$bodyWithoutAI = @{
    use_ai = $false
    min_confidence = 0.8
    rate_limit_delay_ms = 100
    max_retries = 3
}
$result = Send-Request -Method "POST" -Endpoint "/api/normalize/start" -Body $bodyWithoutAI
if ($result.Success) {
    Write-Host "✓ Запрос на нормализацию без AI отправлен" -ForegroundColor Green
    Write-Host "  Сообщение: $($result.Data.message)" -ForegroundColor Gray
    Write-Host "  Время: $($result.Data.timestamp)" -ForegroundColor Gray
    
    # Ждем немного и проверяем статус
    Start-Sleep -Seconds 2
    $statusResult = Send-Request -Method "GET" -Endpoint "/api/normalize/status"
    if ($statusResult.Success) {
        Write-Host "  Текущий статус: $($statusResult.Data.status)" -ForegroundColor Gray
    }
} else {
    Write-Host "✗ Ошибка при запуске нормализации: $($result.Error)" -ForegroundColor Red
}
Write-Host ""

# Тест 3: Нормализация С AI (если ключ установлен)
if ($useAI) {
    Write-Host "Тест 3: Нормализация С AI" -ForegroundColor Cyan
    $bodyWithAI = @{
        use_ai = $true
        min_confidence = 0.8
        rate_limit_delay_ms = 500
        max_retries = 3
    }
    
    # Сначала останавливаем предыдущий процесс, если он запущен
    Write-Host "  Остановка предыдущего процесса (если запущен)..." -ForegroundColor Gray
    Send-Request -Method "POST" -Endpoint "/api/normalize/stop" | Out-Null
    Start-Sleep -Seconds 1
    
    $result = Send-Request -Method "POST" -Endpoint "/api/normalize/start" -Body $bodyWithAI
    if ($result.Success) {
        Write-Host "✓ Запрос на нормализацию с AI отправлен" -ForegroundColor Green
        Write-Host "  Сообщение: $($result.Data.message)" -ForegroundColor Gray
        Write-Host "  Время: $($result.Data.timestamp)" -ForegroundColor Gray
        
        # Ждем немного и проверяем статус
        Start-Sleep -Seconds 2
        $statusResult = Send-Request -Method "GET" -Endpoint "/api/normalize/status"
        if ($statusResult.Success) {
            Write-Host "  Текущий статус: $($statusResult.Data.status)" -ForegroundColor Gray
        }
    } else {
        Write-Host "✗ Ошибка при запуске нормализации с AI: $($result.Error)" -ForegroundColor Red
    }
    Write-Host ""
} else {
    Write-Host "Тест 3: Пропущен (ARLIAI_API_KEY не установлен)" -ForegroundColor Yellow
    Write-Host ""
}

# Тест 4: Проверка статистики
Write-Host "Тест 4: Проверка статистики нормализации" -ForegroundColor Cyan
$result = Send-Request -Method "GET" -Endpoint "/api/normalize/stats"
if ($result.Success) {
    Write-Host "✓ Статистика получена" -ForegroundColor Green
    $stats = $result.Data
    Write-Host "  Всего записей: $($stats.total_items)" -ForegroundColor Gray
    Write-Host "  Обработано: $($stats.processed)" -ForegroundColor Gray
    Write-Host "  Успешно: $($stats.completed)" -ForegroundColor Gray
    Write-Host "  Ошибки: $($stats.errors)" -ForegroundColor Gray
    Write-Host "  Ожидают: $($stats.pending)" -ForegroundColor Gray
} else {
    Write-Host "✗ Ошибка при получении статистики: $($result.Error)" -ForegroundColor Red
}
Write-Host ""

# Тест 5: Проверка групп
Write-Host "Тест 5: Проверка групп нормализации" -ForegroundColor Cyan
$result = Send-Request -Method "GET" -Endpoint "/api/normalize/groups?limit=5"
if ($result.Success) {
    Write-Host "✓ Группы получены" -ForegroundColor Green
    $groups = $result.Data.groups
    Write-Host "  Количество групп (первые 5): $($groups.Count)" -ForegroundColor Gray
    foreach ($group in $groups | Select-Object -First 3) {
        Write-Host "    - $($group.normalized_name) [$($group.category)] (объединено: $($group.merged_count))" -ForegroundColor Gray
        if ($group.ai_confidence -gt 0) {
            Write-Host "      AI уверенность: $($group.ai_confidence)" -ForegroundColor Cyan
        }
    }
} else {
    Write-Host "✗ Ошибка при получении групп: $($result.Error)" -ForegroundColor Red
}
Write-Host ""

# Тест 6: Проверка обработки ошибок (некорректный запрос)
Write-Host "Тест 6: Проверка обработки ошибок" -ForegroundColor Cyan
$invalidBody = @{
    use_ai = $true
    min_confidence = 1.5  # Некорректное значение (> 1.0)
    rate_limit_delay_ms = -100  # Некорректное значение
}
$result = Send-Request -Method "POST" -Endpoint "/api/normalize/start" -Body $invalidBody
if ($result.Success) {
    Write-Host "✓ Запрос обработан (сервер должен использовать значения по умолчанию)" -ForegroundColor Green
} else {
    Write-Host "⚠ Запрос отклонен: $($result.Error)" -ForegroundColor Yellow
}
Write-Host ""

# Тест 7: Проверка SSE событий (если процесс запущен)
Write-Host "Тест 7: Проверка SSE событий" -ForegroundColor Cyan
Write-Host "  Для проверки SSE событий используйте:" -ForegroundColor Gray
Write-Host "  curl -N http://localhost:9999/api/normalize/events" -ForegroundColor Gray
Write-Host ""

# Итоговый отчет
Write-Host "=== Итоговый отчет ===" -ForegroundColor Cyan
Write-Host ""

# Проверяем финальный статус
$finalStatus = Send-Request -Method "GET" -Endpoint "/api/normalize/status"
if ($finalStatus.Success) {
    $status = $finalStatus.Data.status
    Write-Host "Финальный статус: $status" -ForegroundColor $(if ($status -eq "idle") { "Green" } else { "Yellow" })
}

# Проверяем финальную статистику
$finalStats = Send-Request -Method "GET" -Endpoint "/api/normalize/stats"
if ($finalStats.Success) {
    $stats = $finalStats.Data
    Write-Host ""
    Write-Host "Статистика нормализации:" -ForegroundColor Cyan
    Write-Host "  Всего записей: $($stats.total_items)" -ForegroundColor White
    Write-Host "  Обработано: $($stats.processed)" -ForegroundColor White
    Write-Host "  Успешно: $($stats.completed)" -ForegroundColor $(if ($stats.completed -gt 0) { "Green" } else { "Gray" })
    Write-Host "  Ошибки: $($stats.errors)" -ForegroundColor $(if ($stats.errors -gt 0) { "Red" } else { "Gray" })
    Write-Host "  Ожидают: $($stats.pending)" -ForegroundColor $(if ($stats.pending -gt 0) { "Yellow" } else { "Gray" })
}

Write-Host ""
Write-Host "=== Тестирование завершено ===" -ForegroundColor Cyan
Write-Host ""
Write-Host "Рекомендации:" -ForegroundColor Yellow
Write-Host "1. Проверьте логи сервера для детальной информации" -ForegroundColor Gray
Write-Host "2. Используйте SSE эндпоинт для мониторинга процесса в реальном времени" -ForegroundColor Gray
Write-Host "3. Проверьте таблицу normalized_data в базе данных для результатов" -ForegroundColor Gray
if (-not $useAI) {
    Write-Host "4. Установите ARLIAI_API_KEY для тестирования AI функциональности" -ForegroundColor Gray
}

