# Тестовый скрипт для проверки API нормализованной БД
# Использование: .\test_normalized.ps1

$baseUrl = "http://localhost:9999"
$uploadUuid = ""

Write-Host "========================================" -ForegroundColor Cyan
Write-Host "Тестирование API нормализованной БД" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan
Write-Host ""

# Проверка доступности сервера
Write-Host "Проверка доступности сервера..." -ForegroundColor Yellow
try {
    $health = Invoke-RestMethod -Uri "$baseUrl/health" -Method GET
    Write-Host "Сервер доступен: $($health.status)" -ForegroundColor Green
} catch {
    Write-Host "ОШИБКА: Сервер недоступен! Убедитесь, что сервер запущен на порту 9999" -ForegroundColor Red
    exit 1
}
Write-Host ""

# 1. Handshake
Write-Host "1. Handshake (начало выгрузки)..." -ForegroundColor Yellow
$handshakeXml = @"
<?xml version="1.0" encoding="UTF-8"?>
<handshake>
  <version_1c>8.3.25</version_1c>
  <config_name>ТестоваяКонфигурация</config_name>
  <timestamp>2024-01-15T10:30:00Z</timestamp>
</handshake>
"@

try {
    $response = Invoke-RestMethod -Uri "$baseUrl/api/normalized/upload/handshake" `
        -Method POST `
        -ContentType "application/xml; charset=utf-8" `
        -Body $handshakeXml
    
    # Парсим XML ответ
    [xml]$xmlResponse = $response
    $uploadUuid = $xmlResponse.handshake_response.upload_uuid
    Write-Host "   Успешно! Upload UUID: $uploadUuid" -ForegroundColor Green
} catch {
    Write-Host "   ОШИБКА: $($_.Exception.Message)" -ForegroundColor Red
    exit 1
}
Write-Host ""

# 2. Metadata
Write-Host "2. Metadata (метаданные)..." -ForegroundColor Yellow
$metadataXml = @"
<?xml version="1.0" encoding="UTF-8"?>
<metadata>
  <upload_uuid>$uploadUuid</upload_uuid>
  <version_1c>8.3.25</version_1c>
  <config_name>ТестоваяКонфигурация</config_name>
  <timestamp>2024-01-15T10:30:00Z</timestamp>
</metadata>
"@

try {
    Invoke-RestMethod -Uri "$baseUrl/api/normalized/upload/metadata" `
        -Method POST `
        -ContentType "application/xml; charset=utf-8" `
        -Body $metadataXml | Out-Null
    Write-Host "   Успешно!" -ForegroundColor Green
} catch {
    Write-Host "   ОШИБКА: $($_.Exception.Message)" -ForegroundColor Red
}
Write-Host ""

# 3. Constant
Write-Host "3. Constant (константа)..." -ForegroundColor Yellow
$constantXml = @"
<?xml version="1.0" encoding="UTF-8"?>
<constant>
  <upload_uuid>$uploadUuid</upload_uuid>
  <name>ТестоваяКонстанта</name>
  <synonym>ТестоваяКонстанта</synonym>
  <type>Строка</type>
  <value>ТестовоеЗначение</value>
  <timestamp>2024-01-15T10:30:15Z</timestamp>
</constant>
"@

try {
    Invoke-RestMethod -Uri "$baseUrl/api/normalized/upload/constant" `
        -Method POST `
        -ContentType "application/xml; charset=utf-8" `
        -Body $constantXml | Out-Null
    Write-Host "   Успешно!" -ForegroundColor Green
} catch {
    Write-Host "   ОШИБКА: $($_.Exception.Message)" -ForegroundColor Red
}
Write-Host ""

# 4. Catalog Meta
Write-Host "4. Catalog Meta (метаданные справочника)..." -ForegroundColor Yellow
$catalogMetaXml = @"
<?xml version="1.0" encoding="UTF-8"?>
<catalog_meta>
  <upload_uuid>$uploadUuid</upload_uuid>
  <name>ТестовыйСправочник</name>
  <synonym>ТестовыйСправочник</synonym>
  <timestamp>2024-01-15T10:31:00Z</timestamp>
</catalog_meta>
"@

try {
    Invoke-RestMethod -Uri "$baseUrl/api/normalized/upload/catalog/meta" `
        -Method POST `
        -ContentType "application/xml; charset=utf-8" `
        -Body $catalogMetaXml | Out-Null
    Write-Host "   Успешно!" -ForegroundColor Green
} catch {
    Write-Host "   ОШИБКА: $($_.Exception.Message)" -ForegroundColor Red
}
Write-Host ""

# 5. Catalog Item
Write-Host "5. Catalog Item (элемент справочника)..." -ForegroundColor Yellow
$catalogItemXml = @"
<?xml version="1.0" encoding="UTF-8"?>
<catalog_item>
  <upload_uuid>$uploadUuid</upload_uuid>
  <catalog_name>ТестовыйСправочник</catalog_name>
  <reference>Справочник.ТестовыйСправочник.00001</reference>
  <code>00001</code>
  <name>ТестовыйЭлемент</name>
  <attributes><ТестовоеПоле>ТестовоеЗначение</ТестовоеПоле></attributes>
  <table_parts></table_parts>
  <timestamp>2024-01-15T10:31:00Z</timestamp>
</catalog_item>
"@

try {
    Invoke-RestMethod -Uri "$baseUrl/api/normalized/upload/catalog/item" `
        -Method POST `
        -ContentType "application/xml; charset=utf-8" `
        -Body $catalogItemXml | Out-Null
    Write-Host "   Успешно!" -ForegroundColor Green
} catch {
    Write-Host "   ОШИБКА: $($_.Exception.Message)" -ForegroundColor Red
}
Write-Host ""

# 6. Complete
Write-Host "6. Complete (завершение выгрузки)..." -ForegroundColor Yellow
$completeXml = @"
<?xml version="1.0" encoding="UTF-8"?>
<complete>
  <upload_uuid>$uploadUuid</upload_uuid>
  <timestamp>2024-01-15T10:35:00Z</timestamp>
</complete>
"@

try {
    Invoke-RestMethod -Uri "$baseUrl/api/normalized/upload/complete" `
        -Method POST `
        -ContentType "application/xml; charset=utf-8" `
        -Body $completeXml | Out-Null
    Write-Host "   Успешно!" -ForegroundColor Green
} catch {
    Write-Host "   ОШИБКА: $($_.Exception.Message)" -ForegroundColor Red
}
Write-Host ""

Write-Host "========================================" -ForegroundColor Cyan
Write-Host "Проверка данных через API" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan
Write-Host ""

# 7. Проверка списка выгрузок
Write-Host "7. Список выгрузок из нормализованной БД..." -ForegroundColor Yellow
try {
    $uploads = Invoke-RestMethod -Uri "$baseUrl/api/normalized/uploads"
    Write-Host "   Найдено выгрузок: $($uploads.total)" -ForegroundColor Green
    if ($uploads.total -gt 0) {
        Write-Host "   Первая выгрузка: $($uploads.uploads[0].upload_uuid)" -ForegroundColor Green
    }
} catch {
    Write-Host "   ОШИБКА: $($_.Exception.Message)" -ForegroundColor Red
}
Write-Host ""

# 8. Получение деталей выгрузки
Write-Host "8. Детали выгрузки..." -ForegroundColor Yellow
try {
    $details = Invoke-RestMethod -Uri "$baseUrl/api/normalized/uploads/$uploadUuid"
    Write-Host "   Констант: $($details.total_constants)" -ForegroundColor Green
    Write-Host "   Справочников: $($details.total_catalogs)" -ForegroundColor Green
    Write-Host "   Элементов: $($details.total_items)" -ForegroundColor Green
} catch {
    Write-Host "   ОШИБКА: $($_.Exception.Message)" -ForegroundColor Red
}
Write-Host ""

# 9. Получение данных
Write-Host "9. Получение данных (XML)..." -ForegroundColor Yellow
try {
    $dataResponse = Invoke-WebRequest -Uri "$baseUrl/api/normalized/uploads/$uploadUuid/data?type=all" -Method GET
    [xml]$dataXml = $dataResponse.Content
    Write-Host "   Всего элементов: $($dataXml.data_response.total)" -ForegroundColor Green
    Write-Host "   Констант: $($dataXml.data_response.items.item | Where-Object { $_.type -eq 'constant' } | Measure-Object).Count" -ForegroundColor Green
    Write-Host "   Элементов справочников: $($dataXml.data_response.items.item | Where-Object { $_.type -eq 'catalog_item' } | Measure-Object).Count" -ForegroundColor Green
} catch {
    Write-Host "   ОШИБКА: $($_.Exception.Message)" -ForegroundColor Red
}
Write-Host ""

Write-Host "========================================" -ForegroundColor Cyan
Write-Host "Тестирование завершено!" -ForegroundColor Cyan
Write-Host "Upload UUID: $uploadUuid" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan
Write-Host ""
Write-Host "Для проверки данных используйте:" -ForegroundColor Yellow
Write-Host "  curl http://localhost:9999/api/normalized/uploads/$uploadUuid/data?type=all" -ForegroundColor White
Write-Host ""

