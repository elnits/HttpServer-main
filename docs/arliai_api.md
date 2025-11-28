# Arliai API Integration

## Обзор

Система интеграции с Arliai API предоставляет REST API для управления статусом подключения и получения списка доступных моделей AI для нормализации данных.

## API Endpoints

### GET /api/workers/arliai/status

Проверяет статус подключения к Arliai API.

**Заголовки:**
- `X-Request-ID` (опционально): Уникальный идентификатор запроса для трейсинга

**Ответ:**
```json
{
  "success": true,
  "data": {
    "connected": true,
    "status": "ok",
    "model": "GLM-4.5-Air",
    "version": "1.0",
    "provider": "arliai",
    "has_api_key": true,
    "enabled": true,
    "api_available": true,
    "last_check": "2025-11-16T21:47:49Z",
    "response_time_ms": 25
  },
  "timestamp": "2025-11-16T21:47:49Z",
  "duration_ms": 25000000,
  "metadata": {
    "cached": false
  }
}
```

**Особенности:**
- Результаты кешируются на 60 секунд
- При недоступности API возвращается локальный статус
- Поддерживает retry с экспоненциальным бэкоффом

### GET /api/workers/models

Возвращает список доступных моделей AI.

**Query параметры:**
- `status` (опционально): Фильтр по статусу (`active`, `deprecated`, `beta`, `all`)
- `enabled` (опционально): Фильтр по включенным моделям (`true`, `false`, `all`)
- `search` (опционально): Поиск по имени модели

**Заголовки:**
- `X-Request-ID` (опционально): Уникальный идентификатор запроса

**Ответ:**
```json
{
  "success": true,
  "data": {
    "models": [
      {
        "id": "GLM-4.5-Air",
        "name": "GLM-4.5-Air",
        "provider": "arliai",
        "enabled": true,
        "priority": 1,
        "max_tokens": 8192,
        "temperature": 0.7,
        "speed": "fast",
        "quality": "high",
        "status": "active",
        "is_default": true,
        "description": "Fast and efficient model"
      }
    ],
    "provider": "arliai",
    "default_model": "GLM-4.5-Air",
    "total": 5,
    "total_before_filter": 8,
    "api_available": true,
    "filters": {
      "status": "",
      "enabled": "",
      "search": ""
    }
  },
  "timestamp": "2025-11-16T21:47:49Z",
  "duration_ms": 45000000,
  "metadata": {
    "cached": false
  }
}
```

**Особенности:**
- Результаты кешируются на 5 минут
- Модели сортируются по приоритету, затем по имени
- Объединяет данные из API и локальной конфигурации
- Поддерживает фильтрацию и поиск

## Конфигурация

### Переменные окружения

- `ARLIAI_BASE_URL`: Базовый URL API Arliai (по умолчанию: `https://api.arliai.com/v1`)
- `ARLIAI_API_KEY`: API ключ для аутентификации

### Retry конфигурация

По умолчанию:
- Максимум попыток: 3
- Начальная задержка: 500ms
- Максимальная задержка: 10s
- Множитель бэкоффа: 2.0

## Обработка ошибок

Все ошибки возвращаются в стандартизированном формате:

```json
{
  "success": false,
  "error": {
    "code": "ERROR_CODE",
    "message": "Human-readable error message",
    "trace_id": "1234567890-1234567890",
    "timestamp": "2025-11-16T21:47:49Z"
  },
  "timestamp": "2025-11-16T21:47:49Z"
}
```

### Коды ошибок

- `METHOD_NOT_ALLOWED`: Неверный HTTP метод
- `SERVICE_UNAVAILABLE`: Сервис недоступен
- `NO_ACTIVE_PROVIDER`: Нет активного провайдера
- `INVALID_MODEL_NAME`: Неверное имя модели

## Интеграция с нормализацией

При запуске нормализации можно указать модель через параметр `model`:

```json
{
  "use_ai": true,
  "min_confidence": 0.8,
  "rate_limit_delay_ms": 100,
  "max_retries": 3,
  "model": "GLM-4.5-Air"
}
```

Модель будет установлена как активная через `WorkerConfigManager` перед запуском нормализации.

## Безопасность

- Все запросы логируются с trace-id
- Security headers добавляются автоматически
- CORS настраивается через middleware
- Валидация входных данных
- Санитизация имен моделей

## Мониторинг

Все запросы логируются с:
- Trace ID
- Временем выполнения
- Статусом ответа
- Информацией о кеше

Пример лога:
```
[1234567890-1234567890] GET /api/workers/models
[1234567890-1234567890] Models fetch completed (duration: 45ms, count: 5)
```

