# Скрипт для пересборки Docker контейнеров с проверками

Write-Host "=== Пересборка Docker контейнеров ===" -ForegroundColor Cyan
Write-Host ""

# Проверка Docker
Write-Host "🔍 Проверка Docker..." -ForegroundColor Yellow
try {
    $dockerVersion = docker --version 2>&1
    if ($LASTEXITCODE -ne 0) {
        throw "Docker не найден"
    }
    Write-Host "✅ $dockerVersion" -ForegroundColor Green
} catch {
    Write-Host "❌ Docker не установлен или не в PATH" -ForegroundColor Red
    exit 1
}

# Проверка Docker Desktop
Write-Host "`n🔍 Проверка Docker Desktop..." -ForegroundColor Yellow
try {
    docker ps > $null 2>&1
    if ($LASTEXITCODE -ne 0) {
        Write-Host "❌ Docker Desktop не запущен!" -ForegroundColor Red
        Write-Host "`nПожалуйста:" -ForegroundColor Yellow
        Write-Host "1. Запустите Docker Desktop" -ForegroundColor White
        Write-Host "2. Дождитесь полного запуска (иконка в трее станет зеленой)" -ForegroundColor White
        Write-Host "3. Запустите этот скрипт снова" -ForegroundColor White
        Write-Host "`nИли перезапустите локальный сервер:" -ForegroundColor Cyan
        Write-Host "   go run ." -ForegroundColor Gray
        exit 1
    }
    Write-Host "✅ Docker Desktop запущен" -ForegroundColor Green
} catch {
    Write-Host "❌ Не удалось подключиться к Docker" -ForegroundColor Red
    exit 1
}

# Остановка контейнеров
Write-Host "`n🛑 Остановка контейнеров..." -ForegroundColor Yellow
docker-compose down
if ($LASTEXITCODE -ne 0) {
    Write-Host "⚠️  Предупреждение при остановке (возможно, контейнеры уже остановлены)" -ForegroundColor Yellow
}

# Пересборка backend
Write-Host "`n🔨 Пересборка backend (без кэша)..." -ForegroundColor Yellow
$OutputEncoding = [System.Text.Encoding]::UTF8
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
docker-compose build --no-cache backend 2>&1 | Out-String | Write-Host
if ($LASTEXITCODE -ne 0) {
    Write-Host "❌ Ошибка при сборке backend!" -ForegroundColor Red
    exit 1
}
Write-Host "✅ Backend собран" -ForegroundColor Green

# Пересборка frontend
Write-Host "`n🔨 Пересборка frontend (без кэша)..." -ForegroundColor Yellow
docker-compose build --no-cache frontend 2>&1 | Out-String | Write-Host
if ($LASTEXITCODE -ne 0) {
    Write-Host "❌ Ошибка при сборке frontend!" -ForegroundColor Red
    exit 1
}
Write-Host "✅ Frontend собран" -ForegroundColor Green

# Запуск контейнеров
Write-Host "`n🚀 Запуск контейнеров..." -ForegroundColor Yellow
docker-compose up -d
if ($LASTEXITCODE -ne 0) {
    Write-Host "❌ Ошибка при запуске контейнеров!" -ForegroundColor Red
    exit 1
}
Write-Host "✅ Контейнеры запущены" -ForegroundColor Green

# Ожидание запуска
Write-Host "`n⏳ Ожидание запуска сервисов (10 секунд)..." -ForegroundColor Yellow
Start-Sleep -Seconds 10

# Проверка статуса
Write-Host "`n📊 Статус контейнеров:" -ForegroundColor Cyan
docker-compose ps

# Проверка health check
Write-Host "`n🏥 Проверка health check..." -ForegroundColor Cyan
try {
    $health = Invoke-WebRequest -Uri "http://localhost:9999/health" -UseBasicParsing -TimeoutSec 5 -ErrorAction Stop
    if ($health.StatusCode -eq 200) {
        Write-Host "✅ Backend работает!" -ForegroundColor Green
        Write-Host "   Ответ: $($health.Content)" -ForegroundColor Gray
    }
} catch {
    Write-Host "⚠️  Backend еще запускается..." -ForegroundColor Yellow
    Write-Host "   Проверьте логи: docker-compose logs -f backend" -ForegroundColor Gray
}

# Проверка нового эндпоинта
Write-Host "`n🔧 Проверка эндпоинта /api/workers/config..." -ForegroundColor Cyan
try {
    $workers = Invoke-WebRequest -Uri "http://localhost:9999/api/workers/config" -UseBasicParsing -TimeoutSec 5 -ErrorAction Stop
    if ($workers.StatusCode -eq 200) {
        Write-Host "✅ Эндпоинт workers работает!" -ForegroundColor Green
        $config = $workers.Content | ConvertFrom-Json
        Write-Host "   Провайдеров: $($config.providers.PSObject.Properties.Count)" -ForegroundColor Gray
        Write-Host "   Дефолтный провайдер: $($config.default_provider)" -ForegroundColor Gray
    }
} catch {
    if ($_.Exception.Response.StatusCode -eq 404) {
        Write-Host "❌ Эндпоинт не найден (404)" -ForegroundColor Red
        Write-Host "   Возможно, сервер еще запускается или нужен перезапуск" -ForegroundColor Yellow
    } else {
        Write-Host "⚠️  Эндпоинт недоступен: $($_.Exception.Message)" -ForegroundColor Yellow
    }
}

Write-Host "`n✅ Пересборка завершена!" -ForegroundColor Green
Write-Host "`n📝 Полезные команды:" -ForegroundColor Cyan
Write-Host "   Просмотр логов: docker-compose logs -f" -ForegroundColor Gray
Write-Host "   Логи backend: docker-compose logs -f backend" -ForegroundColor Gray
Write-Host "   Логи frontend: docker-compose logs -f frontend" -ForegroundColor Gray
Write-Host "   Остановка: docker-compose down" -ForegroundColor Gray
Write-Host "   Перезапуск: docker-compose restart" -ForegroundColor Gray
Write-Host "`n🌐 Ссылки:" -ForegroundColor Cyan
Write-Host "   Frontend: http://localhost:3000" -ForegroundColor White
Write-Host "   Backend: http://localhost:9999" -ForegroundColor White
Write-Host "   Workers Config: http://localhost:9999/api/workers/config" -ForegroundColor White
Write-Host "`n🆕 Новые возможности в этой версии:" -ForegroundColor Cyan
Write-Host "   ✅ Система извлечения атрибутов товаров" -ForegroundColor Green
Write-Host "   ✅ API для получения атрибутов: /api/normalization/item-attributes/{id}" -ForegroundColor Green
Write-Host "   ✅ Отображение атрибутов на фронтенде" -ForegroundColor Green
Write-Host "   ✅ Автоматическое извлечение размеров, материалов, цветов и т.д." -ForegroundColor Green

