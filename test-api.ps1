# API Testing Script
$baseUrl = "http://localhost:9999"
$passed = 0
$failed = 0

function Test-Endpoint {
    param(
        [string]$Method,
        [string]$Endpoint,
        [string]$Body = $null,
        [int]$ExpectedStatus = 200
    )
    
    Write-Host "Testing $Method $Endpoint..." -ForegroundColor Cyan
    
    try {
        $headers = @{"Content-Type" = "application/json"}
        
        if ($Body) {
            $response = Invoke-WebRequest -Uri "$baseUrl$Endpoint" -Method $Method -Headers $headers -Body $Body -UseBasicParsing -ErrorAction Stop
        } else {
            $response = Invoke-WebRequest -Uri "$baseUrl$Endpoint" -Method $Method -Headers $headers -UseBasicParsing -ErrorAction Stop
        }
        
        $status = $response.StatusCode
        
        if ($status -eq $ExpectedStatus) {
            Write-Host "PASS: Status $status" -ForegroundColor Green
            $script:passed++
            return $true
        } else {
            Write-Host "FAIL: Expected $ExpectedStatus, got $status" -ForegroundColor Red
            $script:failed++
            return $false
        }
    } catch {
        $statusCode = $_.Exception.Response.StatusCode.value__
        Write-Host "FAIL: $statusCode" -ForegroundColor Red
        $script:failed++
        return $false
    }
}

Write-Host "========================================"
Write-Host "API Testing Suite"
Write-Host "========================================"
Write-Host ""

Write-Host "1. Health Check" -ForegroundColor Magenta
Test-Endpoint -Method "GET" -Endpoint "/health"
Write-Host ""

Write-Host "2. Database Info" -ForegroundColor Magenta
Test-Endpoint -Method "GET" -Endpoint "/api/database/info"
Write-Host ""

Write-Host "3. Clients API" -ForegroundColor Magenta
Test-Endpoint -Method "GET" -Endpoint "/api/clients"

$clientData = '{"name":"Test Client","legal_name":"Test Legal Name","description":"Test Description","contact_email":"test@test.com","contact_phone":"+1234567890","tax_id":"123456789"}'
$createResult = Test-Endpoint -Method "POST" -Endpoint "/api/clients" -Body $clientData -ExpectedStatus 201
Write-Host ""

Write-Host "4. Normalization API" -ForegroundColor Magenta
Test-Endpoint -Method "GET" -Endpoint "/api/normalization/status"
Test-Endpoint -Method "GET" -Endpoint "/api/normalization/stats"
Write-Host ""

Write-Host "5. Databases API" -ForegroundColor Magenta
Test-Endpoint -Method "GET" -Endpoint "/api/databases/list"
Write-Host ""

Write-Host "========================================"
Write-Host "Test Summary"
Write-Host "========================================"
$total = $passed + $failed
Write-Host "Total Tests: $total"
Write-Host "Passed: $passed" -ForegroundColor Green
Write-Host "Failed: $failed" -ForegroundColor Red
