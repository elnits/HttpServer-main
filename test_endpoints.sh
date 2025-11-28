#!/bin/bash

# Тестовый скрипт для проверки API endpoints
# Использование: ./test_endpoints.sh [base_url]

BASE_URL="${1:-http://localhost:8080}"

echo "Testing API Endpoints for Versioning and Classification"
echo "========================================================"
echo ""

# Цвета для вывода
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Функция для тестирования endpoint
test_endpoint() {
    local method=$1
    local url=$2
    local data=$3
    local expected_status=$4
    local description=$5

    echo -n "Testing $description... "
    
    if [ -z "$data" ]; then
        response=$(curl -s -w "\n%{http_code}" -X "$method" "$url" 2>/dev/null)
    else
        response=$(curl -s -w "\n%{http_code}" -X "$method" "$url" \
            -H "Content-Type: application/json" \
            -d "$data" 2>/dev/null)
    fi
    
    http_code=$(echo "$response" | tail -n1)
    body=$(echo "$response" | sed '$d')
    
    if [ "$http_code" == "$expected_status" ]; then
        echo -e "${GREEN}✓${NC} (Status: $http_code)"
        if [ ! -z "$body" ]; then
            echo "$body" | python -m json.tool 2>/dev/null || echo "$body"
        fi
        return 0
    else
        echo -e "${RED}✗${NC} (Expected: $expected_status, Got: $http_code)"
        echo "Response: $body"
        return 1
    fi
}

# Тесты для версионирования
echo -e "${YELLOW}=== Versioning Endpoints ===${NC}"
echo ""

# 1. Start Normalization
SESSION_DATA='{"item_id": 1, "original_name": "Тестовый товар"}'
test_endpoint "POST" "$BASE_URL/api/normalization/start" "$SESSION_DATA" "200" "POST /api/normalization/start"

# Извлекаем session_id из ответа (если сервер запущен)
SESSION_ID=$(curl -s -X POST "$BASE_URL/api/normalization/start" \
    -H "Content-Type: application/json" \
    -d "$SESSION_DATA" 2>/dev/null | python -c "import sys, json; print(json.load(sys.stdin).get('session_id', 0))" 2>/dev/null)

if [ "$SESSION_ID" != "0" ] && [ ! -z "$SESSION_ID" ]; then
    echo "Created session ID: $SESSION_ID"
    echo ""
    
    # 2. Apply Patterns
    PATTERNS_DATA="{\"session_id\": $SESSION_ID, \"stage_type\": \"algorithmic\"}"
    test_endpoint "POST" "$BASE_URL/api/normalization/apply-patterns" "$PATTERNS_DATA" "200" "POST /api/normalization/apply-patterns"
    
    # 3. Get History
    test_endpoint "GET" "$BASE_URL/api/normalization/history?session_id=$SESSION_ID" "" "200" "GET /api/normalization/history"
    
    # 4. Revert Stage
    REVERT_DATA="{\"session_id\": $SESSION_ID, \"target_stage\": 1}"
    test_endpoint "POST" "$BASE_URL/api/normalization/revert" "$REVERT_DATA" "200" "POST /api/normalization/revert"
else
    echo -e "${YELLOW}Warning: Could not create session. Make sure server is running.${NC}"
fi

echo ""
echo -e "${YELLOW}=== Classification Endpoints ===${NC}"
echo ""

# 5. Get Strategies
test_endpoint "GET" "$BASE_URL/api/classification/strategies" "" "200" "GET /api/classification/strategies"

# 6. Get Available Strategies
test_endpoint "GET" "$BASE_URL/api/classification/available" "" "200" "GET /api/classification/available"

# 7. Get Client Strategies
test_endpoint "GET" "$BASE_URL/api/classification/strategies/client?client_id=1" "" "200" "GET /api/classification/strategies/client"

# 8. Create Client Strategy
STRATEGY_DATA='{
    "client_id": 1,
    "name": "Тестовая стратегия",
    "description": "Описание тестовой стратегии",
    "max_depth": 2,
    "priority": ["0", "1"],
    "rules": []
}'
test_endpoint "POST" "$BASE_URL/api/classification/strategies/create" "$STRATEGY_DATA" "201" "POST /api/classification/strategies/create"

# 9. Classify Item Direct
CLASSIFY_DATA='{
    "item_name": "Тестовый товар",
    "item_code": "TEST001",
    "strategy_id": "top_priority"
}'
test_endpoint "POST" "$BASE_URL/api/classification/classify-item" "$CLASSIFY_DATA" "200" "POST /api/classification/classify-item"

echo ""
echo "========================================================"
echo "Testing completed!"

