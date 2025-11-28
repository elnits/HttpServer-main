# Инструкция по перезапуску сервера

## Текущая ситуация

Сервер работает **локально** (не в Docker), но новые эндпоинты `/api/workers/config` не доступны, так как сервер был запущен до их добавления.

## Решение: Перезапустить локальный сервер

### Вариант 1: Если сервер запущен через `go run`

1. Найдите окно терминала, где запущен сервер
2. Нажмите `Ctrl+C` для остановки
3. Запустите снова:
```powershell
go run .
```

### Вариант 2: Если сервер запущен как .exe

1. Найдите процесс в диспетчере задач:
   - `httpserver.exe`
   - `server.exe`
   - `http_server.exe`
2. Завершите процесс
3. Запустите снова:
```powershell
.\httpserver.exe
```
или
```powershell
go run .
```

### Вариант 3: Пересборка и запуск

```powershell
# Пересоберите бинарник
go build -o httpserver.exe .

# Запустите
.\httpserver.exe
```

## Проверка после перезапуска

```powershell
# Health check
curl http://localhost:9999/health

# Проверка нового эндпоинта
curl http://localhost:9999/api/workers/config
```

Если эндпоинт возвращает JSON с конфигурацией - всё работает! ✅

## Если нужно использовать Docker

1. Запустите **Docker Desktop**
2. Выполните:
```powershell
.\rebuild-containers.ps1
```

Или вручную:
```powershell
docker-compose down
docker-compose build --no-cache
docker-compose up -d
```

