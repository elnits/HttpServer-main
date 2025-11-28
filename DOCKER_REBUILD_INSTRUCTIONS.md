# Инструкция по пересборке Docker контейнера

## Быстрый способ

1. **Запустите Docker Desktop** (если не запущен)

2. **Выполните скрипт:**
   ```powershell
   .\rebuild-docker.ps1
   ```

   Или:
   ```powershell
   .\rebuild-containers.ps1
   ```

## Ручной способ

Если скрипт не работает, выполните команды вручную:

```powershell
# 1. Остановите текущие контейнеры
docker-compose down

# 2. Пересоберите контейнеры (без кэша)
docker-compose build --no-cache

# 3. Запустите контейнеры
docker-compose up -d

# 4. Проверьте статус
docker-compose ps

# 5. Просмотрите логи
docker-compose logs -f backend
```

## Что включено в новую версию

### ✅ Система извлечения атрибутов
- Автоматическое извлечение размеров (100x100)
- Извлечение материалов, цветов, покрытий
- Извлечение единиц измерения (толщина, вес, мощность и т.д.)
- Извлечение артикулов и технических кодов

### ✅ Новые API endpoints
- `GET /api/normalization/item-attributes/{id}` - получение атрибутов товара
- Атрибуты автоматически включаются в ответ `/api/normalization/group-items`

### ✅ Обновления фронтенда
- Отображение атрибутов в таблице элементов
- Красивые карточки с информацией об атрибутах
- Показ уверенности и исходного текста

## Проверка после пересборки

### 1. Проверка backend
```powershell
# Health check
curl http://localhost:9999/health

# Статистика нормализации
curl http://localhost:9999/api/normalization/stats

# Получение атрибутов (если есть нормализованные данные)
curl http://localhost:9999/api/normalization/item-attributes/1
```

### 2. Проверка frontend
Откройте в браузере:
- http://localhost:3000 - главная страница
- http://localhost:3000/results - результаты нормализации

### 3. Проверка извлечения атрибутов
1. Запустите нормализацию через веб-интерфейс
2. Откройте страницу результатов
3. Выберите любую группу
4. Раскройте элементы для просмотра атрибутов

## Устранение проблем

### Docker Desktop не запущен
```
error during connect: Get "http://%2F%2F.%2Fpipe%2FdockerDesktopLinuxEngine/...
```
**Решение:** Запустите Docker Desktop и дождитесь полного запуска

### Ошибка при сборке
```powershell
# Очистите кэш Docker
docker system prune -a

# Попробуйте снова
docker-compose build --no-cache
```

### Контейнер не запускается
```powershell
# Проверьте логи
docker-compose logs backend

# Проверьте конфигурацию
docker-compose config
```

### Порты заняты
```powershell
# Проверьте, что порты свободны
netstat -ano | findstr :9999
netstat -ano | findstr :3000

# Остановите процессы, использующие порты
# Или измените порты в docker-compose.yml
```

## Полезные команды

```powershell
# Просмотр логов в реальном времени
docker-compose logs -f

# Остановка контейнеров
docker-compose down

# Перезапуск контейнеров
docker-compose restart

# Просмотр статуса
docker-compose ps

# Вход в контейнер
docker exec -it httpserver-backend sh

# Очистка всех данных (ОСТОРОЖНО!)
docker-compose down -v
```

## Структура контейнеров

### Backend контейнер
- **Порт:** 9999
- **Образ:** golang:1.25-alpine (builder) → alpine:latest (runtime)
- **Команда:** `./httpserver`
- **Volumes:**
  - `./data:/app/data` - базы данных
  - `./1c_processing:/app/1c_processing:ro` - файлы для XML

### Frontend контейнер
- **Порт:** 3000
- **Образ:** node:18-alpine
- **Команда:** `npm start`
- **Зависит от:** backend

## Переменные окружения

Можно настроить через `.env` файл или в `docker-compose.yml`:

```env
ARLIAI_API_KEY=your-api-key
ARLIAI_MODEL=GLM-4.5-Air
SERVER_PORT=9999
DATABASE_PATH=/app/data/1c_data.db
NORMALIZED_DATABASE_PATH=/app/data/normalized_data.db
SERVICE_DATABASE_PATH=/app/data/service.db
```

## Обновление без пересборки

Если нужно только перезапустить контейнеры:

```powershell
docker-compose restart
```

Если нужно обновить код без пересборки (для разработки):

```powershell
# Остановите контейнеры
docker-compose down

# Запустите локально
go run .
cd frontend
npm run dev
```

