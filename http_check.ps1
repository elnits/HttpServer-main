# Скрипт для проверки HTTP статусов веб-страниц
# Использование: .\http_check.ps1 [-Config config.json] [-Urls urls.txt] [-Output report.json]

param(
    [string]$Config = "http_check_config.json",
    [string]$Urls = "",
    [string]$Output = "",
    [string]$Log = "",
    [int]$Timeout = 7,
    [int]$Retries = 3,
    [int]$Concurrent = 5
)

$ErrorActionPreference = "Stop"

# Проверяем наличие утилиты
$checkerPath = ".\http_checker.exe"
if (-not (Test-Path $checkerPath)) {
    Write-Host "🔨 Компиляция http_checker..." -ForegroundColor Yellow
    Push-Location $PSScriptRoot
    try {
        go build -o http_checker.exe ./cmd/http_checker/main.go
        if ($LASTEXITCODE -ne 0) {
            Write-Host "❌ Ошибка компиляции" -ForegroundColor Red
            exit 1
        }
        Write-Host "✅ Компиляция завершена" -ForegroundColor Green
    } finally {
        Pop-Location
    }
}

# Формируем аргументы
$args = @()

if ($Config) {
    $args += "-config", $Config
}

if ($Urls) {
    $args += "-urls", $Urls
}

if ($Output) {
    $args += "-output", $Output
} else {
    $timestamp = Get-Date -Format "yyyyMMdd_HHmmss"
    $args += "-output", "reports\http_check_$timestamp.json"
}

if ($Log) {
    $args += "-log", $Log
}

$args += "-timeout", "${Timeout}s"
$args += "-retries", $Retries
$args += "-concurrent", $Concurrent

# Создаем директорию для отчетов
$reportsDir = "reports"
if (-not (Test-Path $reportsDir)) {
    New-Item -ItemType Directory -Path $reportsDir | Out-Null
}

Write-Host "🚀 Запуск проверки HTTP статусов..." -ForegroundColor Green
Write-Host "   Конфигурация: $Config" -ForegroundColor Gray
if ($Urls) {
    Write-Host "   Файл URL: $Urls" -ForegroundColor Gray
}
Write-Host "   Отчет: $($args[$args.IndexOf('-output') + 1])" -ForegroundColor Gray
Write-Host ""

# Запускаем проверку
& $checkerPath $args

if ($LASTEXITCODE -eq 0) {
    Write-Host ""
    Write-Host "✅ Проверка завершена успешно" -ForegroundColor Green
} else {
    Write-Host ""
    Write-Host "❌ Обнаружены критические ошибки" -ForegroundColor Red
    exit $LASTEXITCODE
}

