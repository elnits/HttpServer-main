# PowerShell скрипт для запуска обоих серверов
Write-Host "========================================" -ForegroundColor Cyan
Write-Host "Запуск системы нормализации данных" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan
Write-Host ""

# Проверяем наличие Go
$goExists = Get-Command go -ErrorAction SilentlyContinue
if (-not $goExists) {
    Write-Host "[ОШИБКА] Go не найден в PATH" -ForegroundColor Red
    Write-Host "Установите Go и добавьте его в PATH"
    Read-Host "Нажмите Enter для выхода"
    exit 1
}

# Проверяем наличие Node.js
$npmExists = Get-Command npm -ErrorAction SilentlyContinue
if (-not $npmExists) {
    Write-Host "[ОШИБКА] Node.js не найден в PATH" -ForegroundColor Red
    Write-Host "Установите Node.js и добавьте его в PATH"
    Read-Host "Нажмите Enter для выхода"
    exit 1
}

# Переходим в директорию проекта
Set-Location $PSScriptRoot

Write-Host "[1/2] Запуск бэкенда на порту 9999..." -ForegroundColor Yellow
Start-Process powershell -ArgumentList "-NoExit", "-Command", "cd '$PSScriptRoot'; go run -tags no_gui main_no_gui.go" -WindowStyle Normal

Start-Sleep -Seconds 3

Write-Host "[2/2] Запуск фронтенда на порту 3000..." -ForegroundColor Yellow
Start-Process powershell -ArgumentList "-NoExit", "-Command", "cd '$PSScriptRoot\frontend'; npm run dev" -WindowStyle Normal

Write-Host ""
Write-Host "========================================" -ForegroundColor Green
Write-Host "Серверы запущены!" -ForegroundColor Green
Write-Host ""
Write-Host "Бэкенд:  http://localhost:9999" -ForegroundColor Cyan
Write-Host "Фронтенд: http://localhost:3000" -ForegroundColor Cyan
Write-Host ""
Write-Host "Для остановки закройте окна серверов" -ForegroundColor Yellow
Write-Host "========================================" -ForegroundColor Green
Write-Host ""

Read-Host "Нажмите Enter для выхода"

