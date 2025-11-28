# Скрипт для автоматического запуска HTTP проверок по расписанию
# Использование: Настройте Task Scheduler для запуска этого скрипта

param(
    [string]$Config = "http_check_config.json",
    [string]$OutputDir = "reports",
    [string]$LogDir = "logs"
)

$ErrorActionPreference = "Continue"

# Создаем директории
if (-not (Test-Path $OutputDir)) {
    New-Item -ItemType Directory -Path $OutputDir | Out-Null
}

if (-not (Test-Path $LogDir)) {
    New-Item -ItemType Directory -Path $LogDir | Out-Null
}

# Генерируем имена файлов с временной меткой
$timestamp = Get-Date -Format "yyyyMMdd_HHmmss"
$outputFile = Join-Path $OutputDir "http_check_$timestamp.json"
$logFile = Join-Path $LogDir "http_check_$timestamp.log"

Write-Host "🕐 Запуск запланированной проверки HTTP статусов" -ForegroundColor Cyan
Write-Host "   Время: $(Get-Date -Format 'yyyy-MM-dd HH:mm:ss')" -ForegroundColor Gray
Write-Host "   Конфигурация: $Config" -ForegroundColor Gray
Write-Host "   Отчет: $outputFile" -ForegroundColor Gray
Write-Host ""

# Запускаем проверку
try {
    .\http_check.ps1 -Config $Config -Output $outputFile -Log $logFile
    
    if ($LASTEXITCODE -eq 0) {
        Write-Host "✅ Проверка завершена успешно" -ForegroundColor Green
        
        # Читаем отчет для анализа
        if (Test-Path $outputFile) {
            $report = Get-Content $outputFile | ConvertFrom-Json
            
            Write-Host ""
            Write-Host "📊 Статистика:" -ForegroundColor Cyan
            Write-Host "   Всего проверок: $($report.total_checks)" -ForegroundColor Gray
            Write-Host "   Успешных: $($report.summary.success)" -ForegroundColor Green
            Write-Host "   Ошибок: $($report.summary.total_errors)" -ForegroundColor $(if ($report.summary.total_errors -gt 0) { "Red" } else { "Gray" })
            
            # Если есть ошибки, можно добавить дополнительную логику
            if ($report.summary.total_errors -gt 0) {
                Write-Host ""
                Write-Host "⚠️  Обнаружены проблемы:" -ForegroundColor Yellow
                
                # Сохраняем краткий отчет об ошибках
                $errorReport = @{
                    timestamp = Get-Date -Format "yyyy-MM-dd HH:mm:ss"
                    total_errors = $report.summary.total_errors
                    errors = @()
                }
                
                foreach ($result in $report.results) {
                    if (-not $result.is_valid -or $result.error) {
                        $errorReport.errors += @{
                            url = $result.url
                            status = $result.status
                            error = $result.error
                            category = $result.category
                        }
                    }
                }
                
                $errorReportFile = Join-Path $OutputDir "errors_$timestamp.json"
                $errorReport | ConvertTo-Json -Depth 10 | Out-File $errorReportFile -Encoding UTF8
                Write-Host "   Отчет об ошибках сохранен: $errorReportFile" -ForegroundColor Gray
            }
        }
    } else {
        Write-Host "❌ Проверка завершилась с ошибками" -ForegroundColor Red
        exit $LASTEXITCODE
    }
} catch {
    Write-Host "❌ Критическая ошибка: $_" -ForegroundColor Red
    exit 1
}

# Очистка старых отчетов (оставляем последние 30 дней)
Write-Host ""
Write-Host "🧹 Очистка старых отчетов..." -ForegroundColor Cyan
$cutoffDate = (Get-Date).AddDays(-30)
Get-ChildItem -Path $OutputDir -Filter "http_check_*.json" | 
    Where-Object { $_.LastWriteTime -lt $cutoffDate } | 
    Remove-Item -Force
Write-Host "✅ Очистка завершена" -ForegroundColor Green

