# Полная документация API

## Содержание

1. [Введение](#введение)
2. [Базовые концепции](#базовые-концепции)
3. [Аутентификация и безопасность](#аутентификация-и-безопасность)
4. [Версионирование](#версионирование)
5. [Форматы данных](#форматы-данных)
6. [Обработка ошибок](#обработка-ошибок)
7. [Rate Limiting](#rate-limiting)
8. [Пагинация](#пагинация)
9. [Фильтрация и сортировка](#фильтрация-и-сортировка)
10. [Эндпоинты](#эндпоинты)
11. [Примеры использования](#примеры-использования)
12. [Интеграция с инструментами](#интеграция-с-инструментами)
13. [Changelog](#changelog)

---

## Введение

### О сервисе

HTTP Server API - это RESTful веб-сервис для работы с данными из 1С:Предприятие. Сервис предоставляет возможности для:

- Приема и хранения данных из 1С
- Нормализации и обработки данных
- Классификации товаров с использованием AI
- Анализа качества данных
- Управления AI моделями и воркерами
- Работы с КПВЭД классификатором

### Базовый URL

```
http://localhost:9999  (разработка)
https://api.example.com  (продакшн)
```

### Версия API

Текущая версия: **1.0.0**

---

## Базовые концепции

### Две базы данных

Система использует две независимые SQLite базы данных:

1. **Основная БД** - для исходных данных из 1С
2. **Нормализованная БД** - для обработанных данных

### Выгрузки (Uploads)

Выгрузка - это набор данных, полученных из 1С за один сеанс. Каждая выгрузка имеет:
- Уникальный UUID
- Статус (in_progress, completed)
- Метаданные (версия 1С, конфигурация)
- Статистику (количество констант, справочников, элементов)

### Процесс выгрузки

1. **Handshake** - создание выгрузки
2. **Metadata** - отправка метаданных
3. **Constants** - добавление констант
4. **Catalog Meta** - регистрация справочников
5. **Catalog Items** - добавление элементов
6. **Complete** - завершение выгрузки

---

## Аутентификация и безопасность

### Текущая реализация

В текущей версии API аутентификация не требуется для большинства эндпоинтов. Однако:

- API ключи для AI провайдеров управляются через `/api/workers/config`
- API ключи скрыты в ответах GET запросов
- CORS поддерживается для всех источников

### Рекомендации для продакшна

1. Использовать HTTPS
2. Реализовать аутентификацию через API ключи или OAuth2
3. Ограничить CORS только разрешенными доменами
4. Использовать rate limiting для защиты от злоупотреблений

### Заголовки безопасности

```
X-API-Key: your-api-key  (если требуется)
Content-Type: application/json
Accept: application/json
```

---

## Версионирование

### API v1

Эндпоинты с префиксом `/api/v1/`:
- `/api/v1/health`
- `/api/v1/upload/handshake`
- `/api/v1/upload/metadata`
- `/api/v1/upload/nomenclature/batch`

### Legacy эндпоинты

Старые эндпоинты для обратной совместимости:
- `/handshake`
- `/metadata`
- `/constant`
- `/catalog/meta`
- `/catalog/item`
- `/complete`

**Рекомендация:** Используйте API v1 для новых интеграций.

---

## Форматы данных

### JSON

Используется для:
- REST API запросов и ответов
- Списков выгрузок
- Статусов и статистики
- Конфигураций

**Content-Type:** `application/json`

### XML

Используется для:
- Приема данных из 1С
- Получения данных в формате 1С
- Потоковой передачи данных

**Content-Type:** `application/xml`

### Кодировка

Все данные передаются в кодировке **UTF-8**.

---

## Обработка ошибок

### Стандартные HTTP коды

| Код | Описание | Когда используется |
|-----|----------|-------------------|
| 200 | OK | Успешный запрос |
| 201 | Created | Ресурс создан |
| 400 | Bad Request | Неверный формат запроса |
| 404 | Not Found | Ресурс не найден |
| 500 | Internal Server Error | Внутренняя ошибка сервера |
| 503 | Service Unavailable | Сервис временно недоступен |

### Формат ошибки

```json
{
  "error": "Описание ошибки",
  "code": "ERROR_CODE",
  "details": {
    "field": "Дополнительная информация"
  }
}
```

### Примеры ошибок

#### 400 Bad Request

```json
{
  "error": "Invalid request body",
  "code": "INVALID_REQUEST"
}
```

#### 404 Not Found

```json
{
  "error": "Upload not found",
  "code": "NOT_FOUND"
}
```

#### 500 Internal Server Error

```json
{
  "error": "Internal server error",
  "code": "INTERNAL_ERROR"
}
```

---

## Rate Limiting

### Текущие лимиты

- **AI запросы:** Зависит от провайдера (настраивается через `/api/workers/config`)
- **Обычные запросы:** Не ограничены (в текущей версии)

### Заголовки ответа

```
X-RateLimit-Limit: 120
X-RateLimit-Remaining: 95
X-RateLimit-Reset: 1640995200
```

### Превышение лимита

При превышении лимита возвращается:

**HTTP 429 Too Many Requests**

```json
{
  "error": "Rate limit exceeded",
  "code": "RATE_LIMIT_EXCEEDED",
  "retry_after": 60
}
```

---

## Пагинация

### Параметры

- `page` - номер страницы (начиная с 1)
- `limit` - количество элементов на странице (максимум 1000)

### Формат ответа

```json
{
  "data": [...],
  "pagination": {
    "page": 1,
    "limit": 50,
    "total": 150,
    "total_pages": 3
  }
}
```

### Примеры

```
GET /api/uploads?page=1&limit=50
GET /api/uploads?page=2&limit=50
```

---

## Фильтрация и сортировка

### Фильтрация

Поддерживается через query параметры:

```
GET /api/uploads?status=completed
GET /api/uploads/{uuid}/data?type=constants&catalog_names=Номенклатура,Контрагенты
```

### Сортировка

Параметры:
- `sort` - поле для сортировки
- `order` - направление (asc, desc)

```
GET /api/uploads?sort=started_at&order=desc
```

### Доступные поля для сортировки

- `started_at` - дата начала
- `completed_at` - дата завершения
- `total_items` - количество элементов

---

## Эндпоинты

### Health Check

#### GET /api/v1/health

Проверка состояния сервера.

**Ответ:**
```json
{
  "status": "ok",
  "version": "1.0.0",
  "uptime": 3600
}
```

### Управление выгрузками

#### POST /api/v1/upload/handshake

Создание новой выгрузки.

**Запрос (XML):**
```xml
<?xml version="1.0" encoding="UTF-8"?>
<handshake>
  <version_1c>8.3.20</version_1c>
  <config_name>Управление торговлей</config_name>
  <timestamp>2025-01-15T10:30:00</timestamp>
</handshake>
```

**Ответ (XML):**
```xml
<?xml version="1.0" encoding="UTF-8"?>
<handshake_response>
  <success>true</success>
  <upload_uuid>550e8400-e29b-41d4-a716-446655440000</upload_uuid>
  <message>Upload created successfully</message>
  <timestamp>2025-01-15T10:30:00</timestamp>
</handshake_response>
```

#### GET /api/uploads

Список всех выгрузок.

**Параметры:**
- `page` (query, integer) - номер страницы
- `limit` (query, integer) - количество на странице
- `status` (query, string) - фильтр по статусу
- `sort` (query, string) - поле сортировки
- `order` (query, string) - направление сортировки

**Ответ:**
```json
{
  "uploads": [
    {
      "uuid": "550e8400-e29b-41d4-a716-446655440000",
      "status": "completed",
      "started_at": "2025-01-15T10:30:00Z",
      "completed_at": "2025-01-15T10:35:00Z",
      "version_1c": "8.3.20",
      "config_name": "Управление торговлей",
      "total_constants": 10,
      "total_catalogs": 5,
      "total_items": 1500
    }
  ],
  "pagination": {
    "page": 1,
    "limit": 50,
    "total": 100,
    "total_pages": 2
  }
}
```

#### GET /api/uploads/{uuid}

Детали выгрузки.

**Ответ:**
```json
{
  "uuid": "550e8400-e29b-41d4-a716-446655440000",
  "status": "completed",
  "started_at": "2025-01-15T10:30:00Z",
  "completed_at": "2025-01-15T10:35:00Z",
  "catalogs": [
    {
      "name": "Номенклатура",
      "synonym": "Номенклатура",
      "item_count": 1000
    }
  ],
  "constants": [
    {
      "name": "Организация",
      "synonym": "Организация",
      "type": "Строка"
    }
  ]
}
```

#### GET /api/uploads/{uuid}/data

Получение данных выгрузки.

**Параметры:**
- `type` (query, string) - тип данных: all, constants, catalogs
- `catalog_names` (query, string) - фильтр по именам справочников
- `page` (query, integer) - номер страницы
- `limit` (query, integer) - количество элементов

**Ответ (XML):**
```xml
<?xml version="1.0" encoding="UTF-8"?>
<data_response>
  <upload_uuid>550e8400-e29b-41d4-a716-446655440000</upload_uuid>
  <type>all</type>
  <page>1</page>
  <limit>100</limit>
  <total>1500</total>
  <items>
    <item type="constant">
      <name>Организация</name>
      <value>ООО "Ромашка"</value>
    </item>
    <item type="catalog_item" catalog_name="Номенклатура">
      <reference>00000000-0000-0000-0000-000000000001</reference>
      <code>00001</code>
      <name>Товар 1</name>
    </item>
  </items>
</data_response>
```

### Управление базами данных

#### GET /api/databases/list

Список всех баз данных.

**Ответ:**
```json
{
  "databases": [
    {
      "name": "1c_data.db",
      "path": "1c_data.db",
      "size": 32000000,
      "modified_at": "2025-01-15T10:30:00Z",
      "is_current": true
    }
  ]
}
```

#### GET /api/databases/analytics

Аналитика базы данных.

**Параметры:**
- `path` (query, string, required) - путь к файлу базы данных

**Ответ:**
```json
{
  "total_size": 32000000,
  "table_count": 10,
  "table_stats": [
    {
      "name": "catalog_items",
      "row_count": 15000,
      "size": 5000000
    }
  ]
}
```

### Нормализация

#### POST /api/normalization/start

Запуск нормализации.

**Запрос:**
```json
{
  "use_ai": true,
  "min_confidence": 0.7,
  "rate_limit_delay_ms": 100,
  "max_retries": 3,
  "model": "GLM-4.5-Air"
}
```

**Ответ:**
```json
{
  "success": true,
  "message": "Normalization started"
}
```

#### GET /api/normalization/status

Статус нормализации.

**Ответ:**
```json
{
  "isRunning": true,
  "progress": 45.5,
  "processed": 455,
  "total": 1000,
  "success": 450,
  "errors": 5,
  "currentStep": "Normalizing items",
  "logs": [
    "Started normalization",
    "Processing batch 1/10"
  ],
  "startTime": "2025-01-15T10:30:00Z",
  "elapsedTime": "00:05:30",
  "rate": 1.38
}
```

### Классификация

#### POST /api/classification/classify

Классификация товара.

**Запрос:**
```json
{
  "item_name": "Молоток строительный",
  "description": "Молоток для строительных работ",
  "context": {
    "category": "Инструменты"
  }
}
```

**Ответ:**
```json
{
  "category_path": [
    "C",
    "ПРОДУКЦИЯ ОБРАБАТЫВАЮЩЕЙ ПРОМЫШЛЕННОСТИ",
    "Инструменты"
  ],
  "confidence": 0.95,
  "reasoning": "Товар относится к категории инструментов"
}
```

### Управление воркерами

#### GET /api/workers/config

Получить конфигурацию воркеров.

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

#### POST /api/workers/config/update

Обновить конфигурацию воркеров.

**Запрос:**
```json
{
  "action": "update_provider",
  "data": {
    "name": "arliai",
    "enabled": true,
    "priority": 1,
    "max_workers": 4,
    "api_key": "your-api-key"
  }
}
```

**Ответ:**
```json
{
  "message": "Provider updated successfully"
}
```

#### GET /api/workers/arliai/status

Проверить статус подключения Arliai.

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

---

## Примеры использования

### cURL

#### Создание выгрузки

```bash
curl -X POST http://localhost:9999/api/v1/upload/handshake \
  -H "Content-Type: application/xml" \
  -d '<?xml version="1.0" encoding="UTF-8"?>
<handshake>
  <version_1c>8.3.20</version_1c>
  <config_name>Управление торговлей</config_name>
  <timestamp>2025-01-15T10:30:00</timestamp>
</handshake>'
```

#### Получение списка выгрузок

```bash
curl http://localhost:9999/api/uploads?page=1&limit=50
```

#### Запуск нормализации

```bash
curl -X POST http://localhost:9999/api/normalization/start \
  -H "Content-Type: application/json" \
  -d '{
    "use_ai": true,
    "min_confidence": 0.7,
    "model": "GLM-4.5-Air"
  }'
```

### JavaScript (Fetch API)

```javascript
// Создание выгрузки
const response = await fetch('http://localhost:9999/api/v1/upload/handshake', {
  method: 'POST',
  headers: {
    'Content-Type': 'application/xml'
  },
  body: `<?xml version="1.0" encoding="UTF-8"?>
<handshake>
  <version_1c>8.3.20</version_1c>
  <config_name>Управление торговлей</config_name>
  <timestamp>2025-01-15T10:30:00</timestamp>
</handshake>`
});

const data = await response.text();
console.log(data);
```

### Python (requests)

```python
import requests

# Создание выгрузки
response = requests.post(
    'http://localhost:9999/api/v1/upload/handshake',
    headers={'Content-Type': 'application/xml'},
    data='''<?xml version="1.0" encoding="UTF-8"?>
<handshake>
  <version_1c>8.3.20</version_1c>
  <config_name>Управление торговлей</config_name>
  <timestamp>2025-01-15T10:30:00</timestamp>
</handshake>'''
)

print(response.text)
```

---

## Интеграция с инструментами

### Postman

#### Импорт OpenAPI спецификации

1. Откройте Postman
2. Нажмите **Import**
3. Выберите файл `openapi.yaml`
4. Postman автоматически создаст коллекцию со всеми эндпоинтами

#### Настройка переменных окружения

Создайте environment с переменными:
- `base_url`: `http://localhost:9999`
- `api_key`: `your-api-key` (если требуется)

#### Использование

Все эндпоинты будут доступны в коллекции с предзаполненными параметрами.

### Insomnia

#### Импорт OpenAPI спецификации

1. Откройте Insomnia
2. Нажмите **Create** → **Import/Export** → **Import Data**
3. Выберите **OpenAPI 3.0/3.1**
4. Выберите файл `openapi.yaml`

#### Настройка

Создайте environment с переменными:
- `base_url`: `http://localhost:9999`

### Swagger UI

#### Локальный запуск

```bash
# Установка Swagger UI через Docker
docker run -p 8080:8080 -e SWAGGER_JSON=/openapi.yaml -v $(pwd):/usr/share/nginx/html swaggerapi/swagger-ui

# Откройте http://localhost:8080
```

#### Онлайн версия

Загрузите `openapi.yaml` на https://editor.swagger.io/

### Redoc

```bash
# Установка Redoc
npm install -g redoc-cli

# Генерация документации
redoc-cli serve openapi.yaml
```

---

## Changelog

### Version 1.0.0 (2025-01-15)

#### Добавлено
- API v1 эндпоинты
- Управление выгрузками
- Нормализация данных
- Классификация товаров
- Управление воркерами и моделями
- Работа с базами данных
- OpenAPI 3.1 спецификация

#### Изменено
- Улучшена обработка ошибок
- Добавлена поддержка пагинации
- Добавлена фильтрация и сортировка

#### Исправлено
- Исправлены проблемы с кодировкой
- Улучшена производительность

---

## Поддержка

Для вопросов и поддержки:
- Email: support@example.com
- Документация: https://docs.example.com
- GitHub Issues: https://github.com/example/issues

