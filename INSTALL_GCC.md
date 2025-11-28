# Установка GCC для работы с SQLite

Проект использует SQLite через драйвер `go-sqlite3`, который требует CGO и компилятор C (GCC).

## Вариант 1: MinGW-w64 (рекомендуется)

1. **Скачайте MinGW-w64:**
   - Перейдите на: https://www.mingw-w64.org/downloads/
   - Или используйте установщик: https://sourceforge.net/projects/mingw-w64/

2. **Установите:**
   - Распакуйте в `C:\mingw64\mingw64\bin\`
   - Или используйте установщик и выберите путь установки

3. **Добавьте в PATH:**
   - Откройте "Переменные среды" в Windows
   - Добавьте в PATH: `C:\mingw64\mingw64\bin`
   - Или запустите в PowerShell:
     ```powershell
     [Environment]::SetEnvironmentVariable("Path", $env:Path + ";C:\mingw64\mingw64\bin", "User")
     ```

4. **Проверьте установку:**
   ```bash
   gcc --version
   ```

## Вариант 2: TDM-GCC

1. Скачайте: https://jmeubank.github.io/tdm-gcc/
2. Установите с настройками по умолчанию
3. Перезапустите терминал

## Вариант 3: Использование Docker (без установки GCC)

Если не хотите устанавливать GCC, используйте Docker:

```bash
docker-compose up -d
```

Это запустит и бэкенд, и фронтенд в контейнерах, где GCC уже установлен.

## Проверка после установки

После установки GCC перезапустите терминал и запустите:

```bash
start-backend.bat
```

Сервер должен запуститься без ошибок.

