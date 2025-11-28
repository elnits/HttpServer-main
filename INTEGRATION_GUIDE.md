# Руководство по интеграции с инструментами разработки

## Содержание

1. [Postman](#postman)
2. [Insomnia](#insomnia)
3. [Swagger UI](#swagger-ui)
4. [Redoc](#redoc)
5. [cURL](#curl)
6. [HTTPie](#httpie)
7. [Автоматическая генерация клиентов](#автоматическая-генерация-клиентов)

---

## Postman

### Импорт OpenAPI спецификации

1. Откройте Postman
2. Нажмите **Import** в левом верхнем углу
3. Выберите вкладку **File**
4. Выберите файл `openapi.yaml`
5. Нажмите **Import**

Postman автоматически создаст коллекцию со всеми эндпоинтами из спецификации.

### Импорт готовой коллекции

1. Откройте Postman
2. Нажмите **Import**
3. Выберите файл `postman_collection.json`
4. Нажмите **Import**

### Настройка переменных окружения

1. Нажмите на иконку **Environments** в левой панели
2. Нажмите **+** для создания нового environment
3. Добавьте переменные:
   - `base_url`: `http://localhost:9999`
   - `api_key`: `your-api-key` (если требуется)
   - `upload_uuid`: (будет заполняться автоматически)

4. Выберите созданный environment в выпадающем списке в правом верхнем углу

### Использование переменных

В запросах используйте переменные через двойные фигурные скобки:

```
{{base_url}}/api/uploads
{{upload_uuid}}
```

### Автоматическое тестирование

Создайте тесты в разделе **Tests** для каждого запроса:

```javascript
// Проверка статуса ответа
pm.test("Status code is 200", function () {
    pm.response.to.have.status(200);
});

// Сохранение upload_uuid для последующих запросов
if (pm.response.code === 200) {
    const response = pm.response.json();
    if (response.upload_uuid) {
        pm.environment.set("upload_uuid", response.upload_uuid);
    }
}

// Проверка структуры ответа
pm.test("Response has required fields", function () {
    const jsonData = pm.response.json();
    pm.expect(jsonData).to.have.property('success');
});
```

### Pre-request Scripts

Используйте pre-request scripts для автоматизации:

```javascript
// Автоматическая генерация timestamp
const timestamp = new Date().toISOString();
pm.environment.set("timestamp", timestamp);
```

### Экспорт коллекции

1. Выберите коллекцию
2. Нажмите **...** (три точки)
3. Выберите **Export**
4. Выберите формат (Collection v2.1)
5. Сохраните файл

---

## Insomnia

### Импорт OpenAPI спецификации

1. Откройте Insomnia
2. Нажмите **Create** → **Import/Export** → **Import Data**
3. Выберите **OpenAPI 3.0/3.1**
4. Выберите файл `openapi.yaml`
5. Нажмите **Import**

### Импорт готовой коллекции

1. Откройте Insomnia
2. Нажмите **Create** → **Import/Export** → **Import Data**
3. Выберите **Insomnia v4**
4. Выберите файл `insomnia_collection.json`
5. Нажмите **Import**

### Настройка переменных окружения

1. Нажмите на иконку **Manage Environments** (шестеренка)
2. Нажмите **Create Environment**
3. Добавьте переменные:
   - `base_url`: `http://localhost:9999`
   - `api_key`: `your-api-key`

4. Выберите созданный environment в выпадающем списке

### Использование переменных

В запросах используйте переменные через двойные фигурные скобки:

```
{{ _.base_url }}/api/uploads
```

### Генерация кода

Insomnia может генерировать код для различных языков:

1. Выберите запрос
2. Нажмите **Generate Code** (справа)
3. Выберите язык (JavaScript, Python, cURL, etc.)
4. Скопируйте сгенерированный код

---

## Swagger UI

### Локальный запуск через Docker

```bash
docker run -p 8080:8080 \
  -e SWAGGER_JSON=/openapi.yaml \
  -v $(pwd):/usr/share/nginx/html \
  swaggerapi/swagger-ui
```

Откройте http://localhost:8080 в браузере.

### Локальный запуск через npm

```bash
# Установка
npm install -g swagger-ui-serve

# Запуск
swagger-ui-serve openapi.yaml
```

### Онлайн редактор

1. Откройте https://editor.swagger.io/
2. Нажмите **File** → **Import file**
3. Выберите файл `openapi.yaml`
4. Документация отобразится автоматически

### Встраивание в веб-сайт

```html
<!DOCTYPE html>
<html>
<head>
  <title>API Documentation</title>
  <link rel="stylesheet" type="text/css" href="https://unpkg.com/swagger-ui-dist@4.15.5/swagger-ui.css" />
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://unpkg.com/swagger-ui-dist@4.15.5/swagger-ui-bundle.js"></script>
  <script>
    window.onload = function() {
      SwaggerUIBundle({
        url: "/openapi.yaml",
        dom_id: '#swagger-ui',
      });
    };
  </script>
</body>
</html>
```

---

## Redoc

### Установка и запуск

```bash
# Установка через npm
npm install -g redoc-cli

# Генерация статической HTML документации
redoc-cli build openapi.yaml -o api-docs.html

# Запуск локального сервера
redoc-cli serve openapi.yaml
```

### Встраивание в веб-сайт

```html
<!DOCTYPE html>
<html>
<head>
  <title>API Documentation</title>
  <link href="https://fonts.googleapis.com/css?family=Montserrat:300,400,700|Roboto:300,400,700" rel="stylesheet">
  <style>
    body {
      margin: 0;
      padding: 0;
    }
  </style>
</head>
<body>
  <redoc spec-url="/openapi.yaml"></redoc>
  <script src="https://cdn.redoc.ly/redoc/latest/bundles/redoc.standalone.js"></script>
</body>
</html>
```

---

## cURL

### Базовое использование

```bash
# GET запрос
curl http://localhost:9999/api/v1/health

# POST запрос с JSON
curl -X POST http://localhost:9999/api/normalization/start \
  -H "Content-Type: application/json" \
  -d '{
    "use_ai": true,
    "min_confidence": 0.7
  }'

# POST запрос с XML
curl -X POST http://localhost:9999/api/v1/upload/handshake \
  -H "Content-Type: application/xml" \
  -d '<?xml version="1.0" encoding="UTF-8"?>
<handshake>
  <version_1c>8.3.20</version_1c>
  <config_name>Управление торговлей</config_name>
  <timestamp>2025-01-15T10:30:00</timestamp>
</handshake>'
```

### Сохранение ответа в файл

```bash
curl http://localhost:9999/api/uploads -o response.json
```

### Показ заголовков

```bash
curl -i http://localhost:9999/api/v1/health
```

### С таймаутом

```bash
curl --max-time 7 http://localhost:9999/api/v1/health
```

---

## HTTPie

### Установка

```bash
# macOS
brew install httpie

# Linux
apt install httpie

# Windows
pip install httpie
```

### Использование

```bash
# GET запрос
http GET http://localhost:9999/api/v1/health

# POST запрос с JSON
http POST http://localhost:9999/api/normalization/start \
  use_ai:=true \
  min_confidence:=0.7 \
  model="GLM-4.5-Air"

# POST запрос с файлом
http POST http://localhost:9999/api/v1/upload/handshake \
  Content-Type:application/xml < handshake.xml
```

---

## Автоматическая генерация клиентов

### OpenAPI Generator

#### Установка

```bash
# Через npm
npm install -g @openapitools/openapi-generator-cli

# Через Homebrew (macOS)
brew install openapi-generator
```

#### Генерация клиента для JavaScript

```bash
openapi-generator generate \
  -i openapi.yaml \
  -g javascript \
  -o ./generated-client/js
```

#### Генерация клиента для Python

```bash
openapi-generator generate \
  -i openapi.yaml \
  -g python \
  -o ./generated-client/python
```

#### Генерация клиента для Go

```bash
openapi-generator generate \
  -i openapi.yaml \
  -g go \
  -o ./generated-client/go
```

#### Доступные генераторы

- `javascript` - JavaScript/TypeScript клиент
- `python` - Python клиент
- `go` - Go клиент
- `java` - Java клиент
- `csharp` - C# клиент
- `php` - PHP клиент
- `ruby` - Ruby клиент
- `swift` - Swift клиент
- `kotlin` - Kotlin клиент

### Swagger Codegen

#### Установка

```bash
# Через Homebrew
brew install swagger-codegen

# Или скачайте JAR файл
wget https://repo1.maven.org/maven2/io/swagger/codegen/v3/swagger-codegen-cli/3.0.34/swagger-codegen-cli-3.0.34.jar
```

#### Генерация клиента

```bash
java -jar swagger-codegen-cli.jar generate \
  -i openapi.yaml \
  -l javascript \
  -o ./generated-client/js
```

---

## Примеры интеграции

### JavaScript/TypeScript

```typescript
// Установка: npm install axios

import axios from 'axios';

const API_BASE_URL = 'http://localhost:9999';

const api = axios.create({
  baseURL: API_BASE_URL,
  headers: {
    'Content-Type': 'application/json',
  },
});

// Создание выгрузки
async function createHandshake() {
  const response = await api.post('/api/v1/upload/handshake', 
    `<?xml version="1.0" encoding="UTF-8"?>
<handshake>
  <version_1c>8.3.20</version_1c>
  <config_name>Управление торговлей</config_name>
  <timestamp>${new Date().toISOString()}</timestamp>
</handshake>`,
    {
      headers: {
        'Content-Type': 'application/xml',
      },
    }
  );
  return response.data;
}

// Получение списка выгрузок
async function listUploads(page = 1, limit = 50) {
  const response = await api.get('/api/uploads', {
    params: { page, limit },
  });
  return response.data;
}
```

### Python

```python
# Установка: pip install requests

import requests

API_BASE_URL = 'http://localhost:9999'

def create_handshake():
    url = f'{API_BASE_URL}/api/v1/upload/handshake'
    headers = {'Content-Type': 'application/xml'}
    data = '''<?xml version="1.0" encoding="UTF-8"?>
<handshake>
  <version_1c>8.3.20</version_1c>
  <config_name>Управление торговлей</config_name>
  <timestamp>2025-01-15T10:30:00</timestamp>
</handshake>'''
    response = requests.post(url, headers=headers, data=data)
    return response.json()

def list_uploads(page=1, limit=50):
    url = f'{API_BASE_URL}/api/uploads'
    params = {'page': page, 'limit': limit}
    response = requests.get(url, params=params)
    return response.json()
```

### Go

```go
package main

import (
    "bytes"
    "encoding/json"
    "fmt"
    "net/http"
    "time"
)

const APIBaseURL = "http://localhost:9999"

type Client struct {
    BaseURL    string
    HTTPClient *http.Client
}

func NewClient() *Client {
    return &Client{
        BaseURL: APIBaseURL,
        HTTPClient: &http.Client{
            Timeout: 30 * time.Second,
        },
    }
}

func (c *Client) CreateHandshake() (map[string]interface{}, error) {
    url := fmt.Sprintf("%s/api/v1/upload/handshake", c.BaseURL)
    
    xmlData := `<?xml version="1.0" encoding="UTF-8"?>
<handshake>
  <version_1c>8.3.20</version_1c>
  <config_name>Управление торговлей</config_name>
  <timestamp>2025-01-15T10:30:00</timestamp>
</handshake>`
    
    req, err := http.NewRequest("POST", url, bytes.NewBufferString(xmlData))
    if err != nil {
        return nil, err
    }
    
    req.Header.Set("Content-Type", "application/xml")
    
    resp, err := c.HTTPClient.Do(req)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()
    
    var result map[string]interface{}
    json.NewDecoder(resp.Body).Decode(&result)
    return result, nil
}
```

---

## Рекомендации

1. **Используйте переменные окружения** для базового URL и API ключей
2. **Настройте таймауты** для всех запросов
3. **Обрабатывайте ошибки** правильно
4. **Используйте retry логику** для неустойчивых соединений
5. **Кэшируйте ответы** где это возможно
6. **Логируйте запросы** для отладки
7. **Используйте версионирование** API для стабильности

---

## Дополнительные ресурсы

- [OpenAPI Specification](https://swagger.io/specification/)
- [Postman Documentation](https://learning.postman.com/)
- [Insomnia Documentation](https://docs.insomnia.rest/)
- [Swagger UI](https://swagger.io/tools/swagger-ui/)
- [Redoc](https://github.com/Redocly/redoc)

