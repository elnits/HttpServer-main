# Инструкция по пересборке контейнера

## Вариант 1: С Docker Desktop

1. **Запустите Docker Desktop** (если не запущен)

2. **Остановите текущие контейнеры:**
```bash
docker-compose down
```

3. **Пересоберите контейнеры:**
```bash
docker-compose build --no-cache
```

4. **Запустите контейнеры:**
```bash
docker-compose up -d
```

5. **Проверьте статус:**
```bash
docker-compose ps
```

6. **Проверьте логи:**
```bash
docker-compose logs -f backend
```

## Вариант 2: Без Docker (локальный запуск)

Если Docker не запущен, можно запустить сервер локально:

### Backend:
```bash
# Установите переменную окружения (если нужно)
$env:ARLIAI_API_KEY="your-api-key"

# Запустите сервер
go run .
```

### Frontend:
```bash
cd frontend
npm install
npm run dev
```

## Проверка работы

После пересборки проверьте:

1. **Backend health check:**
```bash
curl http://localhost:9999/health
```

2. **Workers config endpoint:**
```bash
curl http://localhost:9999/api/workers/config
```

3. **Frontend:**
Откройте в браузере: http://localhost:3000

## Важные изменения в новой версии

- ✅ Добавлены эндпоинты `/api/workers/config`
- ✅ Добавлена страница управления воркерами `/workers`
- ✅ Улучшена валидация и обработка ошибок
- ✅ Добавлены информационные блоки

**Важно:** После пересборки новые эндпоинты будут доступны!

