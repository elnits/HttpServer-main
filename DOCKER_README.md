# Docker контейнеризация проекта

Проект полностью контейнеризирован с использованием Docker и docker-compose.

## Структура

- **Dockerfile** - образ для Go бэкенда
- **frontend/Dockerfile** - образ для Next.js фронтенда
- **docker-compose.yml** - оркестрация всех сервисов
- **.dockerignore** - исключения для сборки образов

## Быстрый старт

### 1. Сборка и запуск всех сервисов

```bash
docker-compose up -d --build
```

### 2. Просмотр логов

```bash
# Все сервисы
docker-compose logs -f

# Только бэкенд
docker-compose logs -f backend

# Только фронтенд
docker-compose logs -f frontend
```

### 3. Остановка

```bash
docker-compose down
```

### 4. Перезапуск

```bash
docker-compose restart
```

## Доступ к сервисам

- **Backend API**: http://localhost:9999
- **Frontend**: http://localhost:3000
- **Health Check**: http://localhost:9999/health

## Переменные окружения

Основные переменные окружения настраиваются в `docker-compose.yml`:

```yaml
environment:
  - SERVER_PORT=9999
  - DATABASE_PATH=/app/data/1c_data.db
  - NORMALIZED_DATABASE_PATH=/app/data/normalized_data.db
  - SERVICE_DATABASE_PATH=/app/data/service.db
  - ARLIAI_API_KEY=${ARLIAI_API_KEY:-}
  - ARLIAI_MODEL=${ARLIAI_MODEL:-GLM-4.5-Air}
```

Для переопределения создайте файл `.docker-compose.override.yml` (см. пример в `.docker-compose.override.yml.example`).

## Volumes (персистентное хранение)

Базы данных сохраняются в директории `./data` на хосте:

```
./data/
  ├── 1c_data.db
  ├── normalized_data.db
  └── service.db
```

Файлы для генерации XML монтируются как read-only:
- `./1c_processing` → `/app/1c_processing`
- `./1c_module_extensions.bsl` → `/app/1c_module_extensions.bsl`
- `./1c_export_functions.txt` → `/app/1c_export_functions.txt`

## Режим работы

### Без GUI (по умолчанию в контейнере)

Бэкенд запускается без GUI интерфейса. Для включения GUI установите:

```yaml
environment:
  - USE_GUI=true
```

**Примечание**: GUI требует X11 сервер и не будет работать в обычном Docker контейнере.

## Разработка

### Пересборка после изменений

```bash
# Пересборка всех сервисов
docker-compose build

# Пересборка только бэкенда
docker-compose build backend

# Пересборка только фронтенда
docker-compose build frontend
```

### Hot reload для разработки

Для разработки с hot reload используйте локальный запуск:

```bash
# Бэкенд
go run main.go

# Фронтенд
cd frontend && npm run dev
```

## Health Checks

Оба сервиса имеют health checks:

- **Backend**: проверяет `/health` endpoint каждые 30 секунд
- **Frontend**: проверяется доступность порта 3000

## Troubleshooting

### Проблемы с портами

Если порты заняты, измените их в `docker-compose.yml`:

```yaml
ports:
  - "9998:9999"  # Backend на порту 9998
  - "3001:3000"  # Frontend на порту 3001
```

### Проблемы с базами данных

Если базы данных не создаются:

1. Проверьте права на директорию `./data`:
   ```bash
   mkdir -p data
   chmod 755 data
   ```

2. Проверьте логи:
   ```bash
   docker-compose logs backend
   ```

### Очистка

Полная очистка (включая volumes):

```bash
docker-compose down -v
```

Очистка образов:

```bash
docker-compose down --rmi all
```

## Production

Для production рекомендуется:

1. Использовать `.env` файл для секретов
2. Настроить reverse proxy (nginx/traefik)
3. Использовать Docker secrets для API ключей
4. Настроить мониторинг и логирование

Пример `.env` файла:

```env
ARLIAI_API_KEY=your-secret-key
ARLIAI_MODEL=GLM-4.5-Air
SERVER_PORT=9999
```

Использование в `docker-compose.yml`:

```yaml
env_file:
  - .env
```

