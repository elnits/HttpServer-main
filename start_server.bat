@echo off
chcp 65001 >nul
echo ========================================
echo Запуск 1C HTTP Server
echo ========================================
echo.

REM Проверка наличия Go
where go >nul 2>&1
if %ERRORLEVEL% NEQ 0 (
    echo [ОШИБКА] Go не найден в PATH!
    echo Установите Go или добавьте его в PATH
    pause
    exit /b 1
)

REM Проверка наличия GCC для CGO (SQLite требует CGO)
where gcc >nul 2>&1
if %ERRORLEVEL% NEQ 0 (
    REM Попробуем найти GCC в стандартных местах
    if exist "C:\mingw64\mingw64\bin\gcc.exe" (
        echo [ИНФОРМАЦИЯ] Найден GCC в C:\mingw64\mingw64\bin
        set PATH=%PATH%;C:\mingw64\mingw64\bin
        set USE_GUI=1
    ) else if exist "C:\msys64\mingw64\bin\gcc.exe" (
        echo [ИНФОРМАЦИЯ] Найден GCC в C:\msys64\mingw64\bin
        set PATH=%PATH%;C:\msys64\mingw64\bin
        set USE_GUI=1
    ) else (
        echo [ОШИБКА] GCC не найден!
        echo SQLite требует CGO компилятор.
        echo Установите MinGW-w64 или добавьте GCC в PATH
        echo.
        echo Попробуем компилировать без GUI (может не работать без CGO)...
        set USE_GUI=0
    )
) else (
    echo [ИНФОРМАЦИЯ] GCC найден в PATH
    set USE_GUI=1
)

REM Включаем CGO для SQLite
set CGO_ENABLED=1

REM Компиляция и запуск
if "%USE_GUI%"=="1" (
    echo Компиляция сервера с GUI...
    go build -o httpserver.exe .
    if %ERRORLEVEL% NEQ 0 (
        echo [ПРЕДУПРЕЖДЕНИЕ] Ошибка компиляции с GUI, пробуем версию без GUI...
        set USE_GUI=0
    )
)

if "%USE_GUI%"=="0" (
    echo Компиляция сервера без GUI...
    echo [ВНИМАНИЕ] SQLite требует CGO. Если компиляция не удастся, установите MinGW-w64
    go build -o httpserver.exe main_no_gui.go
    if %ERRORLEVEL% NEQ 0 (
        echo.
        echo [ОШИБКА] Ошибка компиляции!
        echo.
        echo Возможные решения:
        echo 1. Установите MinGW-w64: https://www.mingw-w64.org/
        echo 2. Добавьте путь к gcc.exe в PATH
        echo 3. Или установите в C:\mingw64\mingw64\bin\
        echo.
        pause
        exit /b 1
    )
    echo [ИНФОРМАЦИЯ] Сервер скомпилирован без GUI
)

echo.
echo Сервер успешно скомпилирован!
echo Запуск сервера на порту 9999...
echo.
echo API доступно по адресу: http://localhost:9999
echo Для остановки нажмите Ctrl+C
echo ========================================
echo.

httpserver.exe

pause

