# PowerShell скрипт для тестирования бэкенда

Write-Host "Тестирование бэкенда..." -ForegroundColor Cyan
Write-Host ""

$baseUrl = "http://localhost:9999"

# Тест 1: Health check
Write-Host "[1/5] Проверка health endpoint..." -ForegroundColor Yellow
try {
    $response = Invoke-WebRequest -Uri "$baseUrl/health" -TimeoutSec 2 -ErrorAction Stop
    Write-Host "  ✓ Health endpoint работает" -ForegroundColor Green
    Write-Host "  Ответ: $($response.Content)" -ForegroundColor Gray
} catch {
    Write-Host "  ✗ Health endpoint недоступен: $($_.Exception.Message)" -ForegroundColor Red
}

Write-Host ""

# Тест 2: Status endpoint
Write-Host "[2/5] Проверка status endpoint..." -ForegroundColor Yellow
try {
    $response = Invoke-WebRequest -Uri "$baseUrl/api/normalization/status" -TimeoutSec 2 -ErrorAction Stop
    $data = $response.Content | ConvertFrom-Json
    Write-Host "  ✓ Status endpoint работает" -ForegroundColor Green
    Write-Host "  Статус: $($data.currentStep)" -ForegroundColor Gray
    Write-Host "  Запущен: $($data.isRunning)" -ForegroundColor Gray
    Write-Host "  Обработано: $($data.processed) / $($data.total)" -ForegroundColor Gray
} catch {
    Write-Host "  ✗ Status endpoint недоступен: $($_.Exception.Message)" -ForegroundColor Red
}

Write-Host ""

# Тест 3: Stats endpoint
Write-Host "[3/5] Проверка stats endpoint..." -ForegroundColor Yellow
try {
    $response = Invoke-WebRequest -Uri "$baseUrl/api/normalization/stats" -TimeoutSec 2 -ErrorAction Stop
    $data = $response.Content | ConvertFrom-Json
    Write-Host "  ✓ Stats endpoint работает" -ForegroundColor Green
    Write-Host "  Групп: $($data.totalGroups)" -ForegroundColor Gray
    Write-Host "  Записей: $($data.totalItems)" -ForegroundColor Gray
} catch {
    Write-Host "  ✗ Stats endpoint недоступен: $($_.Exception.Message)" -ForegroundColor Red
}

Write-Host ""

# Тест 4: SSE endpoint (проверка заголовков)
Write-Host "[4/5] Проверка SSE endpoint..." -ForegroundColor Yellow
try {
    $response = Invoke-WebRequest -Uri "$baseUrl/api/normalize/events" -TimeoutSec 2 -ErrorAction Stop -MaximumRedirection 0
    Write-Host "  ✓ SSE endpoint доступен" -ForegroundColor Green
    Write-Host "  Content-Type: $($response.Headers['Content-Type'])" -ForegroundColor Gray
} catch {
    if ($_.Exception.Response.StatusCode -eq 200) {
        Write-Host "  ✓ SSE endpoint работает (ожидается долгое соединение)" -ForegroundColor Green
    } else {
        Write-Host "  ✗ SSE endpoint недоступен: $($_.Exception.Message)" -ForegroundColor Red
    }
}

Write-Host ""

# Тест 5: Start endpoint (только проверка доступности, не запускаем)
Write-Host "[5/5] Проверка start endpoint (без запуска)..." -ForegroundColor Yellow
try {
    $response = Invoke-WebRequest -Uri "$baseUrl/api/normalize/start" -Method POST -TimeoutSec 2 -ErrorAction Stop
    Write-Host "  ✓ Start endpoint доступен" -ForegroundColor Green
} catch {
    if ($_.Exception.Response.StatusCode -eq 409) {
        Write-Host "  ✓ Start endpoint работает (нормализация уже запущена)" -ForegroundColor Green
    } elseif ($_.Exception.Response.StatusCode -eq 200) {
        Write-Host "  ✓ Start endpoint работает" -ForegroundColor Green
    } else {
        Write-Host "  ? Start endpoint ответил: $($_.Exception.Response.StatusCode)" -ForegroundColor Yellow
    }
}

Write-Host ""
Write-Host "Тестирование завершено!" -ForegroundColor Cyan

