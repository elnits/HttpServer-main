# Скрипт для пересборки Docker контейнеров с новыми изменениями

Write-Host "=== Пересборка Docker контейнеров ===" -ForegroundColor Cyan

# Проверяем, запущен ли Docker
Write-Host "`nПроверка Docker..." -ForegroundColor Yellow
try {
    docker ps | Out-Null
    Write-Host "✓ Docker доступен" -ForegroundColor Green
} catch {
    Write-Host "✗ Docker не запущен или недоступен!" -ForegroundColor Red
    Write-Host "Пожалуйста, запустите Docker Desktop и повторите попытку." -ForegroundColor Yellow
    exit 1
}

# Останавливаем текущие контейнеры
Write-Host "`nОстановка текущих контейнеров..." -ForegroundColor Yellow
docker-compose down
if ($LASTEXITCODE -eq 0) {
    Write-Host "✓ Контейнеры остановлены" -ForegroundColor Green
} else {
    Write-Host "⚠ Некоторые контейнеры могли быть уже остановлены" -ForegroundColor Yellow
}

# Пересобираем backend контейнер
Write-Host "`nПересборка backend контейнера (это может занять несколько минут)..." -ForegroundColor Yellow
$OutputEncoding = [System.Text.Encoding]::UTF8
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
docker-compose build --no-cache backend 2>&1 | Out-String | Write-Host
if ($LASTEXITCODE -eq 0) {
    Write-Host "✓ Backend контейнер пересобран" -ForegroundColor Green
} else {
    Write-Host "✗ Ошибка при пересборке backend контейнера" -ForegroundColor Red
    exit 1
}

# Пересобираем frontend контейнер
Write-Host "`nПересборка frontend контейнера..." -ForegroundColor Yellow
docker-compose build --no-cache frontend 2>&1 | Out-String | Write-Host
if ($LASTEXITCODE -eq 0) {
    Write-Host "✓ Frontend контейнер пересобран" -ForegroundColor Green
} else {
    Write-Host "✗ Ошибка при пересборке frontend контейнера" -ForegroundColor Red
    exit 1
}

# Запускаем контейнеры
Write-Host "`nЗапуск контейнеров..." -ForegroundColor Yellow
docker-compose up -d
if ($LASTEXITCODE -eq 0) {
    Write-Host "✓ Контейнеры запущены" -ForegroundColor Green
} else {
    Write-Host "✗ Ошибка при запуске контейнеров" -ForegroundColor Red
    exit 1
}

# Проверяем статус
Write-Host "`nПроверка статуса контейнеров..." -ForegroundColor Yellow
Start-Sleep -Seconds 3
docker-compose ps

# Показываем логи backend
Write-Host "`nПоследние логи backend (Ctrl+C для выхода):" -ForegroundColor Yellow
Write-Host "Для просмотра всех логов используйте: docker-compose logs -f backend" -ForegroundColor Cyan
docker-compose logs --tail=20 backend

Write-Host "`n=== Пересборка завершена ===" -ForegroundColor Green
Write-Host "Backend доступен на: http://localhost:9999" -ForegroundColor Cyan
Write-Host "Frontend доступен на: http://localhost:3000" -ForegroundColor Cyan
Write-Host "`nПроверьте работу системы:" -ForegroundColor Yellow
Write-Host "  - Health check: curl http://localhost:9999/health" -ForegroundColor White
Write-Host "  - API stats: curl http://localhost:9999/api/normalization/stats" -ForegroundColor White
