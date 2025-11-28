#!/bin/bash
# Тест интеграции с Arliai для процесса нормализации

echo "=== Тест интеграции Arliai для нормализации ==="
echo ""

# Проверка наличия API ключа
if [ -z "$ARLIAI_API_KEY" ]; then
    echo "WARNING: ARLIAI_API_KEY не установлен"
    echo "Тестирование будет проводиться только в режиме без AI (fallback на правила)"
    echo ""
    USE_AI=false
else
    echo "OK: ARLIAI_API_KEY найден"
    echo "Тестирование будет проводиться с AI и без AI"
    echo ""
    USE_AI=true
fi

# URL сервера
BASE_URL="http://localhost:9999"

# Функция для отправки запроса
send_request() {
    local method=$1
    local endpoint=$2
    local body=$3
    
    if [ -n "$body" ]; then
        curl -s -X "$method" \
            -H "Content-Type: application/json" \
            -d "$body" \
            --max-time 7 \
            "$BASE_URL$endpoint"
    else
        curl -s -X "$method" \
            -H "Content-Type: application/json" \
            --max-time 7 \
            "$BASE_URL$endpoint"
    fi
}

# Тест 1: Проверка статуса сервера
echo "Тест 1: Проверка статуса сервера"
STATUS_RESPONSE=$(send_request "GET" "/api/normalize/status")
if echo "$STATUS_RESPONSE" | grep -q "status"; then
    echo "OK: Сервер доступен"
    echo "$STATUS_RESPONSE" | grep -o '"status":"[^"]*"' | head -1
else
    echo "ERROR: Сервер недоступен"
    echo "Убедитесь, что сервер запущен на $BASE_URL"
    exit 1
fi
echo ""

# Тест 2: Нормализация БЕЗ AI (fallback на правила)
echo "Тест 2: Нормализация БЕЗ AI (fallback на правила)"
BODY_WITHOUT_AI='{"use_ai":false,"min_confidence":0.8,"rate_limit_delay_ms":100,"max_retries":3}'
RESULT=$(send_request "POST" "/api/normalize/start" "$BODY_WITHOUT_AI")
if echo "$RESULT" | grep -q "success"; then
    echo "OK: Запрос на нормализацию без AI отправлен"
    echo "$RESULT" | grep -o '"message":"[^"]*"' | head -1
    sleep 2
    STATUS_RESPONSE=$(send_request "GET" "/api/normalize/status")
    echo "$STATUS_RESPONSE" | grep -o '"status":"[^"]*"' | head -1
else
    echo "ERROR: Ошибка при запуске нормализации"
    echo "$RESULT"
fi
echo ""

# Тест 3: Нормализация С AI (если ключ установлен)
if [ "$USE_AI" = true ]; then
    echo "Тест 3: Нормализация С AI"
    BODY_WITH_AI='{"use_ai":true,"min_confidence":0.8,"rate_limit_delay_ms":500,"max_retries":3}'
    
    # Останавливаем предыдущий процесс
    echo "  Остановка предыдущего процесса (если запущен)..."
    send_request "POST" "/api/normalize/stop" > /dev/null
    sleep 1
    
    RESULT=$(send_request "POST" "/api/normalize/start" "$BODY_WITH_AI")
    if echo "$RESULT" | grep -q "success"; then
        echo "OK: Запрос на нормализацию с AI отправлен"
        echo "$RESULT" | grep -o '"message":"[^"]*"' | head -1
        sleep 2
        STATUS_RESPONSE=$(send_request "GET" "/api/normalize/status")
        echo "$STATUS_RESPONSE" | grep -o '"status":"[^"]*"' | head -1
    else
        echo "ERROR: Ошибка при запуске нормализации с AI"
        echo "$RESULT"
    fi
    echo ""
else
    echo "Тест 3: Пропущен (ARLIAI_API_KEY не установлен)"
    echo ""
fi

# Тест 4: Проверка статистики
echo "Тест 4: Проверка статистики нормализации"
STATS_RESPONSE=$(send_request "GET" "/api/normalize/stats")
if echo "$STATS_RESPONSE" | grep -q "total_items"; then
    echo "OK: Статистика получена"
    echo "$STATS_RESPONSE" | grep -o '"total_items":[0-9]*' | head -1
    echo "$STATS_RESPONSE" | grep -o '"processed":[0-9]*' | head -1
    echo "$STATS_RESPONSE" | grep -o '"completed":[0-9]*' | head -1
    echo "$STATS_RESPONSE" | grep -o '"errors":[0-9]*' | head -1
    echo "$STATS_RESPONSE" | grep -o '"pending":[0-9]*' | head -1
else
    echo "ERROR: Ошибка при получении статистики"
    echo "$STATS_RESPONSE"
fi
echo ""

# Тест 5: Проверка групп
echo "Тест 5: Проверка групп нормализации"
GROUPS_RESPONSE=$(send_request "GET" "/api/normalize/groups?limit=5")
if echo "$GROUPS_RESPONSE" | grep -q "groups"; then
    echo "OK: Группы получены"
    echo "$GROUPS_RESPONSE" | head -c 500
else
    echo "ERROR: Ошибка при получении групп"
    echo "$GROUPS_RESPONSE"
fi
echo ""

# Итоговый отчет
echo "=== Итоговый отчет ==="
echo ""

FINAL_STATUS=$(send_request "GET" "/api/normalize/status")
if echo "$FINAL_STATUS" | grep -q "status"; then
    STATUS=$(echo "$FINAL_STATUS" | grep -o '"status":"[^"]*"' | head -1 | cut -d'"' -f4)
    echo "Финальный статус: $STATUS"
fi

FINAL_STATS=$(send_request "GET" "/api/normalize/stats")
if echo "$FINAL_STATS" | grep -q "total_items"; then
    echo ""
    echo "Статистика нормализации:"
    echo "$FINAL_STATS" | grep -o '"total_items":[0-9]*' | head -1
    echo "$FINAL_STATS" | grep -o '"processed":[0-9]*' | head -1
    echo "$FINAL_STATS" | grep -o '"completed":[0-9]*' | head -1
    echo "$FINAL_STATS" | grep -o '"errors":[0-9]*' | head -1
    echo "$FINAL_STATS" | grep -o '"pending":[0-9]*' | head -1
fi

echo ""
echo "=== Тестирование завершено ==="
echo ""
echo "Рекомендации:"
echo "1. Проверьте логи сервера для детальной информации"
echo "2. Используйте SSE эндпоинт для мониторинга процесса в реальном времени"
echo "3. Проверьте таблицу normalized_data в базе данных для результатов"
if [ "$USE_AI" = false ]; then
    echo "4. Установите ARLIAI_API_KEY для тестирования AI функциональности"
fi

