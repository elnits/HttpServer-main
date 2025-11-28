#!/bin/bash
# Тестовый скрипт для проверки эндпоинтов импорта данных в 1С
# Использование: ./test_1c_import_api.sh

set -e

# Настройки
SERVER_URL="http://localhost:9999"
TIMEOUT=7

# Цвета для вывода
GREEN='\033[0;32m'
RED='\033[0;31m'
CYAN='\033[0;36m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

function write_success() {
    echo -e "${GREEN}✓ $1${NC}"
}

function write_error() {
    echo -e "${RED}✗ $1${NC}"
}

function write_info() {
    echo -e "${CYAN}ℹ $1${NC}"
}

function write_header() {
    echo ""
    echo -e "${YELLOW}========================================${NC}"
    echo -e "${YELLOW}$1${NC}"
    echo -e "${YELLOW}========================================${NC}"
}

# Функция для извлечения значения из XML
function parse_xml_value() {
    local xml_content="$1"
    local tag_name="$2"
    echo "$xml_content" | grep -oP "(?<=<${tag_name}>)[^<]+" | head -1
}

# Функция для подсчета элементов в XML
function count_xml_elements() {
    local xml_content="$1"
    local tag_name="$2"
    echo "$xml_content" | grep -o "<${tag_name}>" | wc -l
}

# Главная функция тестирования
function test_import_api() {
    
    write_header "ТЕСТИРОВАНИЕ API ИМПОРТА ДАННЫХ В 1С"
    
    # Тест 1: Проверка доступности сервера
    write_header "Тест 1: Проверка доступности сервера"
    
    write_info "Проверяем health endpoint..."
    
    http_code=$(curl -s -o /dev/null -w "%{http_code}" --max-time $TIMEOUT "${SERVER_URL}/health")
    
    if [ "$http_code" = "200" ]; then
        write_success "Сервер доступен (HTTP $http_code)"
    else
        write_error "Сервер недоступен (HTTP $http_code)"
        exit 1
    fi
    
    # Тест 2: Получение списка баз
    write_header "Тест 2: GET /api/1c/databases - Получение списка баз"
    
    response=$(curl -s --max-time $TIMEOUT "${SERVER_URL}/api/1c/databases")
    http_code=$(curl -s -o /dev/null -w "%{http_code}" --max-time $TIMEOUT "${SERVER_URL}/api/1c/databases")
    
    if [ "$http_code" = "200" ]; then
        write_success "Список баз получен (HTTP $http_code)"
        
        total=$(parse_xml_value "$response" "total")
        write_info "Найдено баз: $total"
        
        database_count=$(count_xml_elements "$response" "database")
        write_info "Элементов database: $database_count"
        
        if [ "$database_count" -gt 0 ]; then
            # Парсим первую базу
            TEST_DATABASE_NAME=$(parse_xml_value "$response" "file_name")
            write_info "Первая база: $TEST_DATABASE_NAME"
            
            config_name=$(parse_xml_value "$response" "config_name")
            write_info "Конфигурация: $config_name"
            
            total_catalogs=$(parse_xml_value "$response" "total_catalogs")
            write_info "Справочников: $total_catalogs"
        else
            write_error "Не найдено ни одной базы для тестирования!"
            exit 1
        fi
    else
        write_error "Не удалось получить список баз (HTTP $http_code)"
        exit 1
    fi
    
    # Проверяем что база для тестов найдена
    if [ -z "$TEST_DATABASE_NAME" ]; then
        write_error "Не найдена база данных для тестирования!"
        exit 1
    fi
    
    # Тест 3: Handshake для импорта
    write_header "Тест 3: POST /api/1c/import/handshake - Начало импорта"
    
    handshake_xml="<?xml version=\"1.0\" encoding=\"UTF-8\"?>
<import_handshake>
  <db_name>$TEST_DATABASE_NAME</db_name>
  <client_info>
    <version_1c>8.3.25.1257</version_1c>
    <computer_name>TEST-PC</computer_name>
    <user_name>TestUser</user_name>
  </client_info>
</import_handshake>"
    
    response=$(curl -s --max-time $TIMEOUT \
        -X POST "${SERVER_URL}/api/1c/import/handshake" \
        -H "Content-Type: application/xml; charset=utf-8" \
        -d "$handshake_xml")
    
    http_code=$(curl -s -o /dev/null -w "%{http_code}" --max-time $TIMEOUT \
        -X POST "${SERVER_URL}/api/1c/import/handshake" \
        -H "Content-Type: application/xml; charset=utf-8" \
        -d "$handshake_xml")
    
    if [ "$http_code" = "200" ]; then
        write_success "Handshake успешен (HTTP $http_code)"
        
        success=$(parse_xml_value "$response" "success")
        upload_uuid=$(parse_xml_value "$response" "upload_uuid")
        constants_count=$(parse_xml_value "$response" "constants_count")
        
        write_info "Success: $success"
        write_info "Upload UUID: $upload_uuid"
        write_info "Количество констант: $constants_count"
        
        catalog_count=$(count_xml_elements "$response" "catalog")
        write_info "Количество справочников: $catalog_count"
        
        # Сохраняем имя первого справочника для следующего теста
        TEST_CATALOG_NAME=$(echo "$response" | grep -oP '(?<=<name>)[^<]+' | head -1)
        write_info "Первый справочник: $TEST_CATALOG_NAME"
    else
        write_error "Ошибка handshake (HTTP $http_code)"
        exit 1
    fi
    
    # Тест 4: Получение констант
    write_header "Тест 4: POST /api/1c/import/get-constants - Получение констант"
    
    get_constants_xml="<?xml version=\"1.0\" encoding=\"UTF-8\"?>
<import_get_constants>
  <db_name>$TEST_DATABASE_NAME</db_name>
  <offset>0</offset>
  <limit>100</limit>
</import_get_constants>"
    
    response=$(curl -s --max-time $TIMEOUT \
        -X POST "${SERVER_URL}/api/1c/import/get-constants" \
        -H "Content-Type: application/xml; charset=utf-8" \
        -d "$get_constants_xml")
    
    http_code=$(curl -s -o /dev/null -w "%{http_code}" --max-time $TIMEOUT \
        -X POST "${SERVER_URL}/api/1c/import/get-constants" \
        -H "Content-Type: application/xml; charset=utf-8" \
        -d "$get_constants_xml")
    
    if [ "$http_code" = "200" ]; then
        write_success "Константы получены (HTTP $http_code)"
        
        success=$(parse_xml_value "$response" "success")
        total=$(parse_xml_value "$response" "total")
        
        write_info "Success: $success"
        write_info "Всего констант: $total"
        
        constant_count=$(count_xml_elements "$response" "constant")
        write_info "Получено констант: $constant_count"
    else
        write_error "Ошибка получения констант (HTTP $http_code)"
    fi
    
    # Тест 5: Получение справочника
    if [ -n "$TEST_CATALOG_NAME" ]; then
        write_header "Тест 5: POST /api/1c/import/get-catalog - Получение справочника"
        
        get_catalog_xml="<?xml version=\"1.0\" encoding=\"UTF-8\"?>
<import_get_catalog>
  <db_name>$TEST_DATABASE_NAME</db_name>
  <catalog_name>$TEST_CATALOG_NAME</catalog_name>
  <offset>0</offset>
  <limit>10</limit>
</import_get_catalog>"
        
        response=$(curl -s --max-time $TIMEOUT \
            -X POST "${SERVER_URL}/api/1c/import/get-catalog" \
            -H "Content-Type: application/xml; charset=utf-8" \
            -d "$get_catalog_xml")
        
        http_code=$(curl -s -o /dev/null -w "%{http_code}" --max-time $TIMEOUT \
            -X POST "${SERVER_URL}/api/1c/import/get-catalog" \
            -H "Content-Type: application/xml; charset=utf-8" \
            -d "$get_catalog_xml")
        
        if [ "$http_code" = "200" ]; then
            write_success "Элементы справочника получены (HTTP $http_code)"
            
            success=$(parse_xml_value "$response" "success")
            catalog_name=$(parse_xml_value "$response" "catalog_name")
            total=$(parse_xml_value "$response" "total")
            
            write_info "Success: $success"
            write_info "Справочник: $catalog_name"
            write_info "Всего элементов: $total"
            
            item_count=$(count_xml_elements "$response" "item")
            write_info "Получено элементов: $item_count"
        else
            write_error "Ошибка получения справочника (HTTP $http_code)"
        fi
    else
        write_info "Пропускаем тест 5: не найден справочник для тестирования"
    fi
    
    # Тест 6: Завершение импорта
    write_header "Тест 6: POST /api/1c/import/complete - Завершение импорта"
    
    complete_xml="<?xml version=\"1.0\" encoding=\"UTF-8\"?>
<import_complete>
  <db_name>$TEST_DATABASE_NAME</db_name>
</import_complete>"
    
    response=$(curl -s --max-time $TIMEOUT \
        -X POST "${SERVER_URL}/api/1c/import/complete" \
        -H "Content-Type: application/xml; charset=utf-8" \
        -d "$complete_xml")
    
    http_code=$(curl -s -o /dev/null -w "%{http_code}" --max-time $TIMEOUT \
        -X POST "${SERVER_URL}/api/1c/import/complete" \
        -H "Content-Type: application/xml; charset=utf-8" \
        -d "$complete_xml")
    
    if [ "$http_code" = "200" ]; then
        write_success "Импорт завершен (HTTP $http_code)"
        
        success=$(parse_xml_value "$response" "success")
        message=$(parse_xml_value "$response" "message")
        
        write_info "Success: $success"
        write_info "Message: $message"
    else
        write_error "Ошибка завершения импорта (HTTP $http_code)"
    fi
    
    # Итоги
    write_header "ИТОГИ ТЕСТИРОВАНИЯ"
    write_success "Все тесты завершены"
    write_info "Протестированные эндпоинты:"
    echo "  1. GET  /api/1c/databases"
    echo "  2. POST /api/1c/import/handshake"
    echo "  3. POST /api/1c/import/get-constants"
    echo "  4. POST /api/1c/import/get-catalog"
    echo "  5. POST /api/1c/import/complete"
}

# Запуск тестов
test_import_api



