@echo off
chcp 65001 >nul 2>&1
echo Starting backend on port 9999...
echo.

REM Change to script directory
cd /d "%~dp0"

REM Check for Go
where go >nul 2>&1
if %ERRORLEVEL% NEQ 0 (
    echo Error: Go not found in PATH
    echo Please install Go and add it to PATH
    pause
    exit /b 1
)

REM Check for GCC (required for CGO/SQLite)
where gcc >nul 2>&1
if %ERRORLEVEL% NEQ 0 (
    echo.
    echo [WARNING] GCC compiler not found in PATH
    echo SQLite requires CGO which needs a C compiler (GCC)
    echo.
    echo Please install MinGW-w64:
    echo 1. Download from: https://www.mingw-w64.org/downloads/
    echo 2. Install to C:\mingw64\mingw64\bin\
    echo 3. Add to PATH: C:\mingw64\mingw64\bin
    echo.
    echo Or use Docker: docker-compose up -d
    echo.
    echo Attempting to continue anyway...
    echo.
)

REM Set API key for ArliAI
set ARLIAI_API_KEY=597dbe7e-16ca-4803-ab17-5fa084909f37

REM Enable CGO for SQLite (required)
set CGO_ENABLED=1

REM Check if GCC is available (required for CGO)
where gcc >nul 2>&1
if %ERRORLEVEL% NEQ 0 (
    echo.
    echo ========================================
    echo ERROR: GCC compiler not found!
    echo ========================================
    echo.
    echo SQLite requires CGO which needs GCC compiler.
    echo.
    echo Please install MinGW-w64:
    echo 1. Download: https://sourceforge.net/projects/mingw-w64/
    echo 2. Install to: C:\mingw64\mingw64\bin\
    echo 3. Add to PATH: C:\mingw64\mingw64\bin
    echo.
    echo Or use Docker instead:
    echo   docker-compose up -d
    echo.
    echo ========================================
    pause
    exit /b 1
)

REM Start backend
echo Starting server with AI normalization...
echo CGO_ENABLED=%CGO_ENABLED%
go run -tags no_gui main_no_gui.go

pause
