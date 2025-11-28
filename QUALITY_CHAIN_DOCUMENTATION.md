# Цепочки вызовов: Поиск дубликатов и Страница качества

## 1. Цепочка вызовов для страницы качества (`/quality`)

### Фронтенд (Next.js)

#### 1.1. Инициализация страницы
```
frontend/app/quality/page.tsx
├── QualityPage() - главный компонент
│   ├── useEffect() - получение параметров из URL (tab, database)
│   ├── fetchStats() - загрузка статистики качества
│   │   └── GET /api/quality/stats?database={database}
│   │       └── frontend/app/api/quality/stats/route.ts
│   │           └── GET ${BACKEND_URL}/api/quality/stats
│   │
│   └── handleStartAnalysis() - запуск анализа качества
│       └── POST /api/quality/analyze
│           └── frontend/app/api/quality/analyze/route.ts
│               └── POST ${BACKEND_URL}/api/quality/analyze
```

#### 1.2. Компоненты табов
```
frontend/app/quality/page.tsx
├── QualityOverviewTab - таб обзора
│   └── Отображает stats из fetchStats()
│
├── QualityDuplicatesTab - таб дубликатов (см. раздел 2)
│
├── QualityViolationsTab - таб нарушений
│
└── QualitySuggestionsTab - таб предложений
```

### Бэкенд (Go)

#### 1.3. API эндпоинты
```
server/server.go
├── mux.HandleFunc("/api/quality/stats", s.handleQualityStats)
│   └── server/server.go:5849
│       └── handleQualityStats()
│           └── s.db.GetQualityStats()
│               └── database/database.go
│                   └── GetQualityStats() - SQL запрос к БД
│
└── mux.HandleFunc("/api/quality/analyze", s.handleQualityAnalyze)
    └── server/server_quality.go:602
        └── handleQualityAnalyze()
            ├── Проверка, не выполняется ли уже анализ
            ├── Открытие БД: database.NewDB(reqBody.Database)
            └── go s.runQualityAnalysis() - запуск в фоне
                └── server/server_quality.go:694
                    └── runQualityAnalysis()
                        ├── quality.NewTableAnalyzer(db)
                        ├── analyzer.AnalyzeTableForDuplicates() - шаг 1
                        ├── analyzer.AnalyzeTableForViolations() - шаг 2
                        └── analyzer.AnalyzeTableForSuggestions() - шаг 3
```

#### 1.4. Статус анализа
```
frontend/app/quality/page.tsx
└── QualityAnalysisProgress компонент
    └── Периодический опрос статуса
        └── GET /api/quality/analyze/status
            └── frontend/app/api/quality/analyze/status/route.ts
                └── GET ${BACKEND_URL}/api/quality/analyze/status
                    └── server/server_quality.go:800
                        └── handleQualityAnalyzeStatus()
                            └── Возвращает s.qualityAnalysisStatus
```

---

## 2. Цепочка вызовов для поиска дубликатов

### Фронтенд (Next.js)

#### 2.1. Загрузка списка дубликатов
```
frontend/components/quality/quality-duplicates-tab.tsx
├── QualityDuplicatesTab({ database })
│   ├── useEffect() - при монтировании или изменении database
│   │   └── fetchDuplicates()
│   │       └── GET ${API_BASE}/api/quality/duplicates?
│   │           database={database}&limit={limit}&offset={offset}&unmerged={true}
│   │           └── server/server_quality.go:358
│   │               └── handleQualityDuplicates()
│   │
│   └── useEffect() - автообновление каждые 5 секунд
│       └── Проверка статуса анализа
│           └── GET /api/quality/analyze/status
│               └── Если анализ завершен → fetchDuplicates()
```

#### 2.2. Объединение группы дубликатов
```
frontend/components/quality/quality-duplicates-tab.tsx
└── handleMergeGroup(groupId)
    └── POST ${API_BASE}/api/quality/duplicates/{groupId}/merge
        └── server/server_quality.go:532
            └── handleQualityDuplicateAction()
                └── s.normalizedDB.MarkDuplicateGroupMerged(groupID)
                    └── database/database.go
                        └── MarkDuplicateGroupMerged() - UPDATE duplicate_groups
```

### Бэкенд (Go)

#### 2.3. Получение списка дубликатов
```
server/server_quality.go:358
└── handleQualityDuplicates()
    ├── Получение параметров из query:
    │   ├── database - путь к БД
    │   ├── unmerged - фильтр (только необъединенные)
    │   ├── limit - количество на странице
    │   └── offset - смещение для пагинации
    │
    ├── Открытие БД:
    │   └── database.NewDB(databasePath) или s.normalizedDB
    │
    ├── Получение групп:
    │   └── db.GetDuplicateGroups(onlyUnmerged, limit, offset)
    │       └── database/database.go
    │           └── GetDuplicateGroups() - SQL SELECT из duplicate_groups
    │
    └── Обогащение данными элементов:
        └── Для каждой группы:
            └── SQL SELECT из normalized_data/nomenclature_items/catalog_items
                WHERE id IN (group.ItemIDs)
                └── Возврат полных данных элементов
```

#### 2.4. Анализ и поиск дубликатов (запускается через анализ качества)
```
server/server_quality.go:694
└── runQualityAnalysis()
    └── Шаг 1: Анализ дубликатов
        └── analyzer.AnalyzeTableForDuplicates()
            └── quality/table_analyzer.go:43
                └── AnalyzeTableForDuplicates()
                    ├── Подсчет общего количества записей
                    │   └── SELECT COUNT(*) FROM table WHERE name IS NOT NULL
                    │
                    └── findDuplicatesSimple()
                        └── quality/table_analyzer.go:67
                            ├── Фаза 1: Построение индекса слов
                            │   ├── Чтение записей порциями (batchSize=1000)
                            │   │   └── SELECT id, code, name, ... FROM table LIMIT ? OFFSET ?
                            │   │
                            │   ├── Для каждой записи:
                            │   │   └── tokenizeName(name)
                            │   │       └── quality/table_analyzer.go:164
                            │   │           ├── Приведение к lowercase
                            │   │           ├── Удаление пунктуации
                            │   │           ├── Разделение на слова
                            │   │           └── Фильтрация (стоп-слова, короткие слова)
                            │   │
                            │   └── Построение индексов:
                            │       ├── wordToItems: слово → множество ID записей
                            │       ├── itemWords: ID → множество слов
                            │       └── items: ID → полная запись
                            │
                            ├── Фаза 2: Поиск групп дубликатов
                            │   └── findDuplicateGroupsByWords()
                            │       └── quality/table_analyzer.go:209
                            │           ├── Проход по всем словам в wordToItems
                            │           ├── Для слов, встречающихся в ≥2 записях:
                            │           │   ├── Получение candidateIDs
                            │           │   ├── Сравнение пар записей:
                            │           │   │   └── Подсчет общих слов
                            │           │   │       └── Если общих слов ≥ 2 → группа
                            │           │   │
                            │           │   ├── Вычисление similarity_score:
                            │           │   │   └── Среднее общих слов / максимальное количество слов
                            │           │   │
                            │           │   ├── Выбор master record:
                            │           │   │   └── selectMasterRecord()
                            │           │   │       └── quality/table_analyzer.go:369
                            │           │   │           └── Запись с наибольшим количеством слов
                            │           │   │               + бонус за наличие кода
                            │           │   │               + бонус за качество
                            │           │   │
                            │           │   └── Создание DuplicateGroup
                            │           │
                            │           └── Возврат []DuplicateGroup
                            │
                            └── Фаза 3: Сохранение в БД
                                └── Для каждой группы:
                                    └── saveDuplicateGroup()
                                        └── quality/table_analyzer.go:604
                                            ├── Создание хэша группы (MD5)
                                            ├── Проверка существования:
                                            │   └── SELECT EXISTS(...) FROM duplicate_groups WHERE group_hash = ?
                                            │
                                            └── Сохранение:
                                                └── db.SaveDuplicateGroup()
                                                    └── database/database.go
                                                        └── INSERT INTO duplicate_groups
                                                            (group_hash, duplicate_type, similarity_score, 
                                                             item_ids, suggested_master_id, ...)
```

---

## 3. Детальная схема данных

### 3.1. Структуры данных

#### DuplicateGroup (Go)
```go
// normalization/duplicate_analyzer.go
type DuplicateGroup struct {
    GroupID         string
    Type            DuplicateType  // word_based, exact, semantic, phonetic
    SimilarityScore float64
    ItemIDs         []int
    Items           []DuplicateItem
    SuggestedMaster int
    Confidence      float64
    Reason          string
}
```

#### DuplicateGroup (Frontend TypeScript)
```typescript
// frontend/components/quality/quality-duplicates-tab.tsx
interface DuplicateGroup {
  id: number
  group_hash?: string
  duplicate_type?: string
  detection_method?: string
  similarity_score: number
  suggested_master_id: number
  item_count: number
  merged: boolean
  merged_at: string | null
  created_at: string
  items: DuplicateItem[]
}
```

### 3.2. Таблицы БД

#### duplicate_groups
```sql
CREATE TABLE duplicate_groups (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    group_hash TEXT NOT NULL UNIQUE,
    duplicate_type TEXT NOT NULL,  -- word_based, exact, semantic, phonetic
    similarity_score REAL,
    item_ids TEXT NOT NULL,  -- JSON array
    suggested_master_id INTEGER,
    confidence REAL,
    reason TEXT,
    merged BOOLEAN DEFAULT 0,
    merged_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
)
```

---

## 4. Алгоритм поиска дубликатов (word-based)

### Этап 1: Токенизация
```
Вход: "Станок токарный универсальный"
Выход: ["станок", "токарный", "универсальный"]
```

### Этап 2: Построение индекса
```
wordToItems = {
    "станок": {1, 5, 12},
    "токарный": {1, 5},
    "универсальный": {1, 5, 12, 20}
}

itemWords = {
    1: {"станок", "токарный", "универсальный"},
    5: {"станок", "токарный", "универсальный"},
    12: {"станок", "универсальный"},
    20: {"универсальный"}
}
```

### Этап 3: Группировка
```
Для слова "станок" (встречается в записях 1, 5, 12):
  - Сравниваем 1 и 5: общих слов = 3 → группа [1, 5]
  - Сравниваем 1 и 12: общих слов = 2 → добавляем 12 → группа [1, 5, 12]
  - Сравниваем 5 и 12: общих слов = 2 → уже в группе

Результат: DuplicateGroup {
    ItemIDs: [1, 5, 12],
    SimilarityScore: 0.85,  // среднее по всем парам
    SuggestedMaster: 1  // наибольшее количество слов
}
```

---

## 5. Поток данных при объединении группы

```
1. Пользователь нажимает "Объединить" на группе #123
   └── frontend: handleMergeGroup(123)

2. POST запрос
   └── POST /api/quality/duplicates/123/merge

3. Бэкенд обработка
   └── server: handleQualityDuplicateAction()
       └── s.normalizedDB.MarkDuplicateGroupMerged(123)
           └── UPDATE duplicate_groups 
               SET merged = 1, merged_at = CURRENT_TIMESTAMP 
               WHERE id = 123

4. Обновление фронтенда
   └── fetchDuplicates() - перезагрузка списка
       └── Группа #123 теперь помечена как merged
```

---

## 6. Ключевые файлы

### Фронтенд
- `frontend/app/quality/page.tsx` - главная страница качества
- `frontend/components/quality/quality-duplicates-tab.tsx` - таб дубликатов
- `frontend/app/api/quality/stats/route.ts` - прокси для статистики
- `frontend/app/api/quality/analyze/route.ts` - прокси для запуска анализа
- `frontend/app/api/quality/analyze/status/route.ts` - прокси для статуса

### Бэкенд
- `server/server.go` - регистрация роутов и handleQualityStats
- `server/server_quality.go` - обработчики качества:
  - `handleQualityDuplicates()` - получение списка дубликатов
  - `handleQualityDuplicateAction()` - действия с группами (merge)
  - `handleQualityAnalyze()` - запуск анализа
  - `handleQualityAnalyzeStatus()` - статус анализа
  - `runQualityAnalysis()` - фоновый анализ

### Логика анализа
- `quality/table_analyzer.go` - основной анализатор:
  - `AnalyzeTableForDuplicates()` - анализ дубликатов
  - `findDuplicatesSimple()` - упрощенный алгоритм
  - `tokenizeName()` - токенизация названий
  - `findDuplicateGroupsByWords()` - группировка по словам
  - `selectMasterRecord()` - выбор мастер-записи
  - `saveDuplicateGroup()` - сохранение в БД

### База данных
- `database/database.go` - методы работы с БД:
  - `GetDuplicateGroups()` - получение групп
  - `SaveDuplicateGroup()` - сохранение группы
  - `MarkDuplicateGroupMerged()` - пометка как объединенной
  - `GetQualityStats()` - статистика качества

---

## 7. Временная последовательность операций

### Запуск анализа качества
```
T0: Пользователь нажимает "Запустить анализ"
T1: POST /api/quality/analyze → handleQualityAnalyze()
T2: Проверка, не выполняется ли уже анализ
T3: Открытие БД
T4: go runQualityAnalysis() - запуск в фоне
T5: Возврат ответа клиенту (анализ запущен)
T6: runQualityAnalysis() начинает работу
T7: Шаг 1: AnalyzeTableForDuplicates() - поиск дубликатов
    ├── Построение индекса слов (чтение БД порциями)
    ├── Группировка дубликатов
    └── Сохранение групп в БД
T8: Шаг 2: AnalyzeTableForViolations() - поиск нарушений
T9: Шаг 3: AnalyzeTableForSuggestions() - генерация предложений
T10: Анализ завершен, статус = "completed"
```

### Просмотр дубликатов
```
T0: Пользователь открывает таб "Дубликаты"
T1: QualityDuplicatesTab монтируется
T2: useEffect() → fetchDuplicates()
T3: GET /api/quality/duplicates?database=...&unmerged=true
T4: handleQualityDuplicates() обрабатывает запрос
T5: db.GetDuplicateGroups() - SQL запрос
T6: Обогащение данными элементов (SQL SELECT для каждой группы)
T7: Возврат JSON с группами
T8: Отображение на фронтенде
T9: Автообновление каждые 5 секунд (проверка статуса анализа)
```

---

## 8. Оптимизации

### Батчинг
- Чтение записей порциями по 1000 (batchSize)
- Обновление прогресса через callback

### Индексация
- Обратный индекс: слово → множество ID записей
- Прямой индекс: ID → множество слов
- Позволяет быстро находить записи с общими словами

### Фильтрация
- Игнорирование стоп-слов ("и", "в", "на", ...)
- Игнорирование слов короче 2 символов
- Минимум 2 общих слова для считания дубликатами

### Кеширование
- Проверка существования группы по хэшу перед сохранением
- Избежание дублирования групп в БД

---

## 9. Примеры HTTP запросов и ответов

### 9.1. Получение статистики качества

**Запрос:**
```http
GET /api/quality/stats?database=/path/to/database.db HTTP/1.1
Host: localhost:9999
```

**Ответ:**
```json
{
  "total_items": 15234,
  "by_level": {
    "basic": {
      "count": 8500,
      "avg_quality": 0.65,
      "percentage": 55.8
    },
    "ai_enhanced": {
      "count": 6234,
      "avg_quality": 0.85,
      "percentage": 40.9
    },
    "benchmark": {
      "count": 500,
      "avg_quality": 0.98,
      "percentage": 3.3
    }
  },
  "average_quality": 0.76,
  "benchmark_count": 500,
  "benchmark_percentage": 3.3
}
```

### 9.2. Запуск анализа качества

**Запрос:**
```http
POST /api/quality/analyze HTTP/1.1
Host: localhost:9999
Content-Type: application/json

{
  "database": "/path/to/database.db",
  "table": "normalized_data",
  "code_column": "code",
  "name_column": "normalized_name"
}
```

**Ответ:**
```json
{
  "success": true,
  "message": "Quality analysis started",
  "table": "normalized_data"
}
```

### 9.3. Получение статуса анализа

**Запрос:**
```http
GET /api/quality/analyze/status HTTP/1.1
Host: localhost:9999
```

**Ответ (в процессе):**
```json
{
  "is_running": true,
  "progress": 45.5,
  "processed": 5000,
  "total": 11000,
  "current_step": "duplicates",
  "duplicates_found": 234,
  "violations_found": 0,
  "suggestions_found": 0,
  "error": ""
}
```

**Ответ (завершен):**
```json
{
  "is_running": false,
  "progress": 100,
  "processed": 11000,
  "total": 11000,
  "current_step": "completed",
  "duplicates_found": 234,
  "violations_found": 156,
  "suggestions_found": 89,
  "error": ""
}
```

### 9.4. Получение списка дубликатов

**Запрос:**
```http
GET /api/quality/duplicates?database=/path/to/database.db&limit=10&offset=0&unmerged=true HTTP/1.1
Host: localhost:9999
```

**Ответ:**
```json
{
  "groups": [
    {
      "id": 123,
      "group_hash": "a1b2c3d4e5f6...",
      "duplicate_type": "word_based",
      "similarity_score": 0.85,
      "suggested_master_id": 456,
      "item_count": 3,
      "merged": false,
      "merged_at": null,
      "created_at": "2025-01-15T10:30:00Z",
      "items": [
        {
          "id": 456,
          "code": "ST-001",
          "normalized_name": "Станок токарный универсальный",
          "category": "Оборудование",
          "kpved_code": "28.41.12",
          "quality_score": 0.92,
          "processing_level": "ai_enhanced",
          "merged_count": 0
        },
        {
          "id": 789,
          "code": "ST-002",
          "normalized_name": "Станок токарный универсальный",
          "category": "Оборудование",
          "kpved_code": "28.41.12",
          "quality_score": 0.88,
          "processing_level": "basic",
          "merged_count": 0
        },
        {
          "id": 1011,
          "code": "",
          "normalized_name": "Станок универсальный токарный",
          "category": "Оборудование",
          "kpved_code": "",
          "quality_score": 0.75,
          "processing_level": "basic",
          "merged_count": 0
        }
      ]
    }
  ],
  "total": 234,
  "limit": 10,
  "offset": 0
}
```

### 9.5. Объединение группы дубликатов

**Запрос:**
```http
POST /api/quality/duplicates/123/merge HTTP/1.1
Host: localhost:9999
Content-Type: application/json
```

**Ответ:**
```json
{
  "success": true,
  "message": "Duplicate group marked as merged"
}
```

---

## 10. SQL запросы

### 10.1. Получение групп дубликатов

```sql
-- Подсчет общего количества
SELECT COUNT(*) 
FROM duplicate_groups 
WHERE merged = FALSE;

-- Получение групп с пагинацией
SELECT 
    id, 
    group_hash, 
    duplicate_type, 
    similarity_score, 
    item_ids_json,
    suggested_master_id, 
    confidence, 
    reason, 
    merged, 
    merged_at,
    created_at, 
    updated_at
FROM duplicate_groups
WHERE merged = FALSE
ORDER BY similarity_score DESC, created_at DESC
LIMIT 10 OFFSET 0;
```

### 10.2. Получение данных элементов группы

```sql
-- Для таблицы normalized_data
SELECT 
    id, 
    COALESCE(code, '') as code, 
    COALESCE(normalized_name, '') as normalized_name, 
    COALESCE(category, '') as category, 
    COALESCE(kpved_code, '') as kpved_code, 
    COALESCE(processing_level, 'basic') as processing_level, 
    COALESCE(merged_count, 0) as merged_count
FROM normalized_data
WHERE id IN (456, 789, 1011);

-- Для таблицы nomenclature_items (fallback)
SELECT 
    id, 
    COALESCE(nomenclature_code, '') as code, 
    COALESCE(nomenclature_name, '') as normalized_name, 
    COALESCE(category, '') as category, 
    COALESCE(kpved_code, '') as kpved_code, 
    COALESCE(processing_level, 'basic') as processing_level, 
    0 as merged_count
FROM nomenclature_items
WHERE id IN (456, 789, 1011);
```

### 10.3. Сохранение группы дубликатов

```sql
-- Проверка существования группы
SELECT EXISTS(
    SELECT 1 
    FROM duplicate_groups 
    WHERE group_hash = 'a1b2c3d4e5f6...'
);

-- Вставка новой группы
INSERT INTO duplicate_groups (
    group_hash, 
    duplicate_type, 
    similarity_score, 
    item_ids_json,
    suggested_master_id, 
    confidence, 
    reason, 
    created_at, 
    updated_at
) VALUES (
    'a1b2c3d4e5f6...',
    'word_based',
    0.85,
    '[456, 789, 1011]',
    456,
    0.85,
    'Общие слова (3): станок, токарный, универсальный',
    '2025-01-15 10:30:00',
    '2025-01-15 10:30:00'
);
```

### 10.4. Пометка группы как объединенной

```sql
UPDATE duplicate_groups
SET 
    merged = TRUE, 
    merged_at = '2025-01-15 11:00:00', 
    updated_at = '2025-01-15 11:00:00'
WHERE id = 123;
```

### 10.5. Анализ дубликатов - чтение записей порциями

```sql
-- Подсчет записей для анализа
SELECT COUNT(*) 
FROM normalized_data 
WHERE normalized_name IS NOT NULL AND normalized_name != '';

-- Чтение порциями (batchSize=1000)
SELECT 
    id, 
    code, 
    normalized_name, 
    COALESCE(category, '') as category,
    COALESCE(kpved_code, '') as kpved_code,
    COALESCE(processing_level, 'basic') as processing_level,
    COALESCE(merged_count, 0) as merged_count
FROM normalized_data
WHERE normalized_name IS NOT NULL AND normalized_name != ''
ORDER BY id
LIMIT 1000 OFFSET 0;
```

---

## 11. Диаграммы последовательности

### 11.1. Запуск анализа качества

```
Пользователь          Frontend              Backend              TableAnalyzer         Database
    |                    |                      |                       |                    |
    |--[Нажатие]-------->|                      |                       |                    |
    |                    |--[POST /api/quality/analyze]--------------->|                    |
    |                    |                      |                       |                    |
    |                    |                      |--[Проверка статуса]-->|                    |
    |                    |                      |<--[Не выполняется]---|                    |
    |                    |                      |                       |                    |
    |                    |                      |--[NewDB()]-------------------------------->|
    |                    |                      |<--[DB connection]--------------------------|
    |                    |                      |                       |                    |
    |                    |                      |--[go runQualityAnalysis()]                |
    |                    |                      |                       |                    |
    |                    |<--[200 OK]-----------|                       |                    |
    |<--[Ответ]----------|                      |                       |                    |
    |                    |                      |                       |                    |
    |                    |                      |--[NewTableAnalyzer()]---------------------->|
    |                    |                      |                       |                    |
    |                    |                      |--[AnalyzeTableForDuplicates()]------------>|
    |                    |                      |                       |--[COUNT(*)-------->|
    |                    |                      |                       |<--[total=11000]----|
    |                    |                      |                       |                    |
    |                    |                      |                       |--[SELECT LIMIT 1000]|
    |                    |                      |                       |<--[1000 records]--|
    |                    |                      |                       |                    |
    |                    |                      |                       |[Токенизация]       |
    |                    |                      |                       |[Построение индекса]|
    |                    |                      |                       |                    |
    |                    |                      |                       |[Группировка]       |
    |                    |                      |                       |                    |
    |                    |                      |                       |--[INSERT groups]--->|
    |                    |                      |                       |<--[OK]-------------|
    |                    |                      |                       |                    |
    |                    |--[GET /status]------>|                       |                    |
    |                    |<--[progress: 45%]---|                       |                    |
    |                    |                      |                       |                    |
    |                    |                      |--[Анализ завершен]---->|                    |
    |                    |<--[status: completed]|                       |                    |
```

### 11.2. Просмотр дубликатов

```
Пользователь          Frontend              Backend              Database
    |                    |                      |                    |
    |--[Открытие таба]-->|                      |                    |
    |                    |--[useEffect()]        |                    |
    |                    |                      |                    |
    |                    |--[GET /api/quality/duplicates?database=...&unmerged=true&limit=10&offset=0]-->|
    |                    |                      |                    |
    |                    |                      |--[GetDuplicateGroups()]------------------>|
    |                    |                      |                    |--[SELECT COUNT(*)]->|
    |                    |                      |                    |<--[total=234]-------|
    |                    |                      |                    |                    |
    |                    |                      |                    |--[SELECT groups]--->|
    |                    |                      |                    |<--[10 groups]-------|
    |                    |                      |<--[groups, total]--|                    |
    |                    |                      |                    |                    |
    |                    |                      |--[Для каждой группы: SELECT items]------>|
    |                    |                      |                    |<--[items data]------|
    |                    |                      |                    |                    |
    |                    |<--[JSON с группами]--|                    |                    |
    |                    |                      |                    |                    |
    |                    |--[Отображение]       |                    |                    |
    |<--[UI обновлен]----|                      |                    |                    |
    |                    |                      |                    |                    |
    |                    |--[Автообновление каждые 5 сек]            |                    |
    |                    |--[GET /status]------>|                    |                    |
    |                    |<--[status]-----------|                    |                    |
```

### 11.3. Объединение группы дубликатов

```
Пользователь          Frontend              Backend              Database
    |                    |                      |                    |
    |--[Клик "Объединить"]|                      |                    |
    |                    |                      |                    |
    |                    |--[POST /api/quality/duplicates/123/merge]-->|
    |                    |                      |                    |
    |                    |                      |--[MarkDuplicateGroupMerged(123)]-------->|
    |                    |                      |                    |--[UPDATE SET merged=TRUE]->|
    |                    |                      |                    |<--[OK]-------------|
    |                    |                      |<--[OK]-------------|                    |
    |                    |<--[200 OK]-----------|                    |                    |
    |                    |                      |                    |                    |
    |                    |--[fetchDuplicates()] |                    |                    |
    |                    |--[GET /duplicates]-->|                    |                    |
    |                    |                      |--[GetDuplicateGroups()]------------------>|
    |                    |                      |                    |--[SELECT]---------->|
    |                    |                      |                    |<--[groups]---------|
    |                    |<--[JSON]-------------|                    |                    |
    |                    |                      |                    |                    |
    |                    |--[Обновление UI]     |                    |                    |
    |<--[Группа помечена как merged]|          |                    |                    |
```

---

## 12. Детали реализации

### 12.1. Токенизация названий

```go
// quality/table_analyzer.go:164
func (ta *TableAnalyzer) tokenizeName(name string) []string {
    // 1. Приведение к lowercase
    name = strings.ToLower(name)
    
    // 2. Удаление пунктуации (regexp)
    reg := regexp.MustCompile(`[^\p{L}\p{N}\s]+`)
    name = reg.ReplaceAllString(name, " ")
    
    // 3. Разделение по пробелам
    words := strings.Fields(name)
    
    // 4. Фильтрация:
    //    - Слова короче 2 символов
    //    - Стоп-слова: "и", "в", "на", "для", "с", "по", ...
    //    - Слова без букв (только цифры)
    
    return filtered
}
```

**Пример:**
```
Вход:  "Станок токарный, универсальный (для металла)"
Выход: ["станок", "токарный", "универсальный", "металла"]
```

### 12.2. Вычисление similarity_score

```go
// quality/table_analyzer.go:284-318
// Для каждой пары записей в группе:
commonWords := количество_общих_слов
maxWords := max(len(words1), len(words2))
similarity = commonWords / maxWords

// Среднее по всем парам в группе
avgSimilarity = sum(all_pair_similarities) / number_of_pairs
```

**Пример:**
```
Запись 1: ["станок", "токарный", "универсальный"] (3 слова)
Запись 2: ["станок", "токарный"] (2 слова)
Запись 3: ["станок", "универсальный"] (2 слова)

Пары:
  1-2: common=2, max=3 → similarity = 2/3 = 0.67
  1-3: common=2, max=3 → similarity = 2/3 = 0.67
  2-3: common=1, max=2 → similarity = 1/2 = 0.50

avgSimilarity = (0.67 + 0.67 + 0.50) / 3 = 0.61
```

### 12.3. Выбор master record

```go
// quality/table_analyzer.go:369-401
func (ta *TableAnalyzer) selectMasterRecord(
    items []normalization.DuplicateItem,
    itemWords map[int]map[string]bool,
) int {
    bestID := items[0].ID
    bestScore := 0.0
    
    for _, item := range items {
        words := itemWords[item.ID]
        wordCount := len(words)
        
        score := float64(wordCount)  // Базовый счет
        
        if item.Code != "" {
            score += 0.5  // Бонус за наличие кода
        }
        
        score += item.QualityScore  // Бонус за качество
        
        if score > bestScore {
            bestScore = score
            bestID = item.ID
        }
    }
    
    return bestID
}
```

**Пример:**
```
Запись 1: 3 слова, код="ST-001", quality=0.92 → score = 3 + 0.5 + 0.92 = 4.42
Запись 2: 2 слова, код="ST-002", quality=0.88 → score = 2 + 0.5 + 0.88 = 3.38
Запись 3: 2 слова, код="", quality=0.75 → score = 2 + 0 + 0.75 = 2.75

Master: Запись 1 (наибольший score)
```

### 12.4. Создание хэша группы

```go
// quality/table_analyzer.go:610-612
itemIDsJSON, _ := json.Marshal(group.ItemIDs)
hash := fmt.Sprintf("%x", md5.Sum([]byte(
    fmt.Sprintf("%s-%s", group.Type, string(itemIDsJSON))
)))
```

**Пример:**
```
Type: "word_based"
ItemIDs: [456, 789, 1011]
JSON: "[456,789,1011]"
Input: "word_based-[456,789,1011]"
MD5: "a1b2c3d4e5f6789012345678901234ab"
Hash: "a1b2c3d4e5f6789012345678901234ab"
```

---

## 13. Обработка ошибок

### 13.1. Ошибки на фронтенде

```typescript
// frontend/components/quality/quality-duplicates-tab.tsx
try {
    const response = await fetch(...)
    if (!response.ok) {
        throw new Error('Failed to fetch duplicates')
    }
    const data = await response.json()
    setGroups(data.groups || [])
} catch (err) {
    setError(err instanceof Error ? err.message : 'Unknown error')
    // Отображение Alert с ошибкой
}
```

### 13.2. Ошибки на бэкенде

```go
// server/server_quality.go:401-405
groups, total, err := db.GetDuplicateGroups(onlyUnmerged, limit, offset)
if err != nil {
    log.Printf("Error getting duplicate groups: %v", err)
    s.writeJSONError(w, fmt.Sprintf("Failed to get duplicate groups: %v", err), 
                     http.StatusInternalServerError)
    return
}
```

### 13.3. Ошибки при анализе

```go
// server/server_quality.go:726-730
if err != nil {
    s.qualityAnalysisMutex.Lock()
    s.qualityAnalysisStatus.Error = fmt.Sprintf("Duplicate analysis failed: %v", err)
    s.qualityAnalysisMutex.Unlock()
    return
}
```

---

## 14. Производительность

### 14.1. Метрики анализа

- **Скорость обработки**: ~1000-5000 записей/сек (зависит от длины названий)
- **Память**: O(n + m), где n - количество записей, m - количество уникальных слов
- **Время поиска дубликатов**: O(n²) в худшем случае, но оптимизировано через индексацию

### 14.2. Оптимизации

1. **Батчинг**: Чтение записей порциями по 1000
2. **Индексация**: O(1) поиск записей по слову
3. **Ранний выход**: Пропуск слов, встречающихся только в одной записи
4. **Кеширование**: Проверка существования группы перед сохранением

### 14.3. Масштабируемость

- Для 10,000 записей: ~2-5 секунд
- Для 100,000 записей: ~30-60 секунд
- Для 1,000,000 записей: ~5-10 минут (рекомендуется разбить на части)

---

## 15. Тестирование

### 15.1. Unit тесты

```go
// quality/table_analyzer_test.go (пример)
func TestTokenizeName(t *testing.T) {
    ta := NewTableAnalyzer(nil)
    result := ta.tokenizeName("Станок токарный, универсальный")
    expected := []string{"станок", "токарный", "универсальный"}
    assert.Equal(t, expected, result)
}
```

### 15.2. Интеграционные тесты

```bash
# Тест получения дубликатов
curl -X GET "http://localhost:9999/api/quality/duplicates?database=test.db&limit=10&offset=0&unmerged=true"

# Тест объединения группы
curl -X POST "http://localhost:9999/api/quality/duplicates/123/merge"
```

---

## 16. Будущие улучшения

1. **Параллельная обработка**: Использование goroutines для обработки батчей
2. **Инкрементальный анализ**: Анализ только новых/измененных записей
3. **Машинное обучение**: Использование ML для улучшения точности поиска
4. **Кеширование результатов**: Сохранение индексов в БД для быстрого переиспользования
5. **Веб-воркеры**: Выполнение анализа в отдельном процессе/воркере

