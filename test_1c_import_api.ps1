# Тестовый скрипт для проверки эндпоинтов импорта данных в 1С
# Использование: .\test_1c_import_api.ps1

$ErrorActionPreference = "Stop"

# Настройки
$ServerUrl = "http://localhost:9999"
$Timeout = 7  # секунды

# Цвета для вывода
function Write-Success { Write-Host "✓ $args" -ForegroundColor Green }
function Write-Error { Write-Host "✗ $args" -ForegroundColor Red }
function Write-Info { Write-Host "ℹ $args" -ForegroundColor Cyan }
function Write-Header { 
    Write-Host ""
    Write-Host "========================================" -ForegroundColor Yellow
    Write-Host $args -ForegroundColor Yellow
    Write-Host "========================================" -ForegroundColor Yellow
}

# Функция для отправки HTTP запроса
function Send-HttpRequest {
    param(
        [string]$Method,
        [string]$Url,
        [string]$Body = $null
    )
    
    try {
        $headers = @{
            "Content-Type" = "application/xml; charset=utf-8"
        }
        
        if ($Method -eq "GET") {
            $response = Invoke-WebRequest -Uri $Url -Method $Method -TimeoutSec $Timeout -UseBasicParsing
        } else {
            $response = Invoke-WebRequest -Uri $Url -Method $Method -Body ([System.Text.Encoding]::UTF8.GetBytes($Body)) -Headers $headers -TimeoutSec $Timeout -UseBasicParsing
        }
        
        return @{
            Success = $true
            StatusCode = $response.StatusCode
            Content = $response.Content
        }
    } catch {
        return @{
            Success = $false
            StatusCode = $_.Exception.Response.StatusCode.value__
            Error = $_.Exception.Message
        }
    }
}

# Функция для парсинга XML
function Parse-XmlValue {
    param(
        [string]$XmlContent,
        [string]$TagName
    )
    
    $pattern = "<$TagName>(.*?)</$TagName>"
    if ($XmlContent -match $pattern) {
        return $matches[1]
    }
    return $null
}

# Функция для подсчета элементов в XML
function Count-XmlElements {
    param(
        [string]$XmlContent,
        [string]$TagName
    )
    
    $pattern = "<$TagName>"
    $matches = [regex]::Matches($XmlContent, $pattern)
    return $matches.Count
}

# Главная функция тестирования
function Test-ImportApi {
    
    Write-Header "ТЕСТИРОВАНИЕ API ИМПОРТА ДАННЫХ В 1С"
    
    # Тест 1: Проверка доступности сервера
    Write-Header "Тест 1: Проверка доступности сервера"
    
    Write-Info "Проверяем health endpoint..."
    $result = Send-HttpRequest -Method "GET" -Url "$ServerUrl/health"
    
    if ($result.Success -and $result.StatusCode -eq 200) {
        Write-Success "Сервер доступен (HTTP $($result.StatusCode))"
    } else {
        Write-Error "Сервер недоступен!"
        Write-Error "Ошибка: $($result.Error)"
        return
    }
    
    # Тест 2: Получение списка баз
    Write-Header "Тест 2: GET /api/1c/databases - Получение списка баз"
    
    $result = Send-HttpRequest -Method "GET" -Url "$ServerUrl/api/1c/databases"
    
    if ($result.Success -and $result.StatusCode -eq 200) {
        Write-Success "Список баз получен (HTTP $($result.StatusCode))"
        
        $total = Parse-XmlValue -XmlContent $result.Content -TagName "total"
        Write-Info "Найдено баз: $total"
        
        $databaseCount = Count-XmlElements -XmlContent $result.Content -TagName "database"
        Write-Info "Элементов database: $databaseCount"
        
        if ($databaseCount -gt 0) {
            # Парсим первую базу
            if ($result.Content -match "<file_name>(.*?)</file_name>") {
                $global:TestDatabaseName = $matches[1]
                Write-Info "Первая база: $global:TestDatabaseName"
            }
            
            if ($result.Content -match "<config_name>(.*?)</config_name>") {
                Write-Info "Конфигурация: $($matches[1])"
            }
            
            if ($result.Content -match "<total_catalogs>(.*?)</total_catalogs>") {
                Write-Info "Справочников: $($matches[1])"
            }
        } else {
            Write-Error "Не найдено ни одной базы для тестирования!"
            return
        }
        
    } else {
        Write-Error "Не удалось получить список баз (HTTP $($result.StatusCode))"
        Write-Error "Ошибка: $($result.Error)"
        return
    }
    
    # Проверяем что база для тестов найдена
    if (-not $global:TestDatabaseName) {
        Write-Error "Не найдена база данных для тестирования!"
        return
    }
    
    # Тест 3: Handshake для импорта
    Write-Header "Тест 3: POST /api/1c/import/handshake - Начало импорта"
    
    $handshakeXml = @"
<?xml version="1.0" encoding="UTF-8"?>
<import_handshake>
  <db_name>$global:TestDatabaseName</db_name>
  <client_info>
    <version_1c>8.3.25.1257</version_1c>
    <computer_name>TEST-PC</computer_name>
    <user_name>TestUser</user_name>
  </client_info>
</import_handshake>
"@
    
    $result = Send-HttpRequest -Method "POST" -Url "$ServerUrl/api/1c/import/handshake" -Body $handshakeXml
    
    if ($result.Success -and $result.StatusCode -eq 200) {
        Write-Success "Handshake успешен (HTTP $($result.StatusCode))"
        
        $success = Parse-XmlValue -XmlContent $result.Content -TagName "success"
        $uploadUuid = Parse-XmlValue -XmlContent $result.Content -TagName "upload_uuid"
        $constantsCount = Parse-XmlValue -XmlContent $result.Content -TagName "constants_count"
        
        Write-Info "Success: $success"
        Write-Info "Upload UUID: $uploadUuid"
        Write-Info "Количество констант: $constantsCount"
        
        $catalogCount = Count-XmlElements -XmlContent $result.Content -TagName "catalog"
        Write-Info "Количество справочников: $catalogCount"
        
        # Сохраняем имя первого справочника для следующего теста
        if ($result.Content -match "<name>(.*?)</name>") {
            $global:TestCatalogName = $matches[1]
            Write-Info "Первый справочник: $global:TestCatalogName"
        }
        
    } else {
        Write-Error "Ошибка handshake (HTTP $($result.StatusCode))"
        Write-Error "Ошибка: $($result.Error)"
        return
    }
    
    # Тест 4: Получение констант
    Write-Header "Тест 4: POST /api/1c/import/get-constants - Получение констант"
    
    $getConstantsXml = @"
<?xml version="1.0" encoding="UTF-8"?>
<import_get_constants>
  <db_name>$global:TestDatabaseName</db_name>
  <offset>0</offset>
  <limit>100</limit>
</import_get_constants>
"@
    
    $result = Send-HttpRequest -Method "POST" -Url "$ServerUrl/api/1c/import/get-constants" -Body $getConstantsXml
    
    if ($result.Success -and $result.StatusCode -eq 200) {
        Write-Success "Константы получены (HTTP $($result.StatusCode))"
        
        $success = Parse-XmlValue -XmlContent $result.Content -TagName "success"
        $total = Parse-XmlValue -XmlContent $result.Content -TagName "total"
        
        Write-Info "Success: $success"
        Write-Info "Всего констант: $total"
        
        $constantCount = Count-XmlElements -XmlContent $result.Content -TagName "constant"
        Write-Info "Получено констант: $constantCount"
        
        # Парсим первую константу
        if ($result.Content -match "<constant>[\s\S]*?<name>(.*?)</name>[\s\S]*?<value>(.*?)</value>") {
            Write-Info "Первая константа: $($matches[1]) = $($matches[2])"
        }
        
    } else {
        Write-Error "Ошибка получения констант (HTTP $($result.StatusCode))"
        Write-Error "Ошибка: $($result.Error)"
    }
    
    # Тест 5: Получение справочника
    if ($global:TestCatalogName) {
        Write-Header "Тест 5: POST /api/1c/import/get-catalog - Получение справочника"
        
        $getCatalogXml = @"
<?xml version="1.0" encoding="UTF-8"?>
<import_get_catalog>
  <db_name>$global:TestDatabaseName</db_name>
  <catalog_name>$global:TestCatalogName</catalog_name>
  <offset>0</offset>
  <limit>10</limit>
</import_get_catalog>
"@
        
        $result = Send-HttpRequest -Method "POST" -Url "$ServerUrl/api/1c/import/get-catalog" -Body $getCatalogXml
        
        if ($result.Success -and $result.StatusCode -eq 200) {
            Write-Success "Элементы справочника получены (HTTP $($result.StatusCode))"
            
            $success = Parse-XmlValue -XmlContent $result.Content -TagName "success"
            $catalogName = Parse-XmlValue -XmlContent $result.Content -TagName "catalog_name"
            $total = Parse-XmlValue -XmlContent $result.Content -TagName "total"
            
            Write-Info "Success: $success"
            Write-Info "Справочник: $catalogName"
            Write-Info "Всего элементов: $total"
            
            $itemCount = Count-XmlElements -XmlContent $result.Content -TagName "item"
            Write-Info "Получено элементов: $itemCount"
            
            # Парсим первый элемент
            if ($result.Content -match "<item>[\s\S]*?<code>(.*?)</code>[\s\S]*?<name>(.*?)</name>") {
                Write-Info "Первый элемент: [$($matches[1])] $($matches[2])"
            }
            
        } else {
            Write-Error "Ошибка получения справочника (HTTP $($result.StatusCode))"
            Write-Error "Ошибка: $($result.Error)"
        }
    } else {
        Write-Info "Пропускаем тест 5: не найден справочник для тестирования"
    }
    
    # Тест 6: Завершение импорта
    Write-Header "Тест 6: POST /api/1c/import/complete - Завершение импорта"
    
    $completeXml = @"
<?xml version="1.0" encoding="UTF-8"?>
<import_complete>
  <db_name>$global:TestDatabaseName</db_name>
</import_complete>
"@
    
    $result = Send-HttpRequest -Method "POST" -Url "$ServerUrl/api/1c/import/complete" -Body $completeXml
    
    if ($result.Success -and $result.StatusCode -eq 200) {
        Write-Success "Импорт завершен (HTTP $($result.StatusCode))"
        
        $success = Parse-XmlValue -XmlContent $result.Content -TagName "success"
        $message = Parse-XmlValue -XmlContent $result.Content -TagName "message"
        
        Write-Info "Success: $success"
        Write-Info "Message: $message"
        
    } else {
        Write-Error "Ошибка завершения импорта (HTTP $($result.StatusCode))"
        Write-Error "Ошибка: $($result.Error)"
    }
    
    # Итоги
    Write-Header "ИТОГИ ТЕСТИРОВАНИЯ"
    Write-Success "Все тесты завершены"
    Write-Info "Протестированные эндпоинты:"
    Write-Host "  1. GET  /api/1c/databases" -ForegroundColor White
    Write-Host "  2. POST /api/1c/import/handshake" -ForegroundColor White
    Write-Host "  3. POST /api/1c/import/get-constants" -ForegroundColor White
    Write-Host "  4. POST /api/1c/import/get-catalog" -ForegroundColor White
    Write-Host "  5. POST /api/1c/import/complete" -ForegroundColor White
    
}

# Запуск тестов
try {
    Test-ImportApi
} catch {
    Write-Error "Критическая ошибка при выполнении тестов: $_"
    exit 1
}



