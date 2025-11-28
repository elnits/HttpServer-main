# Скрипт для проверки статуса всех процессов

param(
    [switch]$Watch,
    [int]$Interval = 5
)

$baseUrl = "http://localhost:9999"
$timeout = 7

function Show-ProcessStatus {
    if ($Watch) {
        Clear-Host
    }
    Write-Host "╔══════════════════════════════════════════════════════════════╗" -ForegroundColor Cyan
    Write-Host "║     СТАТУС ВСЕХ ПРОЦЕССОВ                                   ║" -ForegroundColor Cyan
    Write-Host "╚══════════════════════════════════════════════════════════════╝" -ForegroundColor Cyan
    Write-Host "   Время: $(Get-Date -Format 'HH:mm:ss')" -ForegroundColor Gray
    Write-Host ""

    # 1. Проверка нормализации
    Write-Host "📊 НОРМАЛИЗАЦИЯ:" -ForegroundColor Yellow
    Write-Host "─────────────────────────────────────────────────────────────" -ForegroundColor Gray
    try {
        $normStatus = Invoke-RestMethod -Uri "$baseUrl/api/normalization/status" -Method GET -TimeoutSec $timeout
        
        if ($normStatus.isRunning) {
            Write-Host "   Статус: 🟢 ВЫПОЛНЯЕТСЯ" -ForegroundColor Green
            Write-Host "   Обработано: $($normStatus.processed)/$($normStatus.total)" -ForegroundColor Cyan
            
            if ($normStatus.total -gt 0) {
                $progress = [math]::Round(($normStatus.processed / $normStatus.total) * 100, 1)
                Write-Host "   Прогресс: $progress%" -ForegroundColor Cyan
                
                # Расчет оставшегося времени (если есть rate)
                if ($normStatus.processed -gt 0) {
                    if ($normStatus.rate -and $normStatus.rate -gt 0) {
                        $remaining = $normStatus.total - $normStatus.processed
                        $estimatedSeconds = [math]::Round($remaining / $normStatus.rate)
                        $estimatedMinutes = [math]::Round($estimatedSeconds / 60, 1)
                        
                        if ($estimatedSeconds -lt 60) {
                            Write-Host "   ⏱ Осталось: ~$estimatedSeconds сек" -ForegroundColor Yellow
                        } elseif ($estimatedMinutes -lt 60) {
                            Write-Host "   ⏱ Осталось: ~$estimatedMinutes мин" -ForegroundColor Yellow
                        } else {
                            $estimatedHours = [math]::Round($estimatedMinutes / 60, 1)
                            Write-Host "   ⏱ Осталось: ~$estimatedHours час" -ForegroundColor Yellow
                        }
                    } elseif ($normStatus.processed -lt $normStatus.total) {
                        Write-Host "   ⏱ Расчет времени недоступен (нет данных о скорости)" -ForegroundColor DarkGray
                    }
                }
            }
            
            if ($normStatus.success -ne $null) {
                Write-Host "   ✅ Успешно: $($normStatus.success)" -ForegroundColor Green
            }
            if ($normStatus.errors -ne $null) {
                Write-Host "   ❌ Ошибок: $($normStatus.errors)" -ForegroundColor $(if ($normStatus.errors -gt 0) { "Red" } else { "Gray" })
            }
            if ($normStatus.rate -and $normStatus.rate -gt 0) {
                Write-Host "   ⚡ Скорость: $([math]::Round($normStatus.rate, 2)) записей/сек" -ForegroundColor Gray
            }
            
            if ($normStatus.currentStep) {
                Write-Host "   📍 Текущий шаг: $($normStatus.currentStep)" -ForegroundColor Gray
            }
            
            if ($normStatus.logs -and $normStatus.logs.Count -gt 0) {
                Write-Host "   📝 Последний лог: $($normStatus.logs[-1])" -ForegroundColor DarkGray
            }
        } else {
            Write-Host "   Статус: ⚪ НЕ ЗАПУЩЕНА" -ForegroundColor Gray
            if ($normStatus.processed -gt 0) {
                Write-Host "   Последний запуск: обработано $($normStatus.processed) записей" -ForegroundColor Gray
            }
        }
    } catch {
        Write-Host "   ❌ Ошибка получения статуса: $_" -ForegroundColor Red
    }
    Write-Host ""

    # 2. Проверка переклассификации
    Write-Host "🔄 ПЕРЕКЛАССИФИКАЦИЯ:" -ForegroundColor Yellow
    Write-Host "─────────────────────────────────────────────────────────────" -ForegroundColor Gray
    try {
        $reclassStatus = Invoke-RestMethod -Uri "$baseUrl/api/reclassification/status" -Method GET -TimeoutSec $timeout
        
        if ($reclassStatus.isRunning) {
            Write-Host "   Статус: 🟢 ВЫПОЛНЯЕТСЯ" -ForegroundColor Green
            Write-Host "   Обработано: $($reclassStatus.processed)/$($reclassStatus.total)" -ForegroundColor Cyan
            
            if ($reclassStatus.total -gt 0) {
                $progress = [math]::Round(($reclassStatus.processed / $reclassStatus.total) * 100, 1)
                Write-Host "   Прогресс: $progress%" -ForegroundColor Cyan
                
                # Расчет оставшегося времени
                if ($reclassStatus.processed -gt 0 -and $reclassStatus.rate -gt 0) {
                    $remaining = $reclassStatus.total - $reclassStatus.processed
                    $estimatedSeconds = [math]::Round($remaining / $reclassStatus.rate)
                    $estimatedMinutes = [math]::Round($estimatedSeconds / 60, 1)
                    
                    if ($estimatedSeconds -lt 60) {
                        Write-Host "   ⏱ Осталось: ~$estimatedSeconds сек" -ForegroundColor Yellow
                    } elseif ($estimatedMinutes -lt 60) {
                        Write-Host "   ⏱ Осталось: ~$estimatedMinutes мин" -ForegroundColor Yellow
                    } else {
                        $estimatedHours = [math]::Round($estimatedMinutes / 60, 1)
                        Write-Host "   ⏱ Осталось: ~$estimatedHours час" -ForegroundColor Yellow
                    }
                } elseif ($reclassStatus.processed -lt $reclassStatus.total) {
                    Write-Host "   ⏱ Расчет времени недоступен (нет данных о скорости)" -ForegroundColor DarkGray
                }
            }
            
            Write-Host "   ✅ Успешно: $($reclassStatus.success)" -ForegroundColor Green
            Write-Host "   ❌ Ошибок: $($reclassStatus.errors)" -ForegroundColor $(if ($reclassStatus.errors -gt 0) { "Red" } else { "Gray" })
            Write-Host "   ⚡ Скорость: $([math]::Round($reclassStatus.rate, 3)) записей/сек" -ForegroundColor Gray
            
            if ($reclassStatus.currentStep) {
                Write-Host "   📍 Текущий шаг: $($reclassStatus.currentStep)" -ForegroundColor Gray
            }
            
            if ($reclassStatus.logs -and $reclassStatus.logs.Count -gt 0) {
                Write-Host "   📝 Последний лог: $($reclassStatus.logs[-1])" -ForegroundColor DarkGray
            }
            
            if ($reclassStatus.startTime) {
                try {
                    $startTime = [DateTime]::Parse($reclassStatus.startTime)
                    $elapsed = (Get-Date) - $startTime
                    Write-Host "   ⏰ Прошло времени: $([math]::Round($elapsed.TotalMinutes, 1)) мин" -ForegroundColor Gray
                } catch {
                    if ($reclassStatus.elapsedTime) {
                        Write-Host "   ⏰ Прошло времени: $($reclassStatus.elapsedTime)" -ForegroundColor Gray
                    }
                }
            }
        } else {
            Write-Host "   Статус: ⚪ НЕ ЗАПУЩЕНА" -ForegroundColor Gray
            if ($reclassStatus.processed -gt 0) {
                Write-Host "   Последний запуск: обработано $($reclassStatus.processed) записей" -ForegroundColor Gray
            }
        }
    } catch {
        Write-Host "   ❌ Ошибка получения статуса: $_" -ForegroundColor Red
    }
    Write-Host ""

    # 3. Общая информация
    Write-Host "📈 ОБЩАЯ ИНФОРМАЦИЯ:" -ForegroundColor Yellow
    Write-Host "─────────────────────────────────────────────────────────────" -ForegroundColor Gray
    try {
        $health = Invoke-RestMethod -Uri "$baseUrl/health" -Method GET -TimeoutSec $timeout
        Write-Host "   Сервер: ✅ Работает" -ForegroundColor Green
        Write-Host "   Время сервера: $($health.time)" -ForegroundColor Gray
    } catch {
        Write-Host "   Сервер: ❌ Недоступен" -ForegroundColor Red
    }
    Write-Host ""

    # 4. Рекомендации
    $activeProcesses = 0
    try {
        $normStatus = Invoke-RestMethod -Uri "$baseUrl/api/normalization/status" -Method GET -TimeoutSec $timeout
        if ($normStatus.isRunning) { $activeProcesses++ }
    } catch {}

    try {
        $reclassStatus = Invoke-RestMethod -Uri "$baseUrl/api/reclassification/status" -Method GET -TimeoutSec $timeout
        if ($reclassStatus.isRunning) { $activeProcesses++ }
    } catch {}

    if ($activeProcesses -eq 0) {
        Write-Host "💡 Рекомендации:" -ForegroundColor Magenta
        Write-Host "   Нет активных процессов" -ForegroundColor Gray
        Write-Host "   Можно запустить нормализацию или переклассификацию" -ForegroundColor Gray
    } else {
        Write-Host "💡 Рекомендации:" -ForegroundColor Magenta
        Write-Host "   Активных процессов: $activeProcesses" -ForegroundColor Gray
        Write-Host "   Используйте этот скрипт для периодического мониторинга" -ForegroundColor Gray
    }
    Write-Host ""

    Write-Host "╔══════════════════════════════════════════════════════════════╗" -ForegroundColor Green
    Write-Host "║              ПРОВЕРКА ЗАВЕРШЕНА                              ║" -ForegroundColor Green
    Write-Host "╚══════════════════════════════════════════════════════════════╝" -ForegroundColor Green
}

# Запускаем функцию
Show-ProcessStatus

# Автоматическое обновление, если указан флаг -Watch
if ($Watch) {
    while ($true) {
        Start-Sleep -Seconds $Interval
        Show-ProcessStatus
    }
}
