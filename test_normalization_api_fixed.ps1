# Тестирование API нормализации
$ErrorActionPreference = "Stop"

Write-Host "=== Тестирование API нормализации ===" -ForegroundColor Cyan
Write-Host ""

$baseUrl = "http://localhost:9999"
$timeout = 7

# Функция для выполнения curl запросов
function Test-Endpoint {
    param(
        [string]$Method,
        [string]$Url,
        [string]$Body = $null,
        [string]$Description
    )
    
    Write-Host "Тест: $Description" -ForegroundColor Yellow
    Write-Host "  URL: $Method $Url" -ForegroundColor Gray
    
    try {
        $headers = @{
            "Content-Type" = "application/json"
        }
        
        $params = @{
            Uri = $Url
            Method = $Method
            Headers = $headers
            TimeoutSec = $timeout
            ErrorAction = "Stop"
        }
        
        if ($Body) {
            $params.Body = $Body
            Write-Host "  Body: $Body" -ForegroundColor Gray
        }
        
        $response = Invoke-RestMethod @params
        
        Write-Host "  ✓ Успешно" -ForegroundColor Green
        Write-Host "  Response:" -ForegroundColor Gray
        $response | ConvertTo-Json -Depth 10 | Write-Host
        Write-Host ""
        return $response
    }
    catch {
        Write-Host "  ✗ Ошибка: $($_.Exception.Message)" -ForegroundColor Red
        if ($_.ErrorDetails.Message) {
            Write-Host "  Details: $($_.ErrorDetails.Message)" -ForegroundColor Red
        }
        Write-Host ""
        return $null
    }
}

# 1. Проверка статуса нормализации
Write-Host "1. Проверка статуса нормализации" -ForegroundColor Cyan
$status = Test-Endpoint -Method "GET" -Url "$baseUrl/api/normalization/status" -Description "Получение статуса нормализации"

# 2. Проверка статистики нормализации
Write-Host "2. Проверка статистики нормализации" -ForegroundColor Cyan
$stats = Test-Endpoint -Method "GET" -Url "$baseUrl/api/normalization/stats" -Description "Получение статистики нормализации"

# 3. Проверка списка групп (первая страница)
Write-Host "3. Проверка списка групп" -ForegroundColor Cyan
$groupsUrl = "$baseUrl/api/normalization/groups?page=1&limit=5"
$groups = Test-Endpoint -Method "GET" -Url $groupsUrl -Description "Получение списка групп (первая страница)"

# 4. Запуск нормализации (без AI)
Write-Host "4. Запуск нормализации (без AI)" -ForegroundColor Cyan
$startBody = @{
    use_ai = $false
    min_confidence = 0.8
    database = ""
} | ConvertTo-Json

$startResult = Test-Endpoint -Method "POST" -Url "$baseUrl/api/normalize/start" -Body $startBody -Description "Запуск нормализации без AI"

# 5. Проверка статуса после запуска
if ($startResult) {
    Write-Host "5. Проверка статуса после запуска" -ForegroundColor Cyan
    Start-Sleep -Seconds 2
    $statusAfter = Test-Endpoint -Method "GET" -Url "$baseUrl/api/normalization/status" -Description "Статус после запуска"
}

# 6. Проверка конфигурации нормализации
Write-Host "6. Проверка конфигурации нормализации" -ForegroundColor Cyan
$config = Test-Endpoint -Method "GET" -Url "$baseUrl/api/normalization/config" -Description "Получение конфигурации нормализации"

Write-Host "=== Тестирование завершено ===" -ForegroundColor Cyan


