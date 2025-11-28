# Тестовый скрипт для проверки HTTP checker
# Использование: .\test_http_check.ps1

Write-Host "🧪 Тестирование HTTP Checker" -ForegroundColor Cyan
Write-Host ""

# Проверяем наличие утилиты
if (-not (Test-Path ".\http_checker.exe")) {
    Write-Host "❌ http_checker.exe не найден. Запустите компиляцию:" -ForegroundColor Red
    Write-Host "   go build -o http_checker.exe ./cmd/http_checker/main.go" -ForegroundColor Yellow
    exit 1
}

# Тест 1: Проверка одного URL
Write-Host "📋 Тест 1: Проверка одного URL" -ForegroundColor Yellow
.\http_checker.exe http://localhost:9999/health -output reports\test1.json -log logs\test1.log
Write-Host ""

# Тест 2: Проверка с конфигурацией
Write-Host "📋 Тест 2: Проверка с конфигурацией" -ForegroundColor Yellow
if (Test-Path "http_check_config.json") {
    .\http_checker.exe -config http_check_config.json -output reports\test2.json
} else {
    Write-Host "⚠️  Конфигурационный файл не найден, пропускаем тест" -ForegroundColor Yellow
}
Write-Host ""

# Тест 3: Проверка через PowerShell скрипт
Write-Host "📋 Тест 3: Проверка через PowerShell скрипт" -ForegroundColor Yellow
if (Test-Path "http_check.ps1") {
    .\http_check.ps1 -Config http_check_config.json -Output reports\test3.json
} else {
    Write-Host "⚠️  http_check.ps1 не найден" -ForegroundColor Yellow
}
Write-Host ""

Write-Host "✅ Тестирование завершено" -ForegroundColor Green
Write-Host "📊 Проверьте отчеты в директории reports\" -ForegroundColor Cyan

