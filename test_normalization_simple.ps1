# Simple normalization API test
Write-Host "=== Testing Normalization API ===" -ForegroundColor Cyan

$baseUrl = "http://localhost:9999"

# Test 1: Status
Write-Host "`n1. Testing status endpoint..." -ForegroundColor Yellow
try {
    $status = Invoke-RestMethod -Uri "$baseUrl/api/normalization/status" -Method GET -TimeoutSec 7
    Write-Host "Status: OK" -ForegroundColor Green
    Write-Host "  isRunning: $($status.isRunning)" -ForegroundColor Gray
    Write-Host "  processed: $($status.processed)" -ForegroundColor Gray
} catch {
    Write-Host "Status: FAILED - $($_.Exception.Message)" -ForegroundColor Red
}

# Test 2: Start normalization
Write-Host "`n2. Testing start endpoint..." -ForegroundColor Yellow
try {
    $body = @{
        use_ai = $false
        min_confidence = 0.8
        database = ""
    } | ConvertTo-Json
    
    $start = Invoke-RestMethod -Uri "$baseUrl/api/normalize/start" -Method POST -Body $body -ContentType "application/json" -TimeoutSec 7
    Write-Host "Start: OK" -ForegroundColor Green
    Write-Host "  success: $($start.success)" -ForegroundColor Gray
    Write-Host "  message: $($start.message)" -ForegroundColor Gray
} catch {
    Write-Host "Start: FAILED - $($_.Exception.Message)" -ForegroundColor Red
}

# Test 3: Check status after start
Write-Host "`n3. Checking status after start..." -ForegroundColor Yellow
Start-Sleep -Seconds 2
try {
    $statusAfter = Invoke-RestMethod -Uri "$baseUrl/api/normalization/status" -Method GET -TimeoutSec 7
    Write-Host "Status After: OK" -ForegroundColor Green
    Write-Host "  isRunning: $($statusAfter.isRunning)" -ForegroundColor Gray
    Write-Host "  processed: $($statusAfter.processed)" -ForegroundColor Gray
    Write-Host "  progress: $($statusAfter.progress)" -ForegroundColor Gray
} catch {
    Write-Host "Status After: FAILED - $($_.Exception.Message)" -ForegroundColor Red
}

Write-Host "`n=== Test completed ===" -ForegroundColor Cyan

