# Скрипт для непрерывного мониторинга процессов с автообновлением

param(
    [int]$Interval = 5
)

Write-Host "╔══════════════════════════════════════════════════════════════╗" -ForegroundColor Cyan
Write-Host "║     МОНИТОРИНГ ПРОЦЕССОВ (АВТООБНОВЛЕНИЕ)                   ║" -ForegroundColor Cyan
Write-Host "╚══════════════════════════════════════════════════════════════╝" -ForegroundColor Cyan
Write-Host "   Интервал обновления: $Interval сек" -ForegroundColor Gray
Write-Host "   Нажмите Ctrl+C для остановки" -ForegroundColor Gray
Write-Host ""

try {
    while ($true) {
        & ".\check_processes_status.ps1"
        Start-Sleep -Seconds $Interval
    }
} catch {
    Write-Host ""
    Write-Host "Мониторинг остановлен" -ForegroundColor Yellow
}

