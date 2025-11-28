# Скрипт для нормализации с КПВЭД классификацией
# Использование: .\normalize_with_kpved.ps1 [путь_к_базе.db]

param(
    [string]$DatabasePath = "1c_data.db"
)

# Используем API ключ из переменной окружения или дефолтный
$ApiKey = $env:ARLIAI_API_KEY
if (-not $ApiKey -or $ApiKey -eq "") {
    $ApiKey = "597dbe7e-16ca-4803-ab17-5fa084909f37"
}

if (-not $ApiKey) {
    Write-Host "⚠️  ARLIAI_API_KEY не установлен" -ForegroundColor Yellow
    Write-Host "💡 Установите переменную окружения ARLIAI_API_KEY для работы КПВЭД классификации" -ForegroundColor Yellow
    Write-Host ""
    Write-Host "Пример в PowerShell:" -ForegroundColor Cyan
    Write-Host '  $env:ARLIAI_API_KEY="your-api-key-here"' -ForegroundColor Cyan
    Write-Host ""
    $continue = Read-Host "Продолжить без КПВЭД классификации? (y/n)"
    if ($continue -ne "y") {
        exit 1
    }
}

if (-not (Test-Path $DatabasePath)) {
    Write-Host "❌ База данных не найдена: $DatabasePath" -ForegroundColor Red
    exit 1
}

Write-Host "🚀 Запуск нормализации с КПВЭД классификацией..." -ForegroundColor Green
Write-Host "📁 База данных: $DatabasePath" -ForegroundColor Cyan

if ($ApiKey) {
    $env:ARLIAI_API_KEY = $ApiKey
    $env:ARLIAI_MODEL = "GLM-4.5-Air"
    Write-Host "✓ API ключ установлен" -ForegroundColor Green
    Write-Host "✓ Модель: $env:ARLIAI_MODEL" -ForegroundColor Green
    Write-Host ""
    .\normalize.exe -db $DatabasePath -ai
} else {
    Write-Host "⚠️  Запуск без AI (только базовая нормализация)" -ForegroundColor Yellow
    Write-Host ""
    .\normalize.exe -db $DatabasePath
}

Write-Host ""
Write-Host "✅ Нормализация завершена!" -ForegroundColor Green

