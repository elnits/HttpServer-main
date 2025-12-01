# API Документация - HTTP Server для работы с выгрузками из 1С

> **🆕 ОБНОВЛЕНИЕ АРХИТЕКТУРЫ (2025-11-30)**  
> Сервер был обновлён для использования **единой базы данных** (`unified_catalogs.db`) вместо создания отдельных файлов для каждой выгрузки.  
> - Все выгрузки хранятся в одной БД
> - Для каждого справочника создаётся отдельная таблица
> - Обратная совместимость со старыми API сохранена
> - См. [MIGRATION_GUIDE.md](MIGRATION_GUIDE.md) для деталей миграции

## Содержание

1. [Введение](#введение)
2. [Архитектура системы](#архитектура-системы)
3. [Выгрузка данных из 1С](#выгрузка-данных-из-1с)
4. [Загрузка данных в 1С](#загрузка-данных-в-1с)
5. [API для работы с выгрузками](#api-для-работы-с-выгрузками)
6. [Обработка ошибок](#обработка-ошибок)
7. [Технические детали](#технические-детали)

---

## Введение

### Общая информация

HTTP Server для работы с выгрузками из 1С - это RESTful API сервер на Go для обмена данными с 1С:Предприятие.

**Базовый URL:** `http://localhost:9999`

**Технологии:**
- Язык: Go (Golang)
- База данных: SQLite3
- Протокол: HTTP/1.1
- Форматы: JSON, XML
- Кодировка: UTF-8

**Ключевые возможности:**
- ✅ Выгрузка данных из 1С на сервер (экспорт из 1С)
- ✅ Загрузка данных из сервера в 1С (импорт в 1С)
- ✅ Просмотр выгруженных данных через Web API (JSON)
- ✅ **Единая БД для всех выгрузок** (новая архитектура)
- ✅ **Динамические таблицы для каждого справочника**
- ✅ Поддержка констант и справочников
- ✅ Пагинация и потоковая передача
- ✅ CORS для всех источников

### Примеры запросов

**⚠️ Важно:** Примеры curl в документации приведены в трех форматах:
- **Linux/Mac (bash)** - используются одинарные кавычки `'` и обратный слэш `\` для переноса строк
- **Windows (CMD)** - используются двойные кавычки `"` и экранирование `\"`, все в одну строку
- **Windows (PowerShell)** - используется `Invoke-WebRequest` с heredoc синтаксисом `@" ... "@`

Выбирайте подходящий формат для вашей операционной системы!

---

## Архитектура системы

### Компоненты

```
┌─────────────────────────────────────────────────────┐
│              1С:Предприятие                         │
│  (Экспорт данных / Импорт данных)                   │
└──────────────────┬──────────────────────────────────┘
                   │
                   │ HTTP (XML/JSON)
                   │
┌──────────────────▼──────────────────────────────────┐
│              HTTP Server (Go)                       │
│  ┌──────────────────────────────────────────────┐  │
│  │  Handlers: Экспорт, Импорт, API              │  │
│  └──────────────────┬───────────────────────────┘  │
│                     │                               │
│  ┌──────────────────▼───────────────────────────┐  │
│  │  Database Layer (SQLite)                     │  │
│  └──────────────────┬───────────────────────────┘  │
└────────────────────┬────────────────────────────────┘
                     │
┌────────────────────▼────────────────────────────────┐
│           Файловая система                          │
│                                                     │
│  data/                                              │
│  ├── service.db                   (метаданные)     │
│  ├── unified_catalogs.db          (ЕДИНАЯ БД)      │
│  ├── normalized_data.db           (нормализация)   │
│  └── [старые Выгрузка_*.db]       (обратн. совм.)  │
└─────────────────────────────────────────────────────┘
```

### Структура баз данных

**1. Сервисная БД (`service.db`)**
- Хранит метаинформацию о клиентах и проектах
- Содержит пути к файлам выгрузок
- Используется для управления выгрузками

**2. Единая БД справочников (`unified_catalogs.db`) - НОВАЯ АРХИТЕКТУРА**
- Все выгрузки в одной базе данных
- Для каждого справочника создаётся отдельная таблица
- Автоматическая транслитерация названий (Номенклатура → nomenclature_items)
- Содержит:
  - Метаданные всех выгрузок (таблица `uploads`)
  - Все константы (таблица `constants`)
  - Маппинг справочников (таблица `catalog_mappings`)
  - Динамические таблицы справочников (nomenclature_items, contragents_items, и т.д.)

**3. Старые базы выгрузок (обратная совместимость)**
- Формат имени: `Выгрузка_<Тип>_<Конфигурация>_<Дата-Время>.db`
- Пример: `Выгрузка_Номенклатура_ERPWE_mpf_Unknown_2025-11-25_00-16-03.db`
- Поддерживаются для чтения, но новые выгрузки создаются в единой БД

**Структура таблиц в БД выгрузки:**

```sql
-- Метаданные выгрузки
CREATE TABLE uploads (
    id INTEGER PRIMARY KEY,
    upload_uuid TEXT UNIQUE,
    started_at TIMESTAMP,
    completed_at TIMESTAMP,
    status TEXT,
    version_1c TEXT,
    config_name TEXT,
    total_constants INTEGER,
    total_catalogs INTEGER,
    total_items INTEGER
);

-- Константы
CREATE TABLE constants (
    id INTEGER PRIMARY KEY,
    upload_id INTEGER,
    name TEXT,
    synonym TEXT,
    type TEXT,
    value TEXT,
    created_at TIMESTAMP,
    FOREIGN KEY (upload_id) REFERENCES uploads(id) ON DELETE CASCADE
);

-- Справочники
CREATE TABLE catalogs (
    id INTEGER PRIMARY KEY,
    upload_id INTEGER,
    name TEXT,
    synonym TEXT,
    created_at TIMESTAMP,
    FOREIGN KEY (upload_id) REFERENCES uploads(id) ON DELETE CASCADE
);

-- Элементы справочников
CREATE TABLE catalog_items (
    id INTEGER PRIMARY KEY,
    catalog_id INTEGER,
    reference TEXT,
    code TEXT,
    name TEXT,
    attributes TEXT,      -- XML строка (соответствует тегу <attributes_xml>)
    table_parts TEXT,     -- XML строка (соответствует тегу <table_parts_xml>)
    created_at TIMESTAMP,
    FOREIGN KEY (catalog_id) REFERENCES catalogs(id) ON DELETE CASCADE
);
```

### Преимущества архитектуры

✅ **Изоляция данных** - каждая выгрузка независима  
✅ **Простота управления** - легко удалить/скопировать/архивировать  
✅ **Масштабируемость** - нет ограничений на количество выгрузок  
✅ **Безопасность** - повреждение одной БД не влияет на другие  
✅ **Производительность** - нет блокировок между выгрузками

---

## Выгрузка данных из 1С

Процесс экспорта данных из 1С:Предприятие на сервер.

### Описание процесса

**Последовательность операций:**

```
1. Handshake      → Создание выгрузки, получение UUID
2. Metadata       → Отправка метаданных
3. Constants      → Отправка констант (по одной)
4. Catalog Meta   → Регистрация справочников
5. Catalog Items  → Отправка элементов справочников
6. Complete       → Завершение выгрузки
```

**Что происходит на сервере:**
1. Создается новый файл БД с уникальным именем
2. Генерируется UUID для идентификации выгрузки
3. Данные сохраняются в отдельную БД
4. После завершения выгрузка доступна через API

---

### Эндпоинты

#### 1. POST /handshake

Начинает новую выгрузку. Создает файл БД и возвращает UUID.

**Тело запроса (XML):**
```xml
<?xml version="1.0" encoding="UTF-8"?>
<handshake>
  <version_1c>8.3.25.1257</version_1c>
  <config_name>ERPWE</config_name>
  <timestamp>2025-11-27T12:00:00Z</timestamp>
</handshake>
```

**Ответ (XML):**
```xml
<?xml version="1.0" encoding="UTF-8"?>
<handshake_response>
  <success>true</success>
  <upload_uuid>550e8400-e29b-41d4-a716-446655440000</upload_uuid>
  <message>Handshake successful</message>
  <timestamp>2025-11-27T12:00:00Z</timestamp>
</handshake_response>
```

**Пример использования (curl):**

**Linux/Mac (bash):**
```bash
curl -X POST http://localhost:9999/handshake \
  -H "Content-Type: application/xml; charset=utf-8" \
  -d '<?xml version="1.0" encoding="UTF-8"?>
<handshake>
  <version_1c>8.3.25.1257</version_1c>
  <config_name>ERPWE</config_name>
  <timestamp>2025-11-27T12:00:00Z</timestamp>
</handshake>'
```

**Windows (CMD):**
```cmd
curl -X POST http://localhost:9999/handshake -H "Content-Type: application/xml; charset=utf-8" -d "<?xml version=\"1.0\" encoding=\"UTF-8\"?><handshake><version_1c>8.3.25.1257</version_1c><config_name>ERPWE</config_name><timestamp>2025-11-27T12:00:00Z</timestamp></handshake>"
```

**Важно:** Сохраните `upload_uuid` для всех последующих запросов.

---

#### 2. POST /metadata

Отправляет метаданные выгрузки из 1С на сервер.

**Кто отправляет:** 1С:Предприятие (обработка БитЭкспортер)  
**Куда:** HTTP сервер на Go  
**Где хранится:** В таблице `uploads` в SQLite базе данных конкретной выгрузки  
**Назначение:** Подтверждение начала процесса выгрузки и дублирование ключевой информации

**Тело запроса (XML):**
```xml
<?xml version="1.0" encoding="UTF-8"?>
<metadata>
  <upload_uuid>550e8400-e29b-41d4-a716-446655440000</upload_uuid>
  <version_1c>8.3.25.1257</version_1c>
  <config_name>ERPWE</config_name>
  <timestamp>2025-11-27T12:00:05Z</timestamp>
</metadata>
```

**Ответ (XML):**
```xml
<?xml version="1.0" encoding="UTF-8"?>
<metadata_response>
  <success>true</success>
  <message>Metadata received successfully</message>
  <timestamp>2025-11-27T12:00:05Z</timestamp>
</metadata_response>
```

**Пример использования (curl):**

**Linux/Mac (bash):**
```bash
curl -X POST http://localhost:9999/metadata \
  -H "Content-Type: application/xml; charset=utf-8" \
  -d '<?xml version="1.0" encoding="UTF-8"?>
<metadata>
  <upload_uuid>550e8400-e29b-41d4-a716-446655440000</upload_uuid>
  <version_1c>8.3.25.1257</version_1c>
  <config_name>ERPWE</config_name>
  <timestamp>2025-11-27T12:00:05Z</timestamp>
</metadata>'
```

**Windows (CMD):**
```cmd
curl -X POST http://localhost:9999/metadata -H "Content-Type: application/xml; charset=utf-8" -d "<?xml version=\"1.0\" encoding=\"UTF-8\"?><metadata><upload_uuid>550e8400-e29b-41d4-a716-446655440000</upload_uuid><version_1c>8.3.25.1257</version_1c><config_name>ERPWE</config_name><timestamp>2025-11-27T12:00:05Z</timestamp></metadata>"
```

**Примечание:**  
- Эти данные дублируют информацию из `/handshake` и сохраняются в поле `version_1c` и `config_name` таблицы `uploads`
- Фактически этот эндпоинт подтверждает успешное рукопожатие и готовность начать выгрузку
- Данные хранятся в SQLite файле конкретной выгрузки (например: `Выгрузка_ПолнаяВыгрузка_ERPWE_Unknown_Unknown_2025-11-27_16-03-47.db`)

---

#### 3. POST /constant

Добавляет константу в выгрузку.

**Тело запроса (XML):**
```xml
<?xml version="1.0" encoding="UTF-8"?>
<constant>
  <upload_uuid>550e8400-e29b-41d4-a716-446655440000</upload_uuid>
  <name>ОсновнаяВалюта</name>
  <synonym>Основная валюта</synonym>
  <type>СправочникСсылка.Валюты</type>
  <value>643</value>
  <timestamp>2025-11-27T12:00:10Z</timestamp>
</constant>
```

**Ответ (XML):**
```xml
<?xml version="1.0" encoding="UTF-8"?>
<constant_response>
  <success>true</success>
  <message>Constant added successfully</message>
  <timestamp>2025-11-27T12:00:10Z</timestamp>
</constant_response>
```

**Пример использования (curl):**

**Linux/Mac (bash):**
```bash
curl -X POST http://localhost:9999/constant \
  -H "Content-Type: application/xml; charset=utf-8" \
  -d '<?xml version="1.0" encoding="UTF-8"?>
<constant>
  <upload_uuid>550e8400-e29b-41d4-a716-446655440000</upload_uuid>
  <name>ОсновнаяВалюта</name>
  <synonym>Основная валюта</synonym>
  <type>СправочникСсылка.Валюты</type>
  <value>643</value>
  <timestamp>2025-11-27T12:00:10Z</timestamp>
</constant>'
```

**Windows (CMD):**
```cmd
curl -X POST http://localhost:9999/constant -H "Content-Type: application/xml; charset=utf-8" -d "<?xml version=\"1.0\" encoding=\"UTF-8\"?><constant><upload_uuid>550e8400-e29b-41d4-a716-446655440000</upload_uuid><name>ОсновнаяВалюта</name><synonym>Основная валюта</synonym><type>СправочникСсылка.Валюты</type><value>643</value><timestamp>2025-11-27T12:00:10Z</timestamp></constant>"
```

**Примечание:** Поле `value` может содержать сложные XML структуры.

---

#### 4. POST /catalog/meta

Регистрирует справочник.

**Тело запроса (XML):**
```xml
<?xml version="1.0" encoding="UTF-8"?>
<catalog_meta>
  <upload_uuid>550e8400-e29b-41d4-a716-446655440000</upload_uuid>
  <name>Номенклатура</name>
  <synonym>Номенклатура</synonym>
  <timestamp>2025-11-27T12:00:15Z</timestamp>
</catalog_meta>
```

**Ответ (XML):**
```xml
<?xml version="1.0" encoding="UTF-8"?>
<catalog_meta_response>
  <success>true</success>
  <catalog_id>1</catalog_id>
  <message>Catalog metadata added successfully</message>
  <timestamp>2025-11-27T12:00:15Z</timestamp>
</catalog_meta_response>
```

**Пример использования (curl):**

**Linux/Mac (bash):**
```bash
curl -X POST http://localhost:9999/catalog/meta \
  -H "Content-Type: application/xml; charset=utf-8" \
  -d '<?xml version="1.0" encoding="UTF-8"?>
<catalog_meta>
  <upload_uuid>550e8400-e29b-41d4-a716-446655440000</upload_uuid>
  <name>Номенклатура</name>
  <synonym>Номенклатура</synonym>
  <timestamp>2025-11-27T12:00:15Z</timestamp>
</catalog_meta>'
```

**Windows (CMD):**
```cmd
curl -X POST http://localhost:9999/catalog/meta -H "Content-Type: application/xml; charset=utf-8" -d "<?xml version=\"1.0\" encoding=\"UTF-8\"?><catalog_meta><upload_uuid>550e8400-e29b-41d4-a716-446655440000</upload_uuid><name>Номенклатура</name><synonym>Номенклатура</synonym><timestamp>2025-11-27T12:00:15Z</timestamp></catalog_meta>"
```

---

#### 5. POST /catalog/item

Добавляет элемент справочника.

**Тело запроса (XML):**
```xml
<?xml version="1.0" encoding="UTF-8"?>
<catalog_item>
  <upload_uuid>550e8400-e29b-41d4-a716-446655440000</upload_uuid>
  <catalog_name>Номенклатура</catalog_name>
  <reference>8ca1aeb0-67d6-11e8-80ce-00155d647400</reference>
  <code>000000001</code>
  <name>Ноутбук ASUS ROG Strix G15</name>
  <attributes_xml>
    <Артикул>G513QM-HN064</Артикул>
    <ЕдиницаИзмерения>шт</ЕдиницаИзмерения>
    <Цена>125000.00</Цена>
  </attributes_xml>
  <table_parts_xml>
    <ДополнительныеРеквизиты></ДополнительныеРеквизиты>
  </table_parts_xml>
  <timestamp>2025-11-27T12:00:20Z</timestamp>
</catalog_item>
```

**Ответ (XML):**
```xml
<?xml version="1.0" encoding="UTF-8"?>
<catalog_item_response>
  <success>true</success>
  <message>Catalog item added successfully</message>
  <timestamp>2025-11-27T12:00:20Z</timestamp>
</catalog_item_response>
```

**Пример использования (curl):**

**Linux/Mac (bash):**
```bash
curl -X POST http://localhost:9999/catalog/item \
  -H "Content-Type: application/xml; charset=utf-8" \
  -d '<?xml version="1.0" encoding="UTF-8"?>
<catalog_item>
  <upload_uuid>550e8400-e29b-41d4-a716-446655440000</upload_uuid>
  <catalog_name>Номенклатура</catalog_name>
  <reference>8ca1aeb0-67d6-11e8-80ce-00155d647400</reference>
  <code>000000001</code>
  <name>Ноутбук ASUS ROG Strix G15</name>
  <attributes_xml>
    <Артикул>G513QM-HN064</Артикул>
    <ЕдиницаИзмерения>шт</ЕдиницаИзмерения>
    <Цена>125000.00</Цена>
  </attributes_xml>
  <table_parts_xml>
    <ДополнительныеРеквизиты></ДополнительныеРеквизиты>
  </table_parts_xml>
  <timestamp>2025-11-27T12:00:20Z</timestamp>
</catalog_item>'
```

**Windows (CMD):**
```cmd
curl -X POST http://localhost:9999/catalog/item -H "Content-Type: application/xml; charset=utf-8" -d "<?xml version=\"1.0\" encoding=\"UTF-8\"?><catalog_item><upload_uuid>550e8400-e29b-41d4-a716-446655440000</upload_uuid><catalog_name>Номенклатура</catalog_name><reference>8ca1aeb0-67d6-11e8-80ce-00155d647400</reference><code>000000001</code><name>Ноутбук ASUS ROG Strix G15</name><attributes_xml><Артикул>G513QM-HN064</Артикул><ЕдиницаИзмерения>шт</ЕдиницаИзмерения><Цена>125000.00</Цена></attributes_xml><table_parts_xml><ДополнительныеРеквизиты></ДополнительныеРеквизиты></table_parts_xml><timestamp>2025-11-27T12:00:20Z</timestamp></catalog_item>"
```

**Примечание:** Поля `attributes_xml` и `table_parts_xml` сохраняются как XML строки.

---

#### 6. POST /complete

Завершает выгрузку.

**Тело запроса (XML):**
```xml
<?xml version="1.0" encoding="UTF-8"?>
<complete>
  <upload_uuid>550e8400-e29b-41d4-a716-446655440000</upload_uuid>
  <timestamp>2025-11-27T12:10:00Z</timestamp>
</complete>
```

**Ответ (XML):**
```xml
<?xml version="1.0" encoding="UTF-8"?>
<complete_response>
  <success>true</success>
  <message>Upload completed successfully</message>
  <timestamp>2025-11-27T12:10:00Z</timestamp>
</complete_response>
```

**Пример использования (curl):**

**Linux/Mac (bash):**
```bash
curl -X POST http://localhost:9999/complete \
  -H "Content-Type: application/xml; charset=utf-8" \
  -d '<?xml version="1.0" encoding="UTF-8"?>
<complete>
  <upload_uuid>550e8400-e29b-41d4-a716-446655440000</upload_uuid>
  <timestamp>2025-11-27T12:10:00Z</timestamp>
</complete>'
```

**Windows (CMD):**
```cmd
curl -X POST http://localhost:9999/complete -H "Content-Type: application/xml; charset=utf-8" -d "<?xml version=\"1.0\" encoding=\"UTF-8\"?><complete><upload_uuid>550e8400-e29b-41d4-a716-446655440000</upload_uuid><timestamp>2025-11-27T12:10:00Z</timestamp></complete>"
```

---

### Примеры использования

#### Пример 1: Полный цикл выгрузки (curl)

```bash
# Шаг 1: Handshake
curl -X POST http://localhost:9999/handshake \
  -H "Content-Type: application/xml; charset=utf-8" \
  -d '<?xml version="1.0" encoding="UTF-8"?>
<handshake>
  <version_1c>8.3.25.1257</version_1c>
  <config_name>ERPWE</config_name>
  <timestamp>2025-11-27T12:00:00Z</timestamp>
</handshake>'

# Ответ: Сохраните upload_uuid

# Шаг 2: Metadata
curl -X POST http://localhost:9999/metadata \
  -H "Content-Type: application/xml; charset=utf-8" \
  -d '<?xml version="1.0" encoding="UTF-8"?>
<metadata>
  <upload_uuid>550e8400-e29b-41d4-a716-446655440000</upload_uuid>
  <version_1c>8.3.25.1257</version_1c>
  <config_name>ERPWE</config_name>
  <timestamp>2025-11-27T12:00:05Z</timestamp>
</metadata>'

# Шаг 3: Добавить константу
curl -X POST http://localhost:9999/constant \
  -H "Content-Type: application/xml; charset=utf-8" \
  -d '<?xml version="1.0" encoding="UTF-8"?>
<constant>
  <upload_uuid>550e8400-e29b-41d4-a716-446655440000</upload_uuid>
  <name>ОсновнаяВалюта</name>
  <synonym>Основная валюта</synonym>
  <type>СправочникСсылка.Валюты</type>
  <value>643</value>
  <timestamp>2025-11-27T12:00:10Z</timestamp>
</constant>'

# Шаг 4: Зарегистрировать справочник
curl -X POST http://localhost:9999/catalog/meta \
  -H "Content-Type: application/xml; charset=utf-8" \
  -d '<?xml version="1.0" encoding="UTF-8"?>
<catalog_meta>
  <upload_uuid>550e8400-e29b-41d4-a716-446655440000</upload_uuid>
  <name>Номенклатура</name>
  <synonym>Номенклатура</synonym>
  <timestamp>2025-11-27T12:00:15Z</timestamp>
</catalog_meta>'

# Шаг 5: Добавить элемент справочника
curl -X POST http://localhost:9999/catalog/item \
  -H "Content-Type: application/xml; charset=utf-8" \
  -d '<?xml version="1.0" encoding="UTF-8"?>
<catalog_item>
  <upload_uuid>550e8400-e29b-41d4-a716-446655440000</upload_uuid>
  <catalog_name>Номенклатура</catalog_name>
  <reference>8ca1aeb0-67d6-11e8-80ce-00155d647400</reference>
  <code>000000001</code>
  <name>Ноутбук ASUS ROG Strix G15</name>
  <attributes_xml>
    <Артикул>G513QM-HN064</Артикул>
    <ЕдиницаИзмерения>шт</ЕдиницаИзмерения>
  </attributes_xml>
  <table_parts_xml></table_parts_xml>
  <timestamp>2025-11-27T12:00:20Z</timestamp>
</catalog_item>'

# Шаг 6: Завершить выгрузку
curl -X POST http://localhost:9999/complete \
  -H "Content-Type: application/xml; charset=utf-8" \
  -d '<?xml version="1.0" encoding="UTF-8"?>
<complete>
  <upload_uuid>550e8400-e29b-41d4-a716-446655440000</upload_uuid>
  <timestamp>2025-11-27T12:10:00Z</timestamp>
</complete>'
```

#### Пример 2: Выгрузка из 1С (1C:Enterprise Script)

```bsl
// 1. Handshake
ТелоЗапроса = 
"<?xml version=""1.0"" encoding=""UTF-8""?>
|<handshake>
|  <version_1c>" + ВерсияПлатформы() + "</version_1c>
|  <config_name>" + ИмяКонфигурации() + "</config_name>
|  <timestamp>" + ТекущаяДатаСеанса() + "</timestamp>
|</handshake>";

HTTPОтвет = ОтправитьПостЗапрос("http://localhost:9999/handshake", ТелоЗапроса);
УникальныйИдентификатор = ИзвлечьЗначениеИзXML(HTTPОтвет, "upload_uuid");

// 2. Metadata
ТелоЗапроса = СформироватьXMLМетаданные(УникальныйИдентификатор);
ОтправитьПостЗапрос("http://localhost:9999/metadata", ТелоЗапроса);

// 3. Отправка констант
Для Каждого Константа Из ПолучитьСписокКонстант() Цикл
    ТелоЗапроса = СформироватьXMLКонстанты(УникальныйИдентификатор, Константа);
    ОтправитьПостЗапрос("http://localhost:9999/constant", ТелоЗапроса);
КонецЦикла;

// 4. Отправка справочников
Для Каждого Справочник Из ПолучитьСписокСправочников() Цикл
    // Регистрация справочника
    ТелоЗапроса = СформироватьXMLМетаданныхСправочника(УникальныйИдентификатор, Справочник);
    ОтправитьПостЗапрос("http://localhost:9999/catalog/meta", ТелоЗапроса);
    
    // Отправка элементов
    Выборка = Справочник.Выбрать();
    Пока Выборка.Следующий() Цикл
        ТелоЗапроса = СформироватьXMLЭлементаСправочника(УникальныйИдентификатор, Выборка);
        ОтправитьПостЗапрос("http://localhost:9999/catalog/item", ТелоЗапроса);
    КонецЦикла;
КонецЦикла;

// 5. Завершение
ТелоЗапроса = 
"<?xml version=""1.0"" encoding=""UTF-8""?>
|<complete>
|  <upload_uuid>" + УникальныйИдентификатор + "</upload_uuid>
|  <timestamp>" + ТекущаяДатаСеанса() + "</timestamp>
|</complete>";

ОтправитьПостЗапрос("http://localhost:9999/complete", ТелоЗапроса);

Сообщить("Выгрузка завершена успешно!");
```

---

## Загрузка данных в 1С

Процесс импорта данных из сервера обратно в 1С:Предприятие.

### Описание процесса

**Последовательность операций:**

```
1. Получение списка баз   → 1С запрашивает список доступных БД
2. Выбор базы             → Пользователь выбирает нужную БД
3. Import Handshake       → Получение метаданных выбранной БД
4. Загрузка констант      → Получение констант порциями
5. Загрузка справочников  → Получение элементов порциями
6. Import Complete        → Завершение импорта
```

**Что происходит на сервере:**
1. Сканируются файлы `.db` в указанных директориях
2. Для каждого файла извлекаются метаданные
3. Формируется список доступных баз
4. По запросу отправляются данные порциями с пагинацией

---

### Эндпоинты

#### 1. GET /api/1c/databases

Возвращает список всех доступных баз данных на сервере.

**Запрос:**
```bash
curl -X GET http://localhost:9999/api/1c/databases
```

**Ответ (XML):**
```xml
<?xml version="1.0" encoding="UTF-8"?>
<databases>
  <total>2</total>
  <database>
    <file_name>Выгрузка_Номенклатура_ERPWE_mpf_Unknown_2025-11-25_00-16-03.db</file_name>
    <upload_uuid>550e8400-e29b-41d4-a716-446655440000</upload_uuid>
    <config_name>ERPWE</config_name>
    <started_at>2025-11-25T00:16:03Z</started_at>
    <total_catalogs>5</total_catalogs>
    <total_constants>15</total_constants>
    <total_items>1250</total_items>
    <database_id>1</database_id>
    <client_id>1</client_id>
    <project_id>1</project_id>
    <computer_name>PC001</computer_name>
    <user_name>User1</user_name>
    <config_version>8.3.25.1257</config_version>
  </database>
  <database>
    <file_name>Выгрузка_Контрагенты_ERP_2025-11-26_10-30-00.db</file_name>
    <upload_uuid>660f9511-f3ac-52e5-b827-557766551111</upload_uuid>
    <config_name>ERP</config_name>
    <started_at>2025-11-26T10:30:00Z</started_at>
    <total_catalogs>2</total_catalogs>
    <total_constants>8</total_constants>
    <total_items>350</total_items>
  </database>
</databases>
```

**Поля ответа:**
- `file_name` - имя файла базы данных
- `upload_uuid` - UUID выгрузки
- `config_name` - имя конфигурации 1С
- `started_at` - дата и время создания выгрузки
- `total_catalogs` - количество справочников
- `total_constants` - количество констант
- `total_items` - количество элементов справочников
- `database_id`, `client_id`, `project_id` - идентификаторы (опционально)

---

#### 2. POST /api/1c/import/handshake

Начинает процесс импорта из выбранной базы. Возвращает метаданные: список справочников и количество констант.

**Кто отправляет:** 1С:Предприятие (обработка БитЭкспортер)  
**Куда:** HTTP сервер на Go  
**Где хранится:** Данные `client_info` **НЕ сохраняются**, используются только для логирования на сервере  
**Назначение:** Начало процесса импорта данных из сервера в 1С

**Тело запроса (XML):**
```xml
<?xml version="1.0" encoding="UTF-8"?>
<import_handshake>
  <db_name>Выгрузка_ПолнаяВыгрузка_ERPWE_Unknown_Unknown_2025-11-27_16-03-47.db</db_name>
  <client_info>
    <version_1c>8.3.25.1257</version_1c>
    <computer_name>PC001</computer_name>
    <user_name>User1</user_name>
  </client_info>
</import_handshake>
```

**Ответ (XML):**
```xml
<?xml version="1.0" encoding="UTF-8"?>
<import_handshake_response>
  <success>true</success>
  <upload_uuid>550e8400-e29b-41d4-a716-446655440000</upload_uuid>
  <catalogs>
    <catalog>
      <name>Номенклатура</name>
      <synonym>Номенклатура</synonym>
      <item_count>1000</item_count>
    </catalog>
    <catalog>
      <name>Контрагенты</name>
      <synonym>Контрагенты</synonym>
      <item_count>250</item_count>
    </catalog>
  </catalogs>
  <constants_count>15</constants_count>
  <message>Import handshake successful</message>
  <timestamp>2025-11-27T12:00:00Z</timestamp>
</import_handshake_response>
```

**Пример использования (curl):**

**Linux/Mac (bash):**
```bash
curl --max-time 7 -X POST http://localhost:9999/api/1c/import/handshake \
  -H "Content-Type: application/xml; charset=utf-8" \
  -d '<?xml version="1.0" encoding="UTF-8"?>
<import_handshake>
  <db_name>Выгрузка_ПолнаяВыгрузка_ERPWE_Unknown_Unknown_2025-11-27_16-03-47.db</db_name>
  <client_info>
    <version_1c>8.3.25.1257</version_1c>
    <computer_name>PC001</computer_name>
    <user_name>User1</user_name>
  </client_info>
</import_handshake>'
```

**Windows (CMD):**
```cmd
curl --max-time 7 -X POST http://localhost:9999/api/1c/import/handshake -H "Content-Type: application/xml; charset=utf-8" -d "<?xml version=\"1.0\" encoding=\"UTF-8\"?><import_handshake><db_name>Выгрузка_ПолнаяВыгрузка_ERPWE_Unknown_Unknown_2025-11-27_16-03-47.db</db_name><client_info><version_1c>8.3.25.1257</version_1c><computer_name>PC001</computer_name><user_name>User1</user_name></client_info></import_handshake>"
```

**Windows (PowerShell):**
```powershell
$body = @"
<?xml version="1.0" encoding="UTF-8"?>
<import_handshake>
  <db_name>Выгрузка_Номенклатура_ERPWE_mpf_Unknown_2025-11-25_00-16-03.db</db_name>
  <client_info>
    <version_1c>8.3.25.1257</version_1c>
    <computer_name>PC001</computer_name>
    <user_name>User1</user_name>
  </client_info>
</import_handshake>
"@

Invoke-WebRequest -Uri "http://localhost:9999/api/1c/import/handshake" `
  -Method POST `
  -ContentType "application/xml; charset=utf-8" `
  -Body $body `
  -TimeoutSec 7
```

**Примечание:**  
- `db_name` - имя файла базы данных из списка, полученного через `/api/1c/databases`
- `client_info` содержит информацию о клиенте 1С, который запрашивает импорт:
  - `version_1c` - версия платформы 1С клиента (например, "8.3.25.1257")
  - `computer_name` - имя компьютера, с которого происходит импорт (например, "PC001")
  - `user_name` - имя пользователя Windows/1С, который выполняет импорт (например, "User1")
- **Эти данные НЕ сохраняются в БД**, используются только для логирования операции импорта на сервере
- Сервер возвращает `upload_uuid` выгрузки, которую нужно использовать в дальнейших запросах (хотя фактически используется `db_name`)

---

#### 3. POST /api/1c/import/get-constants

Получает константы из базы данных с поддержкой пагинации.

**Тело запроса (XML):**
```xml
<?xml version="1.0" encoding="UTF-8"?>
<import_get_constants>
  <db_name>Выгрузка_Номенклатура_ERPWE_mpf_Unknown_2025-11-25_00-16-03.db</db_name>
  <offset>0</offset>
  <limit>1000</limit>
</import_get_constants>
```

**Параметры:**
- `db_name` (string, обязательный) - имя файла базы данных
- `offset` (int, опциональный) - смещение для пагинации (по умолчанию: 0)
- `limit` (int, опциональный) - количество записей (по умолчанию: 1000, максимум: 10000)

**Ответ (XML):**
```xml
<?xml version="1.0" encoding="UTF-8"?>
<import_constants_response>
  <success>true</success>
  <total>15</total>
  <offset>0</offset>
  <limit>1000</limit>
  <constants>
    <constant>
      <name>ОсновнаяВалюта</name>
      <synonym>Основная валюта</synonym>
      <type>СправочникСсылка.Валюты</type>
      <value>643</value>
      <created_at>2025-11-25T00:16:05Z</created_at>
    </constant>
    <constant>
      <name>ИспользоватьСкладскойУчет</name>
      <synonym>Использовать складской учет</synonym>
      <type>Булево</type>
      <value>true</value>
      <created_at>2025-11-25T00:16:05Z</created_at>
    </constant>
  </constants>
</import_constants_response>
```

**Пример использования (curl):**

**Linux/Mac (bash):**
```bash
curl --max-time 7 -X POST http://localhost:9999/api/1c/import/get-constants \
  -H "Content-Type: application/xml; charset=utf-8" \
  -d '<?xml version="1.0" encoding="UTF-8"?>
<import_get_constants>
  <db_name>Выгрузка_ПолнаяВыгрузка_ERPWE_Unknown_Unknown_2025-11-27_16-03-47.db</db_name>
  <offset>0</offset>
  <limit>1000</limit>
</import_get_constants>'
```

**Windows (CMD):**
```cmd
curl --max-time 7 -X POST http://localhost:9999/api/1c/import/get-constants -H "Content-Type: application/xml; charset=utf-8" -d "<?xml version=\"1.0\" encoding=\"UTF-8\"?><import_get_constants><db_name>Выгрузка_ПолнаяВыгрузка_ERPWE_Unknown_Unknown_2025-11-27_16-03-47.db</db_name><offset>0</offset><limit>1000</limit></import_get_constants>"
```

**Windows (PowerShell):**
```powershell
$body = @"
<?xml version="1.0" encoding="UTF-8"?>
<import_get_constants>
  <db_name>Выгрузка_ПолнаяВыгрузка_ERPWE_Unknown_Unknown_2025-11-27_16-03-47.db</db_name>
  <offset>0</offset>
  <limit>1000</limit>
</import_get_constants>
"@

Invoke-WebRequest -Uri "http://localhost:9999/api/1c/import/get-constants" `
  -Method POST `
  -ContentType "application/xml; charset=utf-8" `
  -Body $body `
  -TimeoutSec 7
```

---

#### 4. POST /api/1c/import/get-catalog

Получает элементы справочника из базы данных с поддержкой пагинации.

**Тело запроса (XML):**
```xml
<?xml version="1.0" encoding="UTF-8"?>
<import_get_catalog>
  <db_name>Выгрузка_Номенклатура_ERPWE_mpf_Unknown_2025-11-25_00-16-03.db</db_name>
  <catalog_name>Номенклатура</catalog_name>
  <offset>0</offset>
  <limit>500</limit>
</import_get_catalog>
```

**Параметры:**
- `db_name` (string, обязательный) - имя файла базы данных
- `catalog_name` (string, обязательный) - имя справочника
- `offset` (int, опциональный) - смещение для пагинации (по умолчанию: 0)
- `limit` (int, опциональный) - количество записей (по умолчанию: 500, максимум: 10000)

**Что такое offset и limit (пагинация)?**

Это параметры для **постраничной загрузки данных**. Если в справочнике 10 000 элементов, загружать их все сразу неэффективно. Пагинация позволяет загружать порциями:

- **`offset`** - с какого элемента начать (пропустить первые N элементов)
- **`limit`** - сколько элементов вернуть (максимум за один запрос)

**Пример:**  
Справочник "Номенклатура" содержит 1500 элементов. Загружаем порциями по 500:

1. **Первый запрос:** `offset=0, limit=500` → элементы с 1 по 500
2. **Второй запрос:** `offset=500, limit=500` → элементы с 501 по 1000  
3. **Третий запрос:** `offset=1000, limit=500` → элементы с 1001 по 1500

Сервер в ответе возвращает `total` (общее количество элементов), чтобы 1С понимала сколько всего запросов нужно сделать.

**Ответ (XML):**
```xml
<?xml version="1.0" encoding="UTF-8"?>
<import_catalog_response>
  <success>true</success>
  <catalog_name>Номенклатура</catalog_name>
  <total>1000</total>
  <offset>0</offset>
  <limit>500</limit>
  <items>
    <item>
      <reference>8ca1aeb0-67d6-11e8-80ce-00155d647400</reference>
      <code>000000001</code>
      <name>Ноутбук ASUS ROG Strix G15</name>
      <attributes_xml>
        <Артикул>G513QM-HN064</Артикул>
        <ЕдиницаИзмерения>шт</ЕдиницаИзмерения>
        <Цена>125000.00</Цена>
      </attributes_xml>
      <table_parts_xml>
        <ТабличнаяЧасть1>
          <Строка>
            <Поле1>Значение1</Поле1>
            <Поле2>Значение2</Поле2>
          </Строка>
        </ТабличнаяЧасть1>
      </table_parts_xml>
      <created_at>2025-11-25T00:16:10Z</created_at>
    </item>
    <item>
      <reference>9db2bfc1-78e7-22f9-91df-11266e758511</reference>
      <code>000000002</code>
      <name>Монитор Samsung Odyssey G7</name>
      <attributes_xml>
        <Артикул>LC27G75TQSIXCI</Артикул>
        <ЕдиницаИзмерения>шт</ЕдиницаИзмерения>
        <Цена>45000.00</Цена>
      </attributes_xml>
      <table_parts_xml></table_parts_xml>
      <created_at>2025-11-25T00:16:11Z</created_at>
    </item>
  </items>
</import_catalog_response>
```

**Пример использования (curl):**

**Linux/Mac (bash):**
```bash
curl --max-time 7 -X POST http://localhost:9999/api/1c/import/get-catalog \
  -H "Content-Type: application/xml; charset=utf-8" \
  -d '<?xml version="1.0" encoding="UTF-8"?>
<import_get_catalog>
  <db_name>Выгрузка_ПолнаяВыгрузка_ERPWE_Unknown_Unknown_2025-11-27_16-03-47.db</db_name>
  <catalog_name>Номенклатура</catalog_name>
  <offset>0</offset>
  <limit>500</limit>
</import_get_catalog>'
```

**Windows (CMD):**
```cmd
curl --max-time 7 -X POST http://localhost:9999/api/1c/import/get-catalog -H "Content-Type: application/xml; charset=utf-8" -d "<?xml version=\"1.0\" encoding=\"UTF-8\"?><import_get_catalog><db_name>Выгрузка_ПолнаяВыгрузка_ERPWE_Unknown_Unknown_2025-11-27_16-03-47.db</db_name><catalog_name>Номенклатура</catalog_name><offset>0</offset><limit>500</limit></import_get_catalog>"
```

**Windows (PowerShell):**
```powershell
$body = @"
<?xml version="1.0" encoding="UTF-8"?>
<import_get_catalog>
  <db_name>Выгрузка_ПолнаяВыгрузка_ERPWE_Unknown_Unknown_2025-11-27_16-03-47.db</db_name>
  <catalog_name>Номенклатура</catalog_name>
  <offset>0</offset>
  <limit>500</limit>
</import_get_catalog>
"@

Invoke-WebRequest -Uri "http://localhost:9999/api/1c/import/get-catalog" `
  -Method POST `
  -ContentType "application/xml; charset=utf-8" `
  -Body $body `
  -TimeoutSec 7
```

**Примечание:** Поля `<attributes_xml>` и `<table_parts_xml>` в ответе содержат XML-данные в экранированном виде (`&#34;` вместо `"`, `&lt;` вместо `<`, `&gt;` вместо `>`). Это корректное поведение XML - при парсинге ответа в 1С эти символы автоматически декодируются обратно в нормальные символы.

---

#### 5. POST /api/1c/import/complete

Завершает процесс импорта. Логирует операцию на сервере.

**Тело запроса (XML):**
```xml
<?xml version="1.0" encoding="UTF-8"?>
<import_complete>
  <db_name>Выгрузка_Номенклатура_ERPWE_mpf_Unknown_2025-11-25_00-16-03.db</db_name>
</import_complete>
```

**Ответ (XML):**
```xml
<?xml version="1.0" encoding="UTF-8"?>
<import_complete_response>
  <success>true</success>
  <message>Import completed successfully</message>
  <timestamp>2025-11-27T12:30:00Z</timestamp>
</import_complete_response>
```

**Пример использования (curl):**

**Linux/Mac (bash):**
```bash
curl --max-time 7 -X POST http://localhost:9999/api/1c/import/complete \
  -H "Content-Type: application/xml; charset=utf-8" \
  -d '<?xml version="1.0" encoding="UTF-8"?>
<import_complete>
  <db_name>Выгрузка_ПолнаяВыгрузка_ERPWE_Unknown_Unknown_2025-11-27_16-03-47.db</db_name>
</import_complete>'
```

**Windows (CMD):**
```cmd
curl --max-time 7 -X POST http://localhost:9999/api/1c/import/complete -H "Content-Type: application/xml; charset=utf-8" -d "<?xml version=\"1.0\" encoding=\"UTF-8\"?><import_complete><db_name>Выгрузка_ПолнаяВыгрузка_ERPWE_Unknown_Unknown_2025-11-27_16-03-47.db</db_name></import_complete>"
```

**Windows (PowerShell):**
```powershell
$body = @"
<?xml version="1.0" encoding="UTF-8"?>
<import_complete>
  <db_name>Выгрузка_ПолнаяВыгрузка_ERPWE_Unknown_Unknown_2025-11-27_16-03-47.db</db_name>
</import_complete>
"@

Invoke-WebRequest -Uri "http://localhost:9999/api/1c/import/complete" `
  -Method POST `
  -ContentType "application/xml; charset=utf-8" `
  -Body $body `
  -TimeoutSec 7
```

---

### Примеры использования

#### Пример 1: Полный цикл импорта (bash)

```bash
# Шаг 1: Получаем список доступных баз
curl -X GET http://localhost:9999/api/1c/databases

# Шаг 2: Выбираем базу и начинаем импорт
curl --max-time 7 -X POST http://localhost:9999/api/1c/import/handshake \
  -H "Content-Type: application/xml; charset=utf-8" \
  -d '<?xml version="1.0" encoding="UTF-8"?>
<import_handshake>
  <db_name>Выгрузка_Номенклатура_ERPWE_mpf_Unknown_2025-11-25_00-16-03.db</db_name>
  <client_info>
    <version_1c>8.3.25.1257</version_1c>
    <computer_name>PC001</computer_name>
    <user_name>User1</user_name>
  </client_info>
</import_handshake>'

# Шаг 3: Загружаем константы
curl --max-time 7 -X POST http://localhost:9999/api/1c/import/get-constants \
  -H "Content-Type: application/xml; charset=utf-8" \
  -d '<?xml version="1.0" encoding="UTF-8"?>
<import_get_constants>
  <db_name>Выгрузка_Номенклатура_ERPWE_mpf_Unknown_2025-11-25_00-16-03.db</db_name>
  <offset>0</offset>
  <limit>1000</limit>
</import_get_constants>'

# Шаг 4: Загружаем справочники порциями
# Для справочника "Номенклатура" - первая порция
curl --max-time 7 -X POST http://localhost:9999/api/1c/import/get-catalog \
  -H "Content-Type: application/xml; charset=utf-8" \
  -d '<?xml version="1.0" encoding="UTF-8"?>
<import_get_catalog>
  <db_name>Выгрузка_Номенклатура_ERPWE_mpf_Unknown_2025-11-25_00-16-03.db</db_name>
  <catalog_name>Номенклатура</catalog_name>
  <offset>0</offset>
  <limit>500</limit>
</import_get_catalog>'

# Для справочника "Номенклатура" - вторая порция
curl --max-time 7 -X POST http://localhost:9999/api/1c/import/get-catalog \
  -H "Content-Type: application/xml; charset=utf-8" \
  -d '<?xml version="1.0" encoding="UTF-8"?>
<import_get_catalog>
  <db_name>Выгрузка_Номенклатура_ERPWE_mpf_Unknown_2025-11-25_00-16-03.db</db_name>
  <catalog_name>Номенклатура</catalog_name>
  <offset>500</offset>
  <limit>500</limit>
</import_get_catalog>'

# Для справочника "Контрагенты"
curl --max-time 7 -X POST http://localhost:9999/api/1c/import/get-catalog \
  -H "Content-Type: application/xml; charset=utf-8" \
  -d '<?xml version="1.0" encoding="UTF-8"?>
<import_get_catalog>
  <db_name>Выгрузка_Номенклатура_ERPWE_mpf_Unknown_2025-11-25_00-16-03.db</db_name>
  <catalog_name>Контрагенты</catalog_name>
  <offset>0</offset>
  <limit>500</limit>
</import_get_catalog>'

# Шаг 5: Завершаем импорт
curl --max-time 7 -X POST http://localhost:9999/api/1c/import/complete \
  -H "Content-Type: application/xml; charset=utf-8" \
  -d '<?xml version="1.0" encoding="UTF-8"?>
<import_complete>
  <db_name>Выгрузка_Номенклатура_ERPWE_mpf_Unknown_2025-11-25_00-16-03.db</db_name>
</import_complete>'
```

#### Пример 2: Импорт в 1С (1C:Enterprise Script)

```bsl
// 1. Получаем список баз
СписокБаз = ПолучитьСписокБазССервера();

Если СписокБаз.Количество() = 0 Тогда
    Сообщить("Нет доступных баз для импорта");
    Возврат;
КонецЕсли;

// 2. Отображаем список пользователю и получаем выбор
ВыбраннаяБаза = ВыбратьБазуИзСписка(СписокБаз);

// 3. Начинаем импорт (handshake)
ТелоЗапроса = 
"<?xml version=""1.0"" encoding=""UTF-8""?>
|<import_handshake>
|  <db_name>" + ВыбраннаяБаза.ИмяФайла + "</db_name>
|  <client_info>
|    <version_1c>" + ВерсияПлатформы() + "</version_1c>
|    <computer_name>" + ИмяКомпьютера() + "</computer_name>
|    <user_name>" + ИмяПользователя() + "</user_name>
|  </client_info>
|</import_handshake>";

HTTPОтвет = ОтправитьПостЗапрос("http://localhost:9999/api/1c/import/handshake", ТелоЗапроса);
Метаданные = РазобратьXMLОтвет(HTTPОтвет);

Сообщить("Справочников: " + Метаданные.КоличествоСправочников);
Сообщить("Констант: " + Метаданные.КоличествоКонстант);

// 4. Загружаем константы
Если Метаданные.КоличествоКонстант > 0 Тогда
    ТелоЗапроса = 
    "<?xml version=""1.0"" encoding=""UTF-8""?>
    |<import_get_constants>
    |  <db_name>" + ВыбраннаяБаза.ИмяФайла + "</db_name>
    |  <offset>0</offset>
    |  <limit>1000</limit>
    |</import_get_constants>";
    
    HTTPОтвет = ОтправитьПостЗапрос("http://localhost:9999/api/1c/import/get-constants", ТелоЗапроса);
    Константы = РазобратьXMLКонстант(HTTPОтвет);
    
    Для Каждого Константа Из Константы Цикл
        ЗаписатьКонстантуВБазу(Константа);
    КонецЦикла;
КонецЕсли;

// 5. Загружаем справочники
Для Каждого ИнфоСправочника Из Метаданные.Справочники Цикл
    
    ИмяСправочника = ИнфоСправочника.Имя;
    КоличествоЭлементов = ИнфоСправочника.КоличествоЭлементов;
    
    Offset = 0;
    Limit = 500;
    
    Пока Offset < КоличествоЭлементов Цикл
        
        ТелоЗапроса = 
        "<?xml version=""1.0"" encoding=""UTF-8""?>
        |<import_get_catalog>
        |  <db_name>" + ВыбраннаяБаза.ИмяФайла + "</db_name>
        |  <catalog_name>" + ИмяСправочника + "</catalog_name>
        |  <offset>" + Строка(Offset) + "</offset>
        |  <limit>" + Строка(Limit) + "</limit>
        |</import_get_catalog>";
        
        HTTPОтвет = ОтправитьПостЗапрос("http://localhost:9999/api/1c/import/get-catalog", ТелоЗапроса);
        Элементы = РазобратьXMLЭлементов(HTTPОтвет);
        
        Для Каждого Элемент Из Элементы Цикл
            ЗаписатьЭлементСправочникаВБазу(Элемент, ИмяСправочника);
        КонецЦикла;
        
        Offset = Offset + Limit;
        Сообщить("Загружено: " + Строка(Offset) + " / " + Строка(КоличествоЭлементов));
        
    КонецЦикла;
    
    Сообщить("✓ Справочник '" + ИмяСправочника + "' загружен");
    
КонецЦикла;

// 6. Завершаем импорт
ТелоЗапроса = 
"<?xml version=""1.0"" encoding=""UTF-8""?>
|<import_complete>
|  <db_name>" + ВыбраннаяБаза.ИмяФайла + "</db_name>
|</import_complete>";

ОтправитьПостЗапрос("http://localhost:9999/api/1c/import/complete", ТелоЗапроса);

Сообщить("✓ Импорт завершен успешно!");
```

---

## API для работы с выгрузками

REST API для получения информации о выгрузках и данных из них.

### Список выгрузок

#### GET /api/uploads

Получить список всех выгрузок.

**Запрос:**
```bash
curl http://localhost:9999/api/uploads
```

**Ответ (JSON):**
```json
{
  "uploads": [
    {
      "upload_uuid": "550e8400-e29b-41d4-a716-446655440000",
      "started_at": "2024-01-15T10:30:00Z",
      "completed_at": "2024-01-15T10:35:00Z",
      "status": "completed",
      "version_1c": "8.3.25",
      "config_name": "УправлениеТорговлей",
      "total_constants": 15,
      "total_catalogs": 5,
      "total_items": 120
    }
  ],
  "total": 1
}
```

---

### Детали выгрузки

#### GET /api/uploads/{uuid}

Получить детальную информацию о выгрузке.

**Запрос:**
```bash
curl http://localhost:9999/api/uploads/550e8400-e29b-41d4-a716-446655440000
```

**Ответ (JSON):**
```json
{
  "upload_uuid": "550e8400-e29b-41d4-a716-446655440000",
  "started_at": "2024-01-15T10:30:00Z",
  "completed_at": "2024-01-15T10:35:00Z",
  "status": "completed",
  "version_1c": "8.3.25",
  "config_name": "УправлениеТорговлей",
  "total_constants": 15,
  "total_catalogs": 5,
  "total_items": 120,
  "catalogs": [
    {
      "id": 1,
      "name": "Номенклатура",
      "synonym": "Номенклатура",
      "item_count": 50,
      "created_at": "2024-01-15T10:31:00Z"
    },
    {
      "id": 2,
      "name": "Контрагенты",
      "synonym": "Контрагенты",
      "item_count": 30,
      "created_at": "2024-01-15T10:31:30Z"
    }
  ],
  "constants": [
    {
      "id": 1,
      "name": "Организация",
      "synonym": "Организация",
      "type": "Строка",
      "value": "ООО Рога и Копыта",
      "created_at": "2024-01-15T10:30:15Z"
    }
  ]
}
```

---

### Получение данных

#### GET /api/uploads/{uuid}/data

Получить данные выгрузки с фильтрацией и пагинацией.

**Query параметры:**
- `type` - тип данных: `all`, `constants`, `catalogs` (по умолчанию: `all`)
- `catalog_names` - список имен справочников через запятую
- `page` - номер страницы (по умолчанию: 1)
- `limit` - количество элементов на странице (по умолчанию: 100, максимум: 1000)

**Запрос:**
```bash
curl "http://localhost:9999/api/uploads/550e8400-e29b-41d4-a716-446655440000/data?type=catalogs&catalog_names=Номенклатура&page=1&limit=50"
```

**Ответ (XML):**
```xml
<?xml version="1.0" encoding="UTF-8"?>
<data_response>
  <upload_uuid>550e8400-e29b-41d4-a716-446655440000</upload_uuid>
  <type>catalogs</type>
  <page>1</page>
  <limit>50</limit>
  <total>883</total>
  <items>
    <item type="catalog_item" id="5133" created_at="2025-11-09T10:33:43Z">
      <catalog_item>
        <id>5133</id>
        <catalog_id>509</catalog_id>
        <catalog_name>Номенклатура</catalog_name>
        <reference>8ca1aeb0-67d6-11e8-80ce-00155d647400</reference>
        <code>000000001</code>
        <name>Ноутбук ASUS ROG Strix G15</name>
        <attributes_xml>
          <Артикул>G513QM-HN064</Артикул>
          <ЕдиницаИзмерения>шт</ЕдиницаИзмерения>
        </attributes_xml>
        <table_parts_xml></table_parts_xml>
        <created_at>2025-11-09T10:33:43Z</created_at>
      </catalog_item>
    </item>
  </items>
</data_response>
```

---

### Потоковая передача

#### GET /api/uploads/{uuid}/stream

Получить данные выгрузки в режиме потоковой передачи (Server-Sent Events).

**Query параметры:**
- `type` - тип данных: `all`, `constants`, `catalogs`
- `catalog_names` - список имен справочников через запятую

**Запрос:**
```bash
curl "http://localhost:9999/api/uploads/550e8400-e29b-41d4-a716-446655440000/stream?type=all"
```

**Ответ (SSE с XML):**
```
Content-Type: text/event-stream

data: <item type="catalog_item" id="5133" created_at="2025-11-09T10:33:43Z">...</item>

data: <item type="constant" id="1" created_at="2025-11-09T10:30:15Z">...</item>

data: <item type="complete"></item>

```

---

## Обработка ошибок

### Формат ошибок

Все ошибки возвращаются в формате JSON с соответствующим HTTP кодом.

**Формат:**
```json
{
  "error": "Описание ошибки",
  "timestamp": "2024-01-15T10:30:00Z"
}
```

### Коды состояния

| Код | Значение | Описание |
|-----|----------|----------|
| 200 | OK | Успешный запрос |
| 201 | Created | Ресурс создан |
| 400 | Bad Request | Неверный формат запроса |
| 404 | Not Found | Ресурс не найден |
| 405 | Method Not Allowed | Неверный HTTP метод |
| 500 | Internal Server Error | Внутренняя ошибка сервера |

### Примеры ошибок

**Выгрузка не найдена (404):**
```json
{
  "error": "Upload not found",
  "timestamp": "2024-01-15T10:30:00Z"
}
```

**Неверный формат запроса (400):**
```json
{
  "error": "Invalid XML format: missing required field 'upload_uuid'",
  "timestamp": "2024-01-15T10:30:00Z"
}
```

**База данных не найдена (404):**
```json
{
  "error": "database file not found: Выгрузка_Test.db",
  "timestamp": "2024-01-15T10:30:00Z"
}
```

---

## Технические детали

### CORS

API поддерживает CORS для всех источников:
- `Access-Control-Allow-Origin: *`
- `Access-Control-Allow-Methods: GET, POST, OPTIONS`
- `Access-Control-Allow-Headers: Content-Type`

**⚠️ Внимание:** В production окружении рекомендуется ограничить CORS конкретными доменами.

---

### Форматы данных

**JSON** - используется для:
- Списков выгрузок
- Детальной информации
- Ошибок API

**XML** - используется для:
- Выгрузки данных из 1С
- Загрузки данных в 1С
- Получения данных выгрузки

**Временные метки:** ISO 8601 (RFC3339), UTC  
**Кодировка:** UTF-8 для всех запросов и ответов

---

### Безопасность

**SQL инъекции:**
- Все параметры передаются через prepared statements
- Используются параметризованные запросы

**Валидация:**
- UUID проверяется на корректность формата
- Числовые параметры валидируются (min/max значения)
- Строковые параметры очищаются от пробелов

**Рекомендации:**
- Используйте HTTPS в production
- Настройте firewall для ограничения доступа
- Регулярно создавайте резервные копии БД

---

### Производительность

**Оптимизации:**
- Индексы на всех внешних ключах
- UUID выгрузки индексирован
- JOIN запросы вместо множественных запросов
- Batch обработка для потоковой передачи

**Рекомендации:**
- Для больших выгрузок используйте `/stream`
- Используйте фильтр `catalog_names` при необходимости
- Выбирайте разумный `limit` (100-500 элементов)
- Для множественных запросов используйте пагинацию

---

### Логирование

Сервер логирует:
- ✅ Создание выгрузок
- ✅ Добавление данных
- ✅ API запросы
- ✅ Ошибки

Логи включают:
- Временную метку
- Уровень (INFO, DEBUG, ERROR)
- Сообщение
- UUID выгрузки (если применимо)
- Эндпоинт

---

### Ограничения

**Размер данных:**
- Максимальный размер одного запроса: ограничен настройками HTTP сервера
- Рекомендуется использовать потоковую передачу для больших объемов

**Пагинация:**
- Максимальный `limit`: 1000 элементов
- Рекомендуемый `limit`: 100-500 элементов

**Параллельные запросы:**
- SQLite поддерживает параллельное чтение
- Запись выполняется последовательно (SQLite limitation)

**База данных:**
- Максимальный размер SQLite БД: теоретически не ограничен
- Практически ограничен размером файловой системы

---

## Приложения

### Тестовые скрипты

Созданы тестовые скрипты для проверки API:

**Windows (PowerShell):**
```powershell
.\test_1c_import_api.ps1
```

**Linux/Mac (Bash):**
```bash
chmod +x test_1c_import_api.sh
./test_1c_import_api.sh
```

Скрипты тестируют:
1. Проверку доступности сервера
2. Получение списка баз
3. Handshake импорта
4. Получение констант
5. Получение элементов справочника
6. Завершение импорта

---

### Дополнительная документация

**Руководство пользователя для импорта:**
- Файл: `IMPORT_FROM_SERVER_GUIDE.md`
- Содержит детальные инструкции по импорту данных в 1С
- Примеры кода на 1С:Предприятие
- Обработка ошибок и FAQ

---

## Контакты и поддержка

При возникновении проблем:
1. Проверьте раздел [Обработка ошибок](#обработка-ошибок)
2. Убедитесь в доступности сервера
3. Проверьте формат запросов
4. Изучите логи сервера

**Документация API:** `API_DOCUMENTATION.md`  
**Руководство по импорту:** `IMPORT_FROM_SERVER_GUIDE.md`

---

© 2025 HTTP Server для работы с выгрузками из 1С
