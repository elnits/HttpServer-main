@echo off
REM Скрипт для компиляции обработки 1С в .epf файл (Windows Batch)
REM Требуется установленный 1С:Предприятие

setlocal enabledelayedexpansion

set "PATH_TO_1C="
set "OUTPUT_PATH=ExportProcessing.epf"
set "SOURCE_PATH=.\1c_processing"

REM Поиск пути к 1С
if "%PATH_TO_1C%"=="" (
    if exist "C:\Program Files\1cv8" (
        for /d %%i in ("C:\Program Files\1cv8\*") do (
            if exist "%%i\bin\1cv8.exe" (
                set "PATH_TO_1C=%%i\bin\1cv8.exe"
                goto :found
            )
        )
    )
    if exist "C:\Program Files (x86)\1cv8" (
        for /d %%i in ("C:\Program Files (x86)\1cv8\*") do (
            if exist "%%i\bin\1cv8.exe" (
                set "PATH_TO_1C=%%i\bin\1cv8.exe"
                goto :found
            )
        )
    )
)

:found
if "%PATH_TO_1C%"=="" (
    echo ОШИБКА: Не найден путь к 1cv8.exe
    echo Укажите путь к 1cv8.exe через переменную PATH_TO_1C
    echo Пример: set PATH_TO_1C=C:\Program Files\1cv8\8.3.25.1234\bin\1cv8.exe
    exit /b 1
)

echo ============================================
echo Компиляция обработки 1С в .epf
echo ============================================
echo Путь к 1С: %PATH_TO_1C%
echo Исходники: %SOURCE_PATH%
echo Выходной файл: %OUTPUT_PATH%
echo.

echo ВАЖНО: Автоматическая компиляция .epf требует:
echo   1. Открыть Конфигуратор 1С
echo   2. Создать новую внешнюю обработку
echo   3. Скопировать код модуля из: %SOURCE_PATH%\Module\Module.bsl
echo   4. Настроить форму согласно инструкции: ..\1c_form_instructions.md
echo   5. Сохранить обработку как .epf
echo.

pause




