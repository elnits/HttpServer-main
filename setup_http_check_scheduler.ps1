# Скрипт для настройки Task Scheduler для автоматических HTTP проверок
# Требует прав администратора

param(
    [string]$TaskName = "HTTP Status Checker",
    [int]$IntervalMinutes = 15,
    [string]$ScriptPath = "http_check_scheduled.ps1"
)

$ErrorActionPreference = "Stop"

# Проверяем права администратора
$isAdmin = ([Security.Principal.WindowsPrincipal] [Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
if (-not $isAdmin) {
    Write-Host "❌ Требуются права администратора" -ForegroundColor Red
    Write-Host "   Запустите PowerShell от имени администратора" -ForegroundColor Yellow
    exit 1
}

# Получаем полный путь к скрипту
$fullScriptPath = Resolve-Path $ScriptPath -ErrorAction Stop

Write-Host "⚙️  Настройка Task Scheduler для HTTP проверок" -ForegroundColor Cyan
Write-Host "   Имя задачи: $TaskName" -ForegroundColor Gray
Write-Host "   Интервал: каждые $IntervalMinutes минут" -ForegroundColor Gray
Write-Host "   Скрипт: $fullScriptPath" -ForegroundColor Gray
Write-Host ""

# Удаляем существующую задачу, если есть
$existingTask = Get-ScheduledTask -TaskName $TaskName -ErrorAction SilentlyContinue
if ($existingTask) {
    Write-Host "🗑️  Удаление существующей задачи..." -ForegroundColor Yellow
    Unregister-ScheduledTask -TaskName $TaskName -Confirm:$false
}

# Создаем действие
$action = New-ScheduledTaskAction -Execute "PowerShell.exe" `
    -Argument "-NoProfile -ExecutionPolicy Bypass -File `"$fullScriptPath`""

# Создаем триггер (каждые N минут)
$trigger = New-ScheduledTaskTrigger -RepetitionInterval (New-TimeSpan -Minutes $IntervalMinutes) `
    -RepetitionDuration (New-TimeSpan -Days 365) `
    -Once -At (Get-Date)

# Настройки задачи
$settings = New-ScheduledTaskSettingsSet `
    -AllowStartIfOnBatteries `
    -DontStopIfGoingOnBatteries `
    -StartWhenAvailable `
    -RunOnlyIfNetworkAvailable

# Регистрируем задачу
Write-Host "📝 Регистрация задачи..." -ForegroundColor Yellow
Register-ScheduledTask -TaskName $TaskName `
    -Action $action `
    -Trigger $trigger `
    -Settings $settings `
    -Description "Автоматическая проверка HTTP статусов веб-страниц и API эндпоинтов" `
    -User "$env:USERDOMAIN\$env:USERNAME" | Out-Null

Write-Host "✅ Задача успешно создана!" -ForegroundColor Green
Write-Host ""
Write-Host "📋 Информация о задаче:" -ForegroundColor Cyan
Get-ScheduledTask -TaskName $TaskName | Format-List TaskName, State, Description

Write-Host ""
Write-Host "💡 Полезные команды:" -ForegroundColor Cyan
Write-Host "   Просмотр задачи: Get-ScheduledTask -TaskName '$TaskName'" -ForegroundColor Gray
Write-Host "   Запуск задачи: Start-ScheduledTask -TaskName '$TaskName'" -ForegroundColor Gray
Write-Host "   Остановка задачи: Stop-ScheduledTask -TaskName '$TaskName'" -ForegroundColor Gray
Write-Host "   Удаление задачи: Unregister-ScheduledTask -TaskName '$TaskName' -Confirm:`$false" -ForegroundColor Gray

