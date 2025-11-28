# API Документация - Полное руководство

## Обзор

Этот проект содержит полную документацию и спецификацию для RESTful API сервиса работы с выгрузками из 1С.

## Структура документации

### 1. OpenAPI/Swagger спецификация

**Файл:** `openapi.yaml`

Полная OpenAPI 3.1 спецификация со всеми эндпоинтами, схемами данных, примерами запросов и ответов.

**Использование:**
- Импорт в Postman/Insomnia для тестирования
- Генерация клиентских библиотек
- Интерактивная документация через Swagger UI/Redoc

### 2. Человекочитаемая документация

**Файл:** `API_COMPREHENSIVE_DOCUMENTATION.md`

Подробное руководство с:
- Описанием всех эндпоинтов
- Примерами запросов и ответов
- Обработкой ошибок
- Rate limiting
- Пагинацией и фильтрацией

### 3. Руководство по интеграции

**Файл:** `INTEGRATION_GUIDE.md`

Инструкции по интеграции с:
- Postman
- Insomnia
- Swagger UI
- Redoc
- cURL
- HTTPie
- Автоматическая генерация клиентов

### 4. Готовые коллекции

**Файлы:**
- `postman_collection.json` - коллекция для Postman
- `insomnia_collection.json` - коллекция для Insomnia

## Быстрый старт

### Просмотр документации

#### Swagger UI (онлайн)
1. Откройте https://editor.swagger.io/
2. Нажмите **File** → **Import file**
3. Выберите `openapi.yaml`

#### Swagger UI (локально)
```bash
docker run -p 8080:8080 \
  -e SWAGGER_JSON=/openapi.yaml \
  -v $(pwd):/usr/share/nginx/html \
  swaggerapi/swagger-ui
```

Откройте http://localhost:8080

#### Redoc
```bash
npm install -g redoc-cli
redoc-cli serve openapi.yaml
```

### Импорт в Postman

1. Откройте Postman
2. Нажмите **Import**
3. Выберите `openapi.yaml` или `postman_collection.json`
4. Настройте переменные окружения:
   - `base_url`: `http://localhost:9999`

### Импорт в Insomnia

1. Откройте Insomnia
2. Нажмите **Create** → **Import/Export** → **Import Data**
3. Выберите **OpenAPI 3.0/3.1** или **Insomnia v4**
4. Выберите соответствующий файл

## Основные эндпоинты

### Health Check
```
GET /api/v1/health
```

### Управление выгрузками
```
POST /api/v1/upload/handshake
GET  /api/uploads
GET  /api/uploads/{uuid}
GET  /api/uploads/{uuid}/data
```

### Нормализация
```
POST /api/normalization/start
GET  /api/normalization/status
```

### Классификация
```
POST /api/classification/classify
```

### Управление воркерами
```
GET  /api/workers/config
POST /api/workers/config/update
GET  /api/workers/arliai/status
```

### Качество данных
```
GET  /api/quality/stats
GET  /api/quality/violations
POST /api/quality/violations/{id}
```

### КПВЭД
```
GET  /api/kpved/hierarchy
GET  /api/kpved/search
POST /api/kpved/classify-test
POST /api/kpved/reclassify
```

## Версионирование

### API v1
Эндпоинты с префиксом `/api/v1/`:
- `/api/v1/health`
- `/api/v1/upload/handshake`
- `/api/v1/upload/metadata`
- `/api/v1/upload/nomenclature/batch`

### Legacy
Старые эндпоинты для обратной совместимости:
- `/handshake`
- `/metadata`
- `/constant`
- `/catalog/meta`
- `/catalog/item`
- `/complete`

## Форматы данных

### JSON
Используется для REST API запросов и ответов.

**Content-Type:** `application/json`

### XML
Используется для приема данных из 1С и получения данных в формате 1С.

**Content-Type:** `application/xml`

## Аутентификация

В текущей версии API аутентификация не требуется для большинства эндпоинтов. API ключи для AI провайдеров управляются через `/api/workers/config`.

## Rate Limiting

Лимиты зависят от провайдера AI и настраиваются через `/api/workers/config`.

## Пагинация

Все списковые эндпоинты поддерживают пагинацию:
- `page` - номер страницы (начиная с 1)
- `limit` - количество элементов (максимум 1000)

## Обработка ошибок

Стандартные HTTP коды:
- `200` - OK
- `400` - Bad Request
- `404` - Not Found
- `500` - Internal Server Error

Формат ошибки:
```json
{
  "error": "Описание ошибки",
  "code": "ERROR_CODE",
  "details": {}
}
```

## Примеры использования

### cURL

```bash
# Health check
curl http://localhost:9999/api/v1/health

# Список выгрузок
curl http://localhost:9999/api/uploads?page=1&limit=50

# Запуск нормализации
curl -X POST http://localhost:9999/api/normalization/start \
  -H "Content-Type: application/json" \
  -d '{"use_ai": true, "min_confidence": 0.7}'
```

### JavaScript

```javascript
const response = await fetch('http://localhost:9999/api/v1/health');
const data = await response.json();
console.log(data);
```

### Python

```python
import requests

response = requests.get('http://localhost:9999/api/v1/health')
print(response.json())
```

## Генерация клиентских библиотек

### OpenAPI Generator

```bash
# JavaScript
openapi-generator generate -i openapi.yaml -g javascript -o ./client/js

# Python
openapi-generator generate -i openapi.yaml -g python -o ./client/python

# Go
openapi-generator generate -i openapi.yaml -g go -o ./client/go
```

## Changelog

### Version 1.0.0 (2025-01-15)

#### Добавлено
- OpenAPI 3.1 спецификация
- Полная документация всех эндпоинтов
- Коллекции для Postman и Insomnia
- Примеры использования на разных языках
- Инструкции по интеграции

#### Эндпоинты
- Health Check
- Управление выгрузками
- Нормализация данных
- Классификация товаров
- Переклассификация
- Управление качеством данных
- Работа с КПВЭД
- Управление воркерами и моделями
- Мониторинг производительности

## Поддержка

Для вопросов и поддержки:
- Документация: `API_COMPREHENSIVE_DOCUMENTATION.md`
- Интеграция: `INTEGRATION_GUIDE.md`
- OpenAPI спецификация: `openapi.yaml`

## Дополнительные ресурсы

- [OpenAPI Specification](https://swagger.io/specification/)
- [Postman Documentation](https://learning.postman.com/)
- [Insomnia Documentation](https://docs.insomnia.rest/)
- [Swagger UI](https://swagger.io/tools/swagger-ui/)
- [Redoc](https://github.com/Redocly/redoc)

