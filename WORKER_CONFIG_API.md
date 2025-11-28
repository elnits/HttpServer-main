# API для управления воркерами и моделями AI

## Обзор

API позволяет настраивать работу воркеров и выбирать приоритетные модели для вычислений от различных провайдеров AI через HTTP интерфейс.

## Эндпоинты

### 1. Получить текущую конфигурацию

**GET** `/api/workers/config`

Возвращает текущую конфигурацию всех провайдеров и моделей.

**Ответ:**
```json
{
  "providers": {
    "arliai": {
      "name": "arliai",
      "base_url": "https://api.arliai.com/v1/chat/completions",
      "enabled": true,
      "priority": 1,
      "max_workers": 2,
      "rate_limit": 120,
      "timeout": "60s",
      "models": [
        {
          "name": "GLM-4.5-Air",
          "provider": "arliai",
          "enabled": true,
          "priority": 1,
          "max_tokens": 4096,
          "temperature": 0.3,
          "speed": "fast",
          "quality": "high"
        }
      ]
    }
  },
  "default_provider": "arliai",
  "default_model": "GLM-4.5-Air",
  "global_max_workers": 2
}
```

### 2. Обновить конфигурацию

**POST/PUT** `/api/workers/config/update`

Обновляет конфигурацию провайдера, модели или глобальные настройки.

**Тело запроса:**
```json
{
  "action": "update_provider|update_model|set_default_provider|set_default_model|set_max_workers",
  "data": {
    // Данные в зависимости от action
  }
}
```

#### Действия:

##### update_provider
Обновляет конфигурацию провайдера.

```json
{
  "action": "update_provider",
  "data": {
    "name": "arliai",
    "enabled": true,
    "priority": 1,
    "max_workers": 4,
    "rate_limit": 200,
    "timeout": "60s",
    "api_key": "your-api-key" // опционально, если не указан, сохраняется старый. Можно обновить через веб-интерфейс на странице /workers
  }
}
```

##### update_model
Обновляет конфигурацию модели.

```json
{
  "action": "update_model",
  "data": {
    "provider": "arliai",
    "name": "GLM-4.5-Air",
    "enabled": true,
    "priority": 1,
    "max_tokens": 4096,
    "temperature": 0.3,
    "speed": "fast",
    "quality": "high"
  }
}
```

##### set_default_provider
Устанавливает провайдера по умолчанию.

```json
{
  "action": "set_default_provider",
  "data": {
    "provider": "arliai"
  }
}
```

##### set_default_model
Устанавливает модель по умолчанию.

```json
{
  "action": "set_default_model",
  "data": {
    "provider": "arliai",
    "model": "GLM-4.5-Air"
  }
}
```

##### set_max_workers
Устанавливает глобальный максимум воркеров.

```json
{
  "action": "set_max_workers",
  "data": {
    "max_workers": 4
  }
}
```

**Ответ:**
```json
{
  "message": "Configuration updated successfully"
}
```

### 3. Проверить статус подключения Arliai

**GET** `/api/workers/arliai/status`

Проверяет статус подключения к Arliai API и валидность API ключа.

**Ответ:**
```json
{
  "connected": true,
  "provider": "arliai",
  "has_api_key": true,
  "model": "GLM-4.5-Air",
  "enabled": true,
  "error": null
}
```

**Возможные значения:**
- `connected`: `true` если подключение установлено, `false` в противном случае
- `has_api_key`: `true` если API ключ установлен (в конфигурации или переменных окружения)
- `model`: название активной модели
- `enabled`: включен ли провайдер
- `error`: описание ошибки (если есть)

**Пример использования:**
```bash
curl http://localhost:9999/api/workers/arliai/status
```

### 4. Получить список доступных провайдеров

**GET** `/api/workers/providers`

Возвращает список всех провайдеров с их моделями.

**Ответ:**
```json
{
  "providers": [
    {
      "name": "arliai",
      "enabled": true,
      "priority": 1,
      "max_workers": 2,
      "rate_limit": 120,
      "models": [
        {
          "name": "GLM-4.5-Air",
          "enabled": true,
          "priority": 1,
          "speed": "fast",
          "quality": "high"
        }
      ]
    }
  ],
  "default_provider": "arliai",
  "default_model": "GLM-4.5-Air"
}
```

## Примеры использования

### Пример 1: Увеличить количество воркеров

```bash
curl -X POST http://localhost:9999/api/workers/config/update \
  -H "Content-Type: application/json" \
  -d '{
    "action": "set_max_workers",
    "data": {
      "max_workers": 4
    }
  }'
```

### Пример 2: Изменить приоритет модели

```bash
curl -X POST http://localhost:9999/api/workers/config/update \
  -H "Content-Type: application/json" \
  -d '{
    "action": "update_model",
    "data": {
      "provider": "arliai",
      "name": "GLM-4.5-Air",
      "priority": 1,
      "enabled": true
    }
  }'
```

### Пример 3: Добавить новый провайдер

```bash
curl -X POST http://localhost:9999/api/workers/config/update \
  -H "Content-Type: application/json" \
  -d '{
    "action": "update_provider",
    "data": {
      "name": "openai",
      "base_url": "https://api.openai.com/v1/chat/completions",
      "enabled": true,
      "priority": 2,
      "max_workers": 3,
      "rate_limit": 60,
      "api_key": "sk-...",
      "models": [
        {
          "name": "gpt-4o-mini",
          "enabled": true,
          "priority": 1
        }
      ]
    }
  }'
```

### Пример 4: Получить текущую конфигурацию

```bash
curl http://localhost:9999/api/workers/config
```

### Пример 5: Проверить статус подключения Arliai

```bash
curl http://localhost:9999/api/workers/arliai/status
```

### Пример 6: Обновить API ключ провайдера

```bash
curl -X POST http://localhost:9999/api/workers/config/update \
  -H "Content-Type: application/json" \
  -d '{
    "action": "update_provider",
    "data": {
      "name": "arliai",
      "api_key": "your-new-api-key"
    }
  }'
```

## Приоритеты

- **Приоритет провайдера**: Меньшее значение = выше приоритет. Система выбирает провайдера с наименьшим приоритетом среди включенных.
- **Приоритет модели**: Меньшее значение = выше приоритет. Система выбирает модель с наименьшим приоритетом среди включенных моделей провайдера.

## Ограничения

- `max_workers` должен быть между 1 и 100
- API ключи не возвращаются в ответах GET запросов (скрыты для безопасности)
- Изменения применяются немедленно, но не сохраняются на диск (требуется реализация персистентности)

## Интеграция

Конфигурация автоматически используется при создании:
- `NomenclatureProcessor` - для обработки номенклатуры
- `AIClient` - для AI запросов
- Воркеров для параллельной обработки

