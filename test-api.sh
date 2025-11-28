#!/bin/bash
# Скрипт для тестирования API endpoints
# Использование: ./test-api.sh

BASE_URL="http://localhost:9999"
PASSED=0
FAILED=0

test_endpoint() {
    local method=$1
    local endpoint=$2
    local body=$3
    local expected_status=${4:-200}
    
    echo -e "\033[0;36mTesting $method $endpoint...\033[0m"
    
    if [ -n "$body" ]; then
        response=$(curl -s -w "\n%{http_code}" -X "$method" \
            -H "Content-Type: application/json" \
            -d "$body" \
            "$BASE_URL$endpoint")
    else
        response=$(curl -s -w "\n%{http_code}" -X "$method" \
            "$BASE_URL$endpoint")
    fi
    
    http_code=$(echo "$response" | tail -n1)
    body_content=$(echo "$response" | sed '$d')
    
    if [ "$http_code" -eq "$expected_status" ]; then
        echo -e "\033[0;32m✓ PASS: Status $http_code\033[0m"
        ((PASSED++))
        return 0
    else
        echo -e "\033[0;31m✗ FAIL: Expected $expected_status, got $http_code\033[0m"
        ((FAILED++))
        return 1
    fi
}

echo "========================================"
echo "API Testing Suite"
echo "========================================"
echo ""

# 1. Health Check
echo -e "\033[0;35m1. Health Check\033[0m"
test_endpoint "GET" "/health"
echo ""

# 2. Database Info
echo -e "\033[0;35m2. Database Info\033[0m"
test_endpoint "GET" "/api/database/info"
echo ""

# 3. Clients API
echo -e "\033[0;35m3. Clients API\033[0m"
test_endpoint "GET" "/api/clients"

CLIENT_DATA='{"name":"Test Client","legal_name":"Test Legal Name","description":"Test Description","contact_email":"test@test.com","contact_phone":"+1234567890","tax_id":"123456789"}'
test_endpoint "POST" "/api/clients" "$CLIENT_DATA" "201"

# Get client ID if creation succeeded
if [ $? -eq 0 ]; then
    clients=$(curl -s "$BASE_URL/api/clients")
    client_id=$(echo "$clients" | grep -o '"id":[0-9]*' | head -1 | grep -o '[0-9]*')
    if [ -n "$client_id" ]; then
        echo "Testing client detail endpoint with ID: $client_id"
        test_endpoint "GET" "/api/clients/$client_id"
        test_endpoint "GET" "/api/clients/$client_id/projects"
    fi
fi
echo ""

# 4. Normalization API
echo -e "\033[0;35m4. Normalization API\033[0m"
test_endpoint "GET" "/api/normalization/status"
test_endpoint "GET" "/api/normalization/stats"
echo ""

# 5. Databases API
echo -e "\033[0;35m5. Databases API\033[0m"
test_endpoint "GET" "/api/databases/list"
echo ""

# Summary
echo "========================================"
echo "Test Summary"
echo "========================================"
TOTAL=$((PASSED + FAILED))
echo "Total Tests: $TOTAL"
echo -e "Passed: \033[0;32m$PASSED\033[0m"
echo -e "Failed: \033[0;31m$FAILED\033[0m"

if [ $FAILED -gt 0 ]; then
    exit 1
fi

exit 0

