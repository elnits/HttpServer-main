# Simple test for Arliai integration
# Test script for normalization with Arliai

$baseUrl = "http://localhost:9999"

Write-Host "=== Arliai Integration Test ===" -ForegroundColor Cyan
Write-Host ""

# Check API key
$apiKey = $env:ARLIAI_API_KEY
if (-not $apiKey) {
    Write-Host "WARNING: ARLIAI_API_KEY not set" -ForegroundColor Yellow
    Write-Host "Testing will use fallback to rules only" -ForegroundColor Yellow
    $useAI = $false
} else {
    Write-Host "OK: ARLIAI_API_KEY found" -ForegroundColor Green
    $useAI = $true
}
Write-Host ""

# Test 1: Check status
Write-Host "Test 1: Check normalization status" -ForegroundColor Cyan
try {
    $response = Invoke-RestMethod -Uri "$baseUrl/api/normalization/status" -Method GET -TimeoutSec 7
    Write-Host "OK: Server is available" -ForegroundColor Green
    Write-Host "  Status: $($response.status)" -ForegroundColor Gray
} catch {
    Write-Host "ERROR: Server unavailable: $_" -ForegroundColor Red
    exit 1
}
Write-Host ""

# Test 2: Start normalization WITHOUT AI
Write-Host "Test 2: Start normalization WITHOUT AI" -ForegroundColor Cyan
$bodyWithoutAI = @{
    use_ai = $false
    min_confidence = 0.8
    rate_limit_delay_ms = 100
    max_retries = 3
} | ConvertTo-Json

try {
    $response = Invoke-RestMethod -Uri "$baseUrl/api/normalize/start" -Method POST -Body $bodyWithoutAI -ContentType "application/json" -TimeoutSec 7
    Write-Host "OK: Normalization started without AI" -ForegroundColor Green
    Write-Host "  Message: $($response.message)" -ForegroundColor Gray
    
    Start-Sleep -Seconds 2
    $statusResponse = Invoke-RestMethod -Uri "$baseUrl/api/normalization/status" -Method GET -TimeoutSec 7
    Write-Host "  Current status: $($statusResponse.status)" -ForegroundColor Gray
} catch {
    Write-Host "ERROR: Failed to start normalization: $_" -ForegroundColor Red
}
Write-Host ""

# Test 3: Start normalization WITH AI (if key is set)
if ($useAI) {
    Write-Host "Test 3: Start normalization WITH AI" -ForegroundColor Cyan
    
    # Stop previous process
    Write-Host "  Stopping previous process..." -ForegroundColor Gray
    try {
        Invoke-RestMethod -Uri "$baseUrl/api/normalization/stop" -Method POST -TimeoutSec 7 | Out-Null
        Start-Sleep -Seconds 1
    } catch {
        # Ignore errors
    }
    
    $bodyWithAI = @{
        use_ai = $true
        min_confidence = 0.8
        rate_limit_delay_ms = 500
        max_retries = 3
    } | ConvertTo-Json
    
    try {
        $response = Invoke-RestMethod -Uri "$baseUrl/api/normalize/start" -Method POST -Body $bodyWithAI -ContentType "application/json" -TimeoutSec 7
        Write-Host "OK: Normalization started with AI" -ForegroundColor Green
        Write-Host "  Message: $($response.message)" -ForegroundColor Gray
        
        Start-Sleep -Seconds 2
        $statusResponse = Invoke-RestMethod -Uri "$baseUrl/api/normalization/status" -Method GET -TimeoutSec 7
        Write-Host "  Current status: $($statusResponse.status)" -ForegroundColor Gray
    } catch {
        Write-Host "ERROR: Failed to start normalization with AI: $_" -ForegroundColor Red
    }
    Write-Host ""
} else {
    Write-Host "Test 3: Skipped (ARLIAI_API_KEY not set)" -ForegroundColor Yellow
    Write-Host ""
}

# Test 4: Check statistics
Write-Host "Test 4: Check normalization statistics" -ForegroundColor Cyan
try {
    $response = Invoke-RestMethod -Uri "$baseUrl/api/normalization/stats" -Method GET -TimeoutSec 7
    Write-Host "OK: Statistics retrieved" -ForegroundColor Green
    Write-Host "  Total items: $($response.total_items)" -ForegroundColor Gray
    Write-Host "  Processed: $($response.processed)" -ForegroundColor Gray
    Write-Host "  Completed: $($response.completed)" -ForegroundColor Gray
    Write-Host "  Errors: $($response.errors)" -ForegroundColor Gray
    Write-Host "  Pending: $($response.pending)" -ForegroundColor Gray
} catch {
    Write-Host "ERROR: Failed to get statistics: $_" -ForegroundColor Red
}
Write-Host ""

# Test 5: Check groups
Write-Host "Test 5: Check normalization groups" -ForegroundColor Cyan
try {
    $response = Invoke-RestMethod -Uri "$baseUrl/api/normalization/groups?limit=5" -Method GET -TimeoutSec 7
    Write-Host "OK: Groups retrieved" -ForegroundColor Green
    $groups = $response.groups
    Write-Host "  Number of groups (first 5): $($groups.Count)" -ForegroundColor Gray
    foreach ($group in $groups | Select-Object -First 3) {
        Write-Host "    - $($group.normalized_name) [$($group.category)] (merged: $($group.merged_count))" -ForegroundColor Gray
        if ($group.ai_confidence -gt 0) {
            Write-Host "      AI confidence: $($group.ai_confidence)" -ForegroundColor Cyan
        }
    }
} catch {
    Write-Host "ERROR: Failed to get groups: $_" -ForegroundColor Red
}
Write-Host ""

# Final report
Write-Host "=== Final Report ===" -ForegroundColor Cyan
Write-Host ""

try {
    $finalStatus = Invoke-RestMethod -Uri "$baseUrl/api/normalization/status" -Method GET -TimeoutSec 7
    Write-Host "Final status: $($finalStatus.status)" -ForegroundColor $(if ($finalStatus.status -eq "idle") { "Green" } else { "Yellow" })
} catch {
    Write-Host "Could not get final status" -ForegroundColor Red
}

try {
    $finalStats = Invoke-RestMethod -Uri "$baseUrl/api/normalization/stats" -Method GET -TimeoutSec 7
    Write-Host ""
    Write-Host "Normalization statistics:" -ForegroundColor Cyan
    Write-Host "  Total items: $($finalStats.total_items)" -ForegroundColor White
    Write-Host "  Processed: $($finalStats.processed)" -ForegroundColor White
    Write-Host "  Completed: $($finalStats.completed)" -ForegroundColor $(if ($finalStats.completed -gt 0) { "Green" } else { "Gray" })
    Write-Host "  Errors: $($finalStats.errors)" -ForegroundColor $(if ($finalStats.errors -gt 0) { "Red" } else { "Gray" })
    Write-Host "  Pending: $($finalStats.pending)" -ForegroundColor $(if ($finalStats.pending -gt 0) { "Yellow" } else { "Gray" })
} catch {
    Write-Host "Could not get final statistics" -ForegroundColor Red
}

Write-Host ""
Write-Host "=== Testing completed ===" -ForegroundColor Cyan
Write-Host ""
Write-Host "Recommendations:" -ForegroundColor Yellow
Write-Host "1. Check server logs for detailed information" -ForegroundColor Gray
Write-Host "2. Use SSE endpoint for real-time monitoring: curl -N $baseUrl/api/normalize/events" -ForegroundColor Gray
Write-Host "3. Check normalized_data table in database for results" -ForegroundColor Gray
if (-not $useAI) {
    Write-Host "4. Set ARLIAI_API_KEY to test AI functionality" -ForegroundColor Gray
}

