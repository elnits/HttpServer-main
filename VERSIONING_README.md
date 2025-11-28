# Многостадийное версионирование и система классификации

## Обзор

Реализована система многостадийного версионирования нормализации с полной историей изменений и система классификации товаров с гибкой настройкой глубины категорий.

## Компоненты

### 1. Многостадийное версионирование

#### База данных
- **Таблицы**: `normalization_sessions`, `normalization_stages`
- Каждая сессия нормализации сохраняет все стадии преобразования
- Каждая стадия содержит входное и выходное название, примененные паттерны, AI контекст

#### API Endpoints
- `POST /api/normalization/start` - начать сессию нормализации
- `POST /api/normalization/apply-patterns` - применить алгоритмические паттерны
- `POST /api/normalization/apply-ai` - применить AI коррекцию (с опцией чата)
- `GET /api/normalization/history?session_id=X` - получить историю сессии
- `POST /api/normalization/revert` - откат к указанной стадии

#### Использование

```go
// Создаем пайплайн
pipeline := normalization.NewVersionedNormalizationPipeline(db, patternDetector, aiIntegrator)

// Начинаем сессию
pipeline.StartSession(catalogItemID, "МОЛОТАК СТРОИТЕЛЬНЫЙ 500гр ER-00013004")

// Применяем паттерны
pipeline.ApplyPatterns()
// Результат: "молотак строительный"

// Применяем AI коррекцию
pipeline.ApplyAICorrection(false)
// Результат: "молоток строительный" (исправлена опечатка)

// Получаем историю
history, _ := pipeline.GetHistory()

// Откатываемся к первой стадии
pipeline.RevertToStage(history[0].ID)
```

### 2. Система классификации

#### Стратегии свертки
- **top_priority** - сохраняет верхние уровни, объединяет нижние
- **bottom_priority** - сохраняет нижние уровни, объединяет верхние
- **mixed_priority** - сохраняет первый и последний уровни

#### Примеры свертки

Исходный путь: `["Оборудование", "Электроинструменты", "Дрели", "Беспроводные дрели"]`

**Стратегия "top" (depth=2):**
- Level 1: "Оборудование"
- Level 2: "Электроинструменты / Дрели / Беспроводные дрели"

**Стратегия "bottom" (depth=2):**
- Level 1: "Оборудование / Электроинструменты / Дрели"
- Level 2: "Беспроводные дрели"

#### API Endpoints
- `POST /api/classification/classify` - классифицировать товар
- `GET /api/classification/strategies` - получить список стратегий
- `POST /api/classification/strategies/configure` - настроить стратегию

#### Использование

```go
// Создаем AI классификатор
aiClassifier := classification.NewAIClassifier(apiKey, model)

// Создаем менеджер стратегий
strategyManager := classification.NewStrategyManager()

// Создаем стадию классификации
classificationStage := normalization.NewClassificationStage(aiClassifier, strategyManager)

// Применяем классификацию
classificationStage.Process(pipeline, "top_priority")
```

## Интеграция

### Полный workflow

```go
// 1. Начинаем сессию
pipeline.StartSession(itemID, originalName)

// 2. Применяем паттерны
pipeline.ApplyPatterns()

// 3. Применяем AI коррекцию
pipeline.ApplyAICorrection(true) // с чат-контекстом

// 4. Применяем классификацию
classificationStage.Process(pipeline, "top_priority")

// 5. Получаем результаты
currentName := pipeline.GetCurrentName()
categoryFolded := pipeline.GetMetadata("category_folded")
```

## База данных

### Таблицы версионирования

**normalization_sessions:**
- `id`, `catalog_item_id`, `original_name`, `current_name`
- `stages_count`, `status`, `created_at`, `updated_at`

**normalization_stages:**
- `id`, `session_id`, `stage_type`, `stage_name`
- `input_name`, `output_name`, `applied_patterns` (JSON)
- `ai_context` (JSON), `category_original` (JSON), `category_folded` (JSON)
- `classification_strategy`, `confidence`, `status`, `created_at`

### Таблицы классификации

**category_classifiers:**
- `id`, `name`, `description`, `max_depth`
- `tree_structure` (JSON), `is_active`

**folding_strategies:**
- `id`, `name`, `description`, `strategy_config` (JSON)
- `client_id`, `is_default`

### Расширение catalog_items

Добавлены поля:
- `category_original` (TEXT/JSON)
- `category_level1` - `category_level5` (TEXT)
- `classification_confidence` (REAL)
- `classification_strategy` (TEXT)

## Файлы

### База данных
- `database/schema_versions.go` - создание таблиц версионирования
- `database/db_versions.go` - методы работы с версиями
- `database/db_classification.go` - методы работы с классификацией

### Нормализация
- `normalization/versioned_pipeline.go` - версионированный пайплайн
- `normalization/classification_stage.go` - стадия классификации

### Классификация
- `classification/folding_strategies.go` - стратегии свертки
- `classification/ai_classifier.go` - AI классификатор

### API
- `server/server_versions.go` - endpoints версионирования
- `server/server_classification.go` - endpoints классификации

## Примеры запросов

### Начать нормализацию
```bash
curl -X POST http://localhost:8080/api/normalization/start \
  -H "Content-Type: application/json" \
  -d '{"item_id": 123, "original_name": "МОЛОТАК СТРОИТЕЛЬНЫЙ 500гр ER-00013004"}'
```

### Применить паттерны
```bash
curl -X POST http://localhost:8080/api/normalization/apply-patterns \
  -H "Content-Type: application/json" \
  -d '{"session_id": 1}'
```

### Применить AI коррекцию
```bash
curl -X POST http://localhost:8080/api/normalization/apply-ai \
  -H "Content-Type: application/json" \
  -d '{"session_id": 1, "use_chat": true}'
```

### Получить историю
```bash
curl http://localhost:8080/api/normalization/history?session_id=1
```

### Классифицировать товар
```bash
curl -X POST http://localhost:8080/api/classification/classify \
  -H "Content-Type: application/json" \
  -d '{"session_id": 1, "strategy_id": "top_priority"}'
```

## Особенности

- **Полная отслеживаемость**: Каждая стадия сохраняется с полным контекстом
- **Откат**: Возможность вернуться к любой предыдущей стадии
- **Чат-контекст**: AI может использовать историю предыдущих стадий
- **Гибкая классификация**: Настраиваемые стратегии свертки для каждого клиента
- **AI интеграция**: Автоматическое определение категорий с помощью AI

