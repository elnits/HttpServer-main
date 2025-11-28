# Запуск сервера с API ключом для переклассификации

$env:ARLIAI_API_KEY = "597dbe7e-16ca-4803-ab17-5fa084909f37"
$env:ARLIAI_MODEL = "GLM-4.5-Air"

Write-Host "🚀 Запуск сервера с API ключом..." -ForegroundColor Green
Write-Host "   ARLIAI_API_KEY установлен" -ForegroundColor Gray
Write-Host "   ARLIAI_MODEL: $env:ARLIAI_MODEL" -ForegroundColor Gray
Write-Host ""

# Запускаем сервер в фоне
Start-Process -FilePath "go" -ArgumentList "run", "main.go" -NoNewWindow -PassThru | Out-Null

Start-Sleep -Seconds 5

# Проверяем, что сервер запустился
try {
    $response = Invoke-WebRequest -Uri "http://localhost:9999/health" -Method GET -TimeoutSec 3 -UseBasicParsing
    if ($response.StatusCode -eq 200) {
        Write-Host "✅ Сервер успешно запущен на порту 9999" -ForegroundColor Green
        Write-Host "   Health check: $($response.Content)" -ForegroundColor Gray
    }
} catch {
    Write-Host "❌ Сервер не отвечает: $_" -ForegroundColor Red
}

