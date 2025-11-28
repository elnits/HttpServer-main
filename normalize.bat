@echo off
chcp 65001 >nul
echo Запуск нормализации данных...
echo.

REM Проверяем наличие Go
where go >nul 2>&1
if %ERRORLEVEL% NEQ 0 (
    echo Ошибка: Go не найден в PATH
    echo Установите Go и добавьте его в PATH
    pause
    exit /b 1
)

REM Компилируем утилиту нормализации
echo Компиляция утилиты нормализации...
go build -o normalize.exe cmd/normalize/main.go

if %ERRORLEVEL% NEQ 0 (
    echo Ошибка компиляции
    pause
    exit /b 1
)

REM Запускаем нормализацию
echo.
echo Запуск нормализации...
echo.
.\normalize.exe -db 1c_data.db

if %ERRORLEVEL% NEQ 0 (
    echo.
    echo Ошибка при выполнении нормализации
    pause
    exit /b 1
)

echo.
echo Нормализация завершена успешно!
pause

