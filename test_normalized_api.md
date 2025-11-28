# Тестирование API для нормализованной БД

## Подготовка

1. Запустите сервер:
   ```bash
   .\httpserver.exe
   ```
   или
   ```bash
   .\start_server.bat
   ```

2. Убедитесь, что сервер запущен на порту 9999

## Тестирование получения данных из нормализованной БД

### 1. Получить список выгрузок из нормализованной БД

```bash
curl http://localhost:9999/api/normalized/uploads
```

**Ожидаемый результат:** JSON с массивом выгрузок (изначально пустой, если БД новая)

### 2. Получить детали конкретной выгрузки

```bash
curl http://localhost:9999/api/normalized/uploads/{uuid}
```

**Пример:**
```bash
curl http://localhost:9999/api/normalized/uploads/550e8400-e29b-41d4-a716-446655440000
```

## Тестирование записи нормализованных данных

### Шаг 1: Handshake (начало выгрузки)

```bash
curl -X POST http://localhost:9999/api/normalized/upload/handshake \
  -H "Content-Type: application/xml" \
  -d "<?xml version=\"1.0\" encoding=\"UTF-8\"?><handshake><version_1c>8.3.25</version_1c><config_name>УправлениеТорговлей</config_name><timestamp>2024-01-15T10:30:00Z</timestamp></handshake>"
```

**Ожидаемый результат:** XML с `upload_uuid` - сохраните этот UUID для следующих шагов

**Пример ответа:**
```xml
<?xml version="1.0" encoding="UTF-8"?>
<handshake_response>
  <success>true</success>
  <upload_uuid>550e8400-e29b-41d4-a716-446655440000</upload_uuid>
  <message>Normalized handshake successful</message>
  <timestamp>2024-01-15T10:30:00Z</timestamp>
</handshake_response>
```

### Шаг 2: Metadata (метаданные)

```bash
curl -X POST http://localhost:9999/api/normalized/upload/metadata \
  -H "Content-Type: application/xml" \
  -d "<?xml version=\"1.0\" encoding=\"UTF-8\"?><metadata><upload_uuid>550e8400-e29b-41d4-a716-446655440000</upload_uuid><version_1c>8.3.25</version_1c><config_name>УправлениеТорговлей</config_name><timestamp>2024-01-15T10:30:00Z</timestamp></metadata>"
```

### Шаг 3: Constant (константа)

```bash
curl -X POST http://localhost:9999/api/normalized/upload/constant \
  -H "Content-Type: application/xml" \
  -d "<?xml version=\"1.0\" encoding=\"UTF-8\"?><constant><upload_uuid>550e8400-e29b-41d4-a716-446655440000</upload_uuid><name>Организация</name><synonym>Организация</synonym><type>Строка</type><value>ООО Рога и Копыта</value><timestamp>2024-01-15T10:30:15Z</timestamp></constant>"
```

### Шаг 4: Catalog Meta (метаданные справочника)

```bash
curl -X POST http://localhost:9999/api/normalized/upload/catalog/meta \
  -H "Content-Type: application/xml" \
  -d "<?xml version=\"1.0\" encoding=\"UTF-8\"?><catalog_meta><upload_uuid>550e8400-e29b-41d4-a716-446655440000</upload_uuid><name>Номенклатура</name><synonym>Номенклатура</synonym><timestamp>2024-01-15T10:31:00Z</timestamp></catalog_meta>"
```

### Шаг 5: Catalog Item (элемент справочника)

```bash
curl -X POST http://localhost:9999/api/normalized/upload/catalog/item \
  -H "Content-Type: application/xml" \
  -d "<?xml version=\"1.0\" encoding=\"UTF-8\"?><catalog_item><upload_uuid>550e8400-e29b-41d4-a716-446655440000</upload_uuid><catalog_name>Номенклатура</catalog_name><reference>Справочник.Номенклатура.12345</reference><code>00001</code><name>Товар 1</name><attributes><ЕдиницаИзмерения>шт</ЕдиницаИзмерения><Вес>10.5</Вес></attributes><table_parts></table_parts><timestamp>2024-01-15T10:31:00Z</timestamp></catalog_item>"
```

### Шаг 6: Complete (завершение выгрузки)

```bash
curl -X POST http://localhost:9999/api/normalized/upload/complete \
  -H "Content-Type: application/xml" \
  -d "<?xml version=\"1.0\" encoding=\"UTF-8\"?><complete><upload_uuid>550e8400-e29b-41d4-a716-446655440000</upload_uuid><timestamp>2024-01-15T10:35:00Z</timestamp></complete>"
```

## Полный цикл тестирования

### Сценарий 1: Полный цикл записи и чтения нормализованных данных

1. **Записать нормализованные данные** (выполнить шаги 1-6 выше)

2. **Проверить список выгрузок:**
   ```bash
   curl http://localhost:9999/api/normalized/uploads
   ```

3. **Получить детали выгрузки:**
   ```bash
   curl http://localhost:9999/api/normalized/uploads/550e8400-e29b-41d4-a716-446655440000
   ```

4. **Получить данные выгрузки:**
   ```bash
   curl "http://localhost:9999/api/normalized/uploads/550e8400-e29b-41d4-a716-446655440000/data?type=all"
   ```

5. **Получить только константы:**
   ```bash
   curl "http://localhost:9999/api/normalized/uploads/550e8400-e29b-41d4-a716-446655440000/data?type=constants"
   ```

6. **Получить только элементы справочников:**
   ```bash
   curl "http://localhost:9999/api/normalized/uploads/550e8400-e29b-41d4-a716-446655440000/data?type=catalogs"
   ```

7. **Потоковая отправка:**
   ```bash
   curl "http://localhost:9999/api/normalized/uploads/550e8400-e29b-41d4-a716-446655440000/stream?type=all"
   ```

### Сценарий 2: Тестирование с реальными данными из основной БД

1. **Получить данные из основной БД:**
   ```bash
   # Сначала получите UUID выгрузки из основной БД
   curl http://localhost:9999/api/uploads
   
   # Затем получите данные (замените UUID на реальный)
   curl "http://localhost:9999/api/uploads/22bc596d-0933-4036-8664-51cdf26a4c34/data?type=all&limit=10" > original_data.xml
   ```

2. **Нормализовать данные** (внешний ресурс)

3. **Записать нормализованные данные** (шаги 1-6 выше)

4. **Сравнить данные:**
   ```bash
   # Получить нормализованные данные
   curl "http://localhost:9999/api/normalized/uploads/{normalized_uuid}/data?type=all" > normalized_data.xml
   
   # Сравнить структуру (должна быть идентичной)
   ```

## Проверка через браузер

Для JSON эндпоинтов можно использовать браузер:

- `http://localhost:9999/api/normalized/uploads`
- `http://localhost:9999/api/normalized/uploads/{uuid}`

## Проверка базы данных напрямую

Если у вас установлен SQLite:

```bash
sqlite3 normalized_data.db "SELECT * FROM uploads;"
sqlite3 normalized_data.db "SELECT COUNT(*) FROM constants;"
sqlite3 normalized_data.db "SELECT COUNT(*) FROM catalogs;"
sqlite3 normalized_data.db "SELECT COUNT(*) FROM catalog_items;"
```

## Пример тестового скрипта (PowerShell)

Создайте файл `test_normalized.ps1`:

```powershell
$baseUrl = "http://localhost:9999"
$uploadUuid = ""

# 1. Handshake
Write-Host "1. Handshake..."
$handshakeXml = @"
<?xml version="1.0" encoding="UTF-8"?>
<handshake>
  <version_1c>8.3.25</version_1c>
  <config_name>ТестоваяКонфигурация</config_name>
  <timestamp>2024-01-15T10:30:00Z</timestamp>
</handshake>
"@

$response = Invoke-RestMethod -Uri "$baseUrl/api/normalized/upload/handshake" `
  -Method POST `
  -ContentType "application/xml" `
  -Body $handshakeXml

$uploadUuid = $response.handshake_response.upload_uuid
Write-Host "Upload UUID: $uploadUuid"

# 2. Metadata
Write-Host "2. Metadata..."
$metadataXml = @"
<?xml version="1.0" encoding="UTF-8"?>
<metadata>
  <upload_uuid>$uploadUuid</upload_uuid>
  <version_1c>8.3.25</version_1c>
  <config_name>ТестоваяКонфигурация</config_name>
  <timestamp>2024-01-15T10:30:00Z</timestamp>
</metadata>
"@

Invoke-RestMethod -Uri "$baseUrl/api/normalized/upload/metadata" `
  -Method POST `
  -ContentType "application/xml" `
  -Body $metadataXml

# 3. Constant
Write-Host "3. Constant..."
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

Invoke-RestMethod -Uri "$baseUrl/api/normalized/upload/constant" `
  -Method POST `
  -ContentType "application/xml" `
  -Body $constantXml

# 4. Catalog Meta
Write-Host "4. Catalog Meta..."
$catalogMetaXml = @"
<?xml version="1.0" encoding="UTF-8"?>
<catalog_meta>
  <upload_uuid>$uploadUuid</upload_uuid>
  <name>ТестовыйСправочник</name>
  <synonym>ТестовыйСправочник</synonym>
  <timestamp>2024-01-15T10:31:00Z</timestamp>
</catalog_meta>
"@

Invoke-RestMethod -Uri "$baseUrl/api/normalized/upload/catalog/meta" `
  -Method POST `
  -ContentType "application/xml" `
  -Body $catalogMetaXml

# 5. Catalog Item
Write-Host "5. Catalog Item..."
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

Invoke-RestMethod -Uri "$baseUrl/api/normalized/upload/catalog/item" `
  -Method POST `
  -ContentType "application/xml" `
  -Body $catalogItemXml

# 6. Complete
Write-Host "6. Complete..."
$completeXml = @"
<?xml version="1.0" encoding="UTF-8"?>
<complete>
  <upload_uuid>$uploadUuid</upload_uuid>
  <timestamp>2024-01-15T10:35:00Z</timestamp>
</complete>
"@

Invoke-RestMethod -Uri "$baseUrl/api/normalized/upload/complete" `
  -Method POST `
  -ContentType "application/xml" `
  -Body $completeXml

Write-Host "`nВыгрузка завершена! UUID: $uploadUuid"

# 7. Проверка данных
Write-Host "`n7. Проверка списка выгрузок..."
$uploads = Invoke-RestMethod -Uri "$baseUrl/api/normalized/uploads"
Write-Host "Найдено выгрузок: $($uploads.total)"

Write-Host "`n8. Получение данных..."
$data = Invoke-RestMethod -Uri "$baseUrl/api/normalized/uploads/$uploadUuid/data?type=all"
Write-Host "Всего элементов: $($data.data_response.total)"
```

Запуск:
```powershell
.\test_normalized.ps1
```

## Ожидаемые результаты

После выполнения всех шагов:

1. В `normalized_data.db` должна появиться новая выгрузка
2. В таблице `uploads` должна быть запись с созданным UUID
3. В таблице `constants` должна быть запись константы
4. В таблице `catalogs` должна быть запись справочника
5. В таблице `catalog_items` должна быть запись элемента справочника
6. API должен возвращать данные в том же формате, что и основная БД

## Отладка

Если что-то не работает:

1. Проверьте логи сервера
2. Убедитесь, что сервер запущен
3. Проверьте, что файл `normalized_data.db` создан
4. Проверьте формат XML запросов
5. Используйте `curl -v` для детальной информации о запросах

