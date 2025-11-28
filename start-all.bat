@echo off
chcp 65001 >nul 2>&1
echo ========================================
echo Starting normalization system
echo ========================================
echo.

REM Check for Go
where go >nul 2>&1
if %ERRORLEVEL% NEQ 0 (
    echo [ERROR] Go not found in PATH
    echo Please install Go and add it to PATH
    pause
    exit /b 1
)

REM Check for Node.js
where npm >nul 2>&1
if %ERRORLEVEL% NEQ 0 (
    echo [ERROR] Node.js not found in PATH
    echo Please install Node.js and add it to PATH
    pause
    exit /b 1
)

REM Change to script directory
cd /d "%~dp0"

echo [1/2] Starting backend on port 9999...
start "Backend Server" cmd /k "cd /d %~dp0 && set CGO_ENABLED=1 && go run -tags no_gui main_no_gui.go"
timeout /t 3 /nobreak >nul 2>&1

echo [2/2] Starting frontend on port 3001...
start "Frontend Server" cmd /k "cd /d %~dp0\frontend && npm run dev"

echo.
echo ========================================
echo Servers started!
echo.
echo Backend:  http://localhost:9999
echo Frontend: http://localhost:3001
echo.
echo To stop servers, close the windows
echo ========================================
echo.
pause
