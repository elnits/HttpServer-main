# PowerShell скрипт для проверки статуса серверов

Write-Host "Проверка статуса серверов..." -ForegroundColor Cyan
Write-Host ""

# Проверка бэкенда
Write-Host "Бэкенд (http://localhost:9999): " -NoNewline
try {
    $response = Invoke-WebRequest -Uri "http://localhost:9999/health" -TimeoutSec 2 -ErrorAction Stop
    Write-Host "✓ Работает" -ForegroundColor Green
    Write-Host "  Ответ: $($response.Content)" -ForegroundColor Gray
} catch {
    Write-Host "✗ Недоступен" -ForegroundColor Red
    Write-Host "  Ошибка: $($_.Exception.Message)" -ForegroundColor Gray
}

Write-Host ""

# Проверка фронтенда
Write-Host "Фронтенд (http://localhost:3000): " -NoNewline
try {
    $response = Invoke-WebRequest -Uri "http://localhost:3000" -TimeoutSec 2 -ErrorAction Stop
    if ($response.Content -match "Нормализатор") {
        Write-Host "✓ Работает" -ForegroundColor Green
    } else {
        Write-Host "? Запущен, но контент не найден" -ForegroundColor Yellow
    }
} catch {
    Write-Host "✗ Недоступен" -ForegroundColor Red
    Write-Host "  Ошибка: $($_.Exception.Message)" -ForegroundColor Gray
}

Write-Host ""

