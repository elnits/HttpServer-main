@echo off
chcp 65001 >nul 2>&1
echo Starting frontend on port 3001...
echo.

REM Change to script directory
cd /d "%~dp0"

REM Check for Node.js
where npm >nul 2>&1
if %ERRORLEVEL% NEQ 0 (
    echo Error: Node.js not found in PATH
    echo Please install Node.js and add it to PATH
    pause
    exit /b 1
)

REM Change to frontend directory
cd frontend

REM Start frontend
echo Starting Next.js...
npm run dev

pause
