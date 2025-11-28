# Система обнаружения паттернов в названиях номенклатуры

## Описание

Система для алгоритмического обнаружения проблемных паттернов в названиях номенклатуры и предложения исправлений с использованием правил и AI.

## Компоненты

### 1. PatternDetector (`normalization/pattern_detector.go`)

Детектор паттернов, который обнаруживает следующие типы проблем:

- **Технические коды** (ER-00013004, ABC-12345)
- **Артикулы** (арт.123, артикул 456)
- **Размеры** (100x100, 50х50)
- **Единицы измерения** (100м, 50кг, 2.5л)
- **Лишние пробелы** (более 2 подряд)
- **Смешанный регистр** (СоСтАвЛеНнЫй)
- **Специальные символы** (!@#$%^&*)
- **Дублирующиеся слова** (молоток молоток)
- **Числа в начале/конце** (123Товар, Товар456)
- **Префиксы/суффиксы** (№123, #456, -TEST)
- **Незавершенные слова** (товар...)

### 2. PatternAIIntegrator (`normalization/pattern_ai_integrator.go`)

Интегратор паттернов с AI для предложения улучшенных исправлений:

- Обнаруживает паттерны алгоритмически
- Применяет алгоритмические исправления
- Использует AI для сложных случаев
- Предлагает финальное исправление с учетом всех факторов

### 3. API Endpoints

#### POST `/api/patterns/detect`

Обнаруживает паттерны в названии.

**Запрос:**
```json
{
  "name": "МОЛОТАК СТРОИТЕЛЬНЫЙ 500гр ER-00013004"
}
```

**Ответ:**
```json
{
  "original_name": "МОЛОТАК СТРОИТЕЛЬНЫЙ 500гр ER-00013004",
  "patterns": [
    {
      "type": "technical_code",
      "position": 30,
      "length": 11,
      "matched_text": "ER-00013004",
      "suggested_fix": "",
      "confidence": 0.95,
      "description": "Технический код в названии",
      "severity": "medium",
      "auto_fixable": true
    }
  ],
  "algorithmic_fix": "молотак строительный",
  "summary": {
    "total": 1,
    "by_type": {"technical_code": 1},
    "by_severity": {"medium": 1},
    "auto_fixable": 1
  },
  "patterns_count": 1
}
```

#### POST `/api/patterns/suggest`

Предлагает исправление с использованием паттернов и AI.

**Запрос:**
```json
{
  "name": "МОЛОТАК СТРОИТЕЛЬНЫЙ 500гр ER-00013004",
  "use_ai": true
}
```

**Ответ:**
```json
{
  "original_name": "МОЛОТАК СТРОИТЕЛЬНЫЙ 500гр ER-00013004",
  "patterns": [...],
  "algorithmic_fix": "молотак строительный",
  "ai_suggested_fix": "молоток строительный",
  "final_suggestion": "молоток строительный",
  "confidence": 0.95,
  "reasoning": "AI предложение: Исправлена опечатка 'молотак' на 'молоток', удален артикул и вес. Найдено паттернов: 3",
  "requires_review": false
}
```

#### POST `/api/patterns/test-batch`

Тестирует паттерны на выборке данных из базы.

**Запрос:**
```json
{
  "limit": 50,
  "use_ai": false,
  "table": "catalog_items",
  "column": "name"
}
```

**Ответ:**
```json
{
  "total_analyzed": 50,
  "results": [...],
  "statistics": {
    "total_patterns": 120,
    "items_with_patterns": 35,
    "items_requiring_review": 5,
    "auto_fixable_patterns": 100,
    "patterns_by_type": {
      "technical_code": 45,
      "articul": 30,
      "dimension": 20
    },
    "patterns_by_severity": {
      "medium": 80,
      "low": 40
    },
    "avg_patterns_per_item": 2.4,
    "items_with_patterns_percent": 70.0
  }
}
```

## Использование

### Через API

1. Запустите сервер:
```bash
go run main.go
```

2. Отправьте запрос:
```bash
curl -X POST http://localhost:8080/api/patterns/detect \
  -H "Content-Type: application/json" \
  -d '{"name": "МОЛОТАК СТРОИТЕЛЬНЫЙ 500гр ER-00013004"}'
```

### Через тестовый скрипт

```powershell
# Базовое тестирование (50 записей, без AI)
.\test_patterns.ps1

# С AI
.\test_patterns.ps1 -UseAI

# С указанием лимита
.\test_patterns.ps1 -Limit 100 -UseAI
```

### Через командную утилиту

```bash
# Компиляция
go build -o test_patterns.exe ./cmd/test_patterns

# Запуск
.\test_patterns.exe 1c_data.db 50
```

## Интеграция с нормализацией

Система паттернов может быть интегрирована в процесс нормализации:

```go
// Создаем детектор
detector := normalization.NewPatternDetector()

// Обнаруживаем паттерны
matches := detector.DetectPatterns(item.Name)

// Применяем исправления
fixedName := detector.ApplyFixes(item.Name, matches)

// Или используем AI интегратор
apiKey := os.Getenv("ARLIAI_API_KEY")
if apiKey != "" {
    aiNormalizer := normalization.NewAINormalizer(apiKey)
    aiIntegrator := normalization.NewPatternAIIntegrator(detector, aiNormalizer)
    
    result, err := aiIntegrator.SuggestCorrectionWithAI(item.Name)
    if err == nil {
        fixedName = result.FinalSuggestion
    }
}
```

## Расширение паттернов

Для добавления новых паттернов отредактируйте `registerDefaultPatterns()` в `pattern_detector.go`:

```go
pd.patterns = append(pd.patterns, PatternRule{
    Type:        PatternYourNewType,
    Regex:       regexp.MustCompile(`ваш_регулярный_выражение`),
    Description: "Описание паттерна",
    Severity:    "medium",
    AutoFixable: true,
    FixFunc:     func(s string, r *regexp.Regexp) string {
        // Ваша логика исправления
        return r.ReplaceAllString(s, "")
    },
    Confidence:  0.8,
})
```

## Примеры использования

### Пример 1: Обнаружение технических кодов

```go
detector := normalization.NewPatternDetector()
matches := detector.DetectPatterns("Товар ER-12345")
// Найдет: технический код "ER-12345"
```

### Пример 2: Исправление смешанного регистра

```go
detector := normalization.NewPatternDetector()
matches := detector.DetectPatterns("МоЛоТоК СтрОиТеЛьНыЙ")
fixed := detector.ApplyFixes("МоЛоТоК СтрОиТеЛьНыЙ", matches)
// Результат: "Молоток Строительный"
```

### Пример 3: Использование AI для сложных случаев

```go
aiIntegrator := normalization.NewPatternAIIntegrator(detector, aiNormalizer)
result, _ := aiIntegrator.SuggestCorrectionWithAI("МОЛОТАК СТРОИТЕЛЬНЫЙ 500гр ER-00013004")
// Результат: "молоток строительный" с исправлением опечатки
```

## Статистика и отчеты

Система предоставляет детальную статистику:

- Общее количество найденных паттернов
- Распределение по типам
- Распределение по серьезности
- Количество автоприменяемых паттернов
- Процент записей с паттернами
- Среднее количество паттернов на запись

## Примечания

- AI интеграция требует установки переменной окружения `ARLIAI_API_KEY`
- Автоприменяемые исправления имеют высокую уверенность (>0.8)
- Паттерны с низкой уверенностью требуют ручной проверки
- Система оптимизирована для работы с большими объемами данных

