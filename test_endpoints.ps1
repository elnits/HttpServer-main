# PowerShell скрипт для тестирования API endpoints
# Использование: .\test_endpoints.ps1 [base_url]

param(
    [string]$BaseUrl = "http://localhost:8080"
)

Write-Host "Testing API Endpoints for Versioning and Classification" -ForegroundColor Cyan
Write-Host "========================================================" -ForegroundColor Cyan
Write-Host ""

$sessionId = $null

# Функция для тестирования endpoint
function Test-Endpoint {
    param(
        [string]$Method,
        [string]$Url,
        [string]$Data = $null,
        [int]$ExpectedStatus = 200,
        [string]$Description
    )
    
    Write-Host -NoNewline "Testing $Description... "
    
    try {
        $headers = @{
            "Content-Type" = "application/json"
        }
        
        if ($Data) {
            $response = Invoke-WebRequest -Uri $Url -Method $Method -Headers $headers -Body $Data -ErrorAction Stop
        } else {
            $response = Invoke-WebRequest -Uri $Url -Method $Method -ErrorAction Stop
        }
        
        if ($response.StatusCode -eq $ExpectedStatus) {
            Write-Host "✓" -ForegroundColor Green -NoNewline
            Write-Host " (Status: $($response.StatusCode))"
            
            if ($response.Content) {
                try {
                    $json = $response.Content | ConvertFrom-Json
                    $json | ConvertTo-Json -Depth 10 | Write-Host
                } catch {
                    Write-Host $response.Content
                }
            }
            return $true
        } else {
            Write-Host "✗" -ForegroundColor Red -NoNewline
            Write-Host " (Expected: $ExpectedStatus, Got: $($response.StatusCode))"
            Write-Host "Response: $($response.Content)"
            return $false
        }
    } catch {
        Write-Host "✗" -ForegroundColor Red -NoNewline
        Write-Host " (Error: $($_.Exception.Message))"
        return $false
    }
}

# Тесты для версионирования
Write-Host "=== Versioning Endpoints ===" -ForegroundColor Yellow
Write-Host ""

# 1. Start Normalization
$sessionData = @{
    item_id = 1
    original_name = "Тестовый товар"
} | ConvertTo-Json

if (Test-Endpoint -Method "POST" -Url "$BaseUrl/api/normalization/start" -Data $sessionData -ExpectedStatus 200 -Description "POST /api/normalization/start") {
    try {
        $response = Invoke-RestMethod -Uri "$BaseUrl/api/normalization/start" -Method POST -Headers @{"Content-Type"="application/json"} -Body $sessionData
        $sessionId = $response.session_id
        Write-Host "Created session ID: $sessionId" -ForegroundColor Green
        Write-Host ""
    } catch {
        Write-Host "Warning: Could not extract session ID" -ForegroundColor Yellow
    }
}

if ($sessionId) {
    # 2. Apply Patterns
    $patternsData = @{
        session_id = $sessionId
        stage_type = "algorithmic"
    } | ConvertTo-Json
    
    Test-Endpoint -Method "POST" -Url "$BaseUrl/api/normalization/apply-patterns" -Data $patternsData -ExpectedStatus 200 -Description "POST /api/normalization/apply-patterns"
    
    # 3. Get History
    Test-Endpoint -Method "GET" -Url "$BaseUrl/api/normalization/history?session_id=$sessionId" -ExpectedStatus 200 -Description "GET /api/normalization/history"
    
    # 4. Revert Stage
    $revertData = @{
        session_id = $sessionId
        target_stage = 1
    } | ConvertTo-Json
    
    Test-Endpoint -Method "POST" -Url "$BaseUrl/api/normalization/revert" -Data $revertData -ExpectedStatus 200 -Description "POST /api/normalization/revert"
}

Write-Host ""
Write-Host "=== Classification Endpoints ===" -ForegroundColor Yellow
Write-Host ""

# 5. Get Strategies
Test-Endpoint -Method "GET" -Url "$BaseUrl/api/classification/strategies" -ExpectedStatus 200 -Description "GET /api/classification/strategies"

# 6. Get Available Strategies
Test-Endpoint -Method "GET" -Url "$BaseUrl/api/classification/available" -ExpectedStatus 200 -Description "GET /api/classification/available"

# 7. Get Client Strategies
Test-Endpoint -Method "GET" -Url "$BaseUrl/api/classification/strategies/client?client_id=1" -ExpectedStatus 200 -Description "GET /api/classification/strategies/client"

# 8. Create Client Strategy
$strategyData = @{
    client_id = 1
    name = "Тестовая стратегия"
    description = "Описание тестовой стратегии"
    max_depth = 2
    priority = @("0", "1")
    rules = @()
} | ConvertTo-Json

Test-Endpoint -Method "POST" -Url "$BaseUrl/api/classification/strategies/create" -Data $strategyData -ExpectedStatus 201 -Description "POST /api/classification/strategies/create"

# 9. Classify Item Direct
$classifyData = @{
    item_name = "Тестовый товар"
    item_code = "TEST001"
    strategy_id = "top_priority"
} | ConvertTo-Json

Test-Endpoint -Method "POST" -Url "$BaseUrl/api/classification/classify-item" -Data $classifyData -ExpectedStatus 200 -Description "POST /api/classification/classify-item"

Write-Host ""
Write-Host "========================================================" -ForegroundColor Cyan
Write-Host "Testing completed!" -ForegroundColor Green

