# Быстрый старт с Docker

## Запуск проекта

```bash
# Сборка и запуск всех сервисов
docker-compose up -d --build

# Просмотр логов
docker-compose logs -f

# Остановка
docker-compose down
```

## Доступ

- **Frontend**: http://localhost:3000
- **Backend API**: http://localhost:9999
- **Health Check**: http://localhost:9999/health

## Первый запуск

1. Запустите контейнеры:
   ```bash
   docker-compose up -d --build
   ```

2. Дождитесь готовности (проверьте логи):
   ```bash
   docker-compose logs -f
   ```

3. Откройте браузер: http://localhost:3000

## Остановка

```bash
docker-compose down
```

## Перезапуск

```bash
docker-compose restart
```

## Полезные команды

```bash
# Пересборка после изменений
docker-compose build

# Просмотр статуса
docker-compose ps

# Выполнение команды в контейнере
docker-compose exec backend sh
docker-compose exec frontend sh

# Очистка (включая volumes)
docker-compose down -v
```

Подробная документация: см. `DOCKER_README.md`

