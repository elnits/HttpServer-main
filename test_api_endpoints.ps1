# Скрипт для тестирования API эндпоинтов версионирования и классификации

$baseUrl = "http://localhost:9999"
$results = @()

# Функция для выполнения запроса
function Test-Endpoint {
    param(
        [string]$Name,
        [string]$Method,
        [string]$Url,
        [string]$Body = $null,
        [string]$Description
    )
    
    Write-Host "`n=== Тестирование: $Name ===" -ForegroundColor Cyan
    Write-Host "Описание: $Description" -ForegroundColor Gray
    Write-Host "Метод: $Method" -ForegroundColor Gray
    Write-Host "URL: $Url" -ForegroundColor Gray
    
    try {
        $headers = @{
            "Content-Type" = "application/json"
        }
        
        if ($Body) {
            Write-Host "Тело запроса: $Body" -ForegroundColor Gray
            $response = Invoke-WebRequest -Uri $Url -Method $Method -Headers $headers -Body $Body -TimeoutSec 7 -ErrorAction Stop
        } else {
            $response = Invoke-WebRequest -Uri $Url -Method $Method -Headers $headers -TimeoutSec 7 -ErrorAction Stop
        }
        
        $statusCode = $response.StatusCode
        $content = $response.Content | ConvertFrom-Json
        
        Write-Host "Статус: $statusCode" -ForegroundColor Green
        Write-Host "Ответ: $($content | ConvertTo-Json -Depth 10)" -ForegroundColor Green
        
        $results += [PSCustomObject]@{
            Name = $Name
            Method = $Method
            URL = $Url
            Request = $Body
            Status = $statusCode
            Response = ($content | ConvertTo-Json -Depth 10)
            Success = $true
            Error = $null
        }
        
        return $content
    }
    catch {
        $statusCode = $_.Exception.Response.StatusCode.value__
        $errorMsg = $_.Exception.Message
        
        Write-Host "Ошибка: $errorMsg" -ForegroundColor Red
        Write-Host "Статус: $statusCode" -ForegroundColor Red
        
        $results += [PSCustomObject]@{
            Name = $Name
            Method = $Method
            URL = $Url
            Request = $Body
            Status = $statusCode
            Response = $null
            Success = $false
            Error = $errorMsg
        }
        
        return $null
    }
}

# 1. Начало сессии нормализации
$startBody = @{
    item_id = 1
    original_name = "МОЛОТАК СТРОИТЕЛЬНЫЙ 500гр ER-00013004"
} | ConvertTo-Json

$sessionResult = Test-Endpoint -Name "Start Normalization" `
    -Method "POST" `
    -Url "$baseUrl/api/normalization/start" `
    -Body $startBody `
    -Description "Начинает новую сессию нормализации для элемента"

$sessionId = $null
if ($sessionResult -and $sessionResult.session_id) {
    $sessionId = $sessionResult.session_id
    Write-Host "Создана сессия с ID: $sessionId" -ForegroundColor Yellow
}

# 2. Применение паттернов
if ($sessionId) {
    $patternsBody = @{
        session_id = $sessionId
        stage_type = "pattern"
    } | ConvertTo-Json
    
    Test-Endpoint -Name "Apply Patterns" `
        -Method "POST" `
        -Url "$baseUrl/api/normalization/apply-patterns" `
        -Body $patternsBody `
        -Description "Применяет алгоритмические паттерны для исправления названия"
    
    # 3. Применение AI коррекции
    $aiBody = @{
        session_id = $sessionId
        stage_type = "ai"
        use_chat = $false
        context = @("Исправить грамматику")
    } | ConvertTo-Json
    
    Test-Endpoint -Name "Apply AI Correction" `
        -Method "POST" `
        -Url "$baseUrl/api/normalization/apply-ai" `
        -Body $aiBody `
        -Description "Применяет AI коррекцию для улучшения названия"
    
    # 4. Получение истории сессии
    Test-Endpoint -Name "Get Session History" `
        -Method "GET" `
        -Url "$baseUrl/api/normalization/history?session_id=$sessionId" `
        -Description "Получает полную историю стадий для сессии"
    
    # 5. Откат к стадии
    $revertBody = @{
        session_id = $sessionId
        target_stage = 1
    } | ConvertTo-Json
    
    Test-Endpoint -Name "Revert Stage" `
        -Method "POST" `
        -Url "$baseUrl/api/normalization/revert" `
        -Body $revertBody `
        -Description "Откатывает сессию к указанной стадии"
}

# 6. Получение списка стратегий классификации
Test-Endpoint -Name "Get Strategies" `
    -Method "GET" `
    -Url "$baseUrl/api/classification/strategies" `
    -Description "Получает список доступных стратегий свертки категорий"

# 7. Настройка стратегии
$strategyBody = @{
    client_id = 1
    max_depth = 2
    priority = @("0", "1")
    rules = @()
    name = "Тестовая стратегия"
    description = "Тестовая стратегия для проверки"
} | ConvertTo-Json

Test-Endpoint -Name "Configure Strategy" `
    -Method "POST" `
    -Url "$baseUrl/api/classification/strategies/configure" `
    -Body $strategyBody `
    -Description "Настраивает новую стратегию свертки категорий"

# 8. Классификация товара
if ($sessionId) {
    $classifyBody = @{
        session_id = $sessionId
        strategy_id = "top_priority"
    } | ConvertTo-Json
    
    Test-Endpoint -Name "Classify Item" `
        -Method "POST" `
        -Url "$baseUrl/api/classification/classify" `
        -Body $classifyBody `
        -Description "Классифицирует товар с использованием AI и применяет стратегию свертки"
}

# Вывод результатов
Write-Host "`n=== ИТОГИ ТЕСТИРОВАНИЯ ===" -ForegroundColor Cyan
$successCount = ($results | Where-Object { $_.Success }).Count
$failCount = ($results | Where-Object { -not $_.Success }).Count

Write-Host "Успешно: $successCount" -ForegroundColor Green
Write-Host "Ошибок: $failCount" -ForegroundColor $(if ($failCount -gt 0) { "Red" } else { "Green" })

# Сохранение результатов в переменную для экспорта
$global:testResults = $results

