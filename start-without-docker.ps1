# Скрипт для запуска приложения без Docker
Write-Host "Запуск приложения без Docker..." -ForegroundColor Cyan

# Проверка Go
Write-Host "`nПроверка Go..." -ForegroundColor Yellow
$goVersion = go version 2>&1
if ($LASTEXITCODE -ne 0) {
    Write-Host "ОШИБКА: Go не установлен!" -ForegroundColor Red
    exit 1
}
Write-Host $goVersion -ForegroundColor Green

# Проверка Node.js
Write-Host "`nПроверка Node.js..." -ForegroundColor Yellow
$nodeVersion = node --version 2>&1
if ($LASTEXITCODE -ne 0) {
    Write-Host "ОШИБКА: Node.js не установлен!" -ForegroundColor Red
    exit 1
}
Write-Host $nodeVersion -ForegroundColor Green

# Сборка backend
Write-Host "`nСборка backend..." -ForegroundColor Yellow
go build -tags no_gui -o httpserver.exe .
if ($LASTEXITCODE -ne 0) {
    Write-Host "ОШИБКА: Не удалось собрать backend!" -ForegroundColor Red
    exit 1
}
Write-Host "Backend собран успешно" -ForegroundColor Green

# Запуск backend в фоне
Write-Host "`nЗапуск backend на порту 9999..." -ForegroundColor Yellow
Start-Process -FilePath ".\httpserver.exe" -WindowStyle Minimized
Start-Sleep -Seconds 3

# Проверка backend
$backendCheck = Invoke-WebRequest -Uri "http://localhost:9999/health" -UseBasicParsing -ErrorAction SilentlyContinue
if ($backendCheck.StatusCode -eq 200) {
    Write-Host "Backend запущен успешно" -ForegroundColor Green
} else {
    Write-Host "Предупреждение: Backend может быть еще не готов" -ForegroundColor Yellow
}

# Запуск frontend
Write-Host "`nЗапуск frontend..." -ForegroundColor Yellow
if (Test-Path "app") {
    $frontendPath = "app"
} elseif (Test-Path "frontend") {
    $frontendPath = "frontend"
} else {
    Write-Host "ОШИБКА: Директория frontend не найдена!" -ForegroundColor Red
    exit 1
}

Write-Host "Frontend директория: $frontendPath" -ForegroundColor Cyan
Write-Host "`nДля запуска frontend выполните вручную:" -ForegroundColor Yellow
Write-Host "  cd $frontendPath" -ForegroundColor White
Write-Host "  npm run dev" -ForegroundColor White

Write-Host "`n✅ Backend запущен на http://localhost:9999" -ForegroundColor Green
Write-Host "📝 Frontend нужно запустить вручную в директории $frontendPath" -ForegroundColor Yellow

