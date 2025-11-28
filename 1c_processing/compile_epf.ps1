# Скрипт для компиляции обработки 1С в .epf файл
# Требуется установленный 1С:Предприятие с утилитой командной строки

param(
    [string]$PathTo1C = "",
    [string]$OutputPath = ".\ExportProcessing.epf",
    [string]$SourcePath = ".\1c_processing"
)

# Поиск пути к 1С, если не указан
if ([string]::IsNullOrEmpty($PathTo1C)) {
    $possiblePaths = @(
        "${env:ProgramFiles}\1cv8\*\bin\1cv8.exe",
        "${env:ProgramFiles(x86)}\1cv8\*\bin\1cv8.exe",
        "C:\Program Files\1cv8\*\bin\1cv8.exe",
        "C:\Program Files (x86)\1cv8\*\bin\1cv8.exe"
    )
    
    foreach ($path in $possiblePaths) {
        $found = Get-ChildItem -Path $path -ErrorAction SilentlyContinue | Select-Object -First 1
        if ($found) {
            $PathTo1C = $found.FullName
            Write-Host "Найден 1С: $PathTo1C" -ForegroundColor Green
            break
        }
    }
}

if ([string]::IsNullOrEmpty($PathTo1C)) {
    Write-Host "ОШИБКА: Не найден путь к 1cv8.exe" -ForegroundColor Red
    Write-Host "Укажите путь к 1cv8.exe через параметр -PathTo1C" -ForegroundColor Yellow
    Write-Host "Пример: .\compile_epf.ps1 -PathTo1C `"C:\Program Files\1cv8\8.3.25.1234\bin\1cv8.exe`"" -ForegroundColor Yellow
    exit 1
}

if (-not (Test-Path $PathTo1C)) {
    Write-Host "ОШИБКА: Файл 1cv8.exe не найден по пути: $PathTo1C" -ForegroundColor Red
    exit 1
}

# Проверка наличия исходников
if (-not (Test-Path $SourcePath)) {
    Write-Host "ОШИБКА: Папка с исходниками не найдена: $SourcePath" -ForegroundColor Red
    exit 1
}

Write-Host "============================================" -ForegroundColor Cyan
Write-Host "Компиляция обработки 1С в .epf" -ForegroundColor Cyan
Write-Host "============================================" -ForegroundColor Cyan
Write-Host "Путь к 1С: $PathTo1C" -ForegroundColor White
Write-Host "Исходники: $SourcePath" -ForegroundColor White
Write-Host "Выходной файл: $OutputPath" -ForegroundColor White
Write-Host ""

# Создание временного каталога для компиляции
$TempDir = Join-Path $env:TEMP "1c_compile_$(Get-Date -Format 'yyyyMMddHHmmss')"
New-Item -ItemType Directory -Path $TempDir -Force | Out-Null

try {
    Write-Host "Создание структуры обработки..." -ForegroundColor Yellow
    
    # Копирование структуры
    $ModulePath = Join-Path $TempDir "Module"
    $FormsPath = Join-Path $TempDir "Forms"
    New-Item -ItemType Directory -Path $ModulePath -Force | Out-Null
    New-Item -ItemType Directory -Path $FormsPath -Force | Out-Null
    
    # Копирование модуля
    if (Test-Path (Join-Path $SourcePath "Module\Module.bsl")) {
        Copy-Item (Join-Path $SourcePath "Module\Module.bsl") (Join-Path $ModulePath "Module.bsl") -Force
        Write-Host "  ✓ Модуль скопирован" -ForegroundColor Green
    } else {
        Write-Host "  ✗ Модуль не найден" -ForegroundColor Red
    }
    
    # Копирование формы (если есть)
    if (Test-Path (Join-Path $SourcePath "Forms\Form1")) {
        $Form1Path = Join-Path $FormsPath "Form1"
        New-Item -ItemType Directory -Path $Form1Path -Force | Out-Null
        Copy-Item (Join-Path $SourcePath "Forms\Form1\*") $Form1Path -Recurse -Force -ErrorAction SilentlyContinue
        Write-Host "  ✓ Форма скопирована" -ForegroundColor Green
    }
    
    Write-Host ""
    Write-Host "ВАЖНО: Автоматическая компиляция .epf требует:" -ForegroundColor Yellow
    Write-Host "  1. Открыть Конфигуратор 1С" -ForegroundColor Yellow
    Write-Host "  2. Создать новую внешнюю обработку" -ForegroundColor Yellow
    Write-Host "  3. Скопировать код модуля из: $ModulePath\Module.bsl" -ForegroundColor Yellow
    Write-Host "  4. Настроить форму согласно инструкции: ..\1c_form_instructions.md" -ForegroundColor Yellow
    Write-Host "  5. Сохранить обработку как .epf" -ForegroundColor Yellow
    Write-Host ""
    Write-Host "Альтернативный способ (через командную строку):" -ForegroundColor Cyan
    Write-Host "  Используйте утилиту 1cv8c.exe для компиляции:" -ForegroundColor White
    Write-Host "  `"$PathTo1C`" /D `"$TempDir`" /N `"Администратор`" /P `"пароль`" /C `"СоздатьОбработку`"" -ForegroundColor Gray
    Write-Host ""
    Write-Host "Временная папка: $TempDir" -ForegroundColor Cyan
    
} catch {
    Write-Host "ОШИБКА: $($_.Exception.Message)" -ForegroundColor Red
    exit 1
} finally {
    # Не удаляем временную папку, чтобы пользователь мог использовать файлы
    Write-Host ""
    Write-Host "Временные файлы сохранены в: $TempDir" -ForegroundColor Cyan
}




