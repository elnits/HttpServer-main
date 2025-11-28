# Цепочка вызовов для экспорта кода 1С в XML

## Обзор процесса

Процесс экспорта кода обработки 1С в XML файл проходит через три уровня:
1. **Frontend (React/Next.js)** - пользовательский интерфейс
2. **Next.js API Route** - прокси-слой между фронтендом и бэкендом
3. **Backend (Go)** - генерация XML из исходных файлов

---

## 1. Frontend: Инициация запроса

### Файл: `frontend/app/page.tsx`

**Точка входа:** Пользователь нажимает кнопку "Скачать XML обработки"

```typescript
// Строка 58-94: Обработчик клика на кнопку
const handleDownloadXML = async () => {
  setDownloadingXML(true)  // Устанавливаем состояние загрузки
  try {
    // Шаг 1: Отправка GET запроса на Next.js API route
    const response = await fetch('/api/1c/processing/xml')
    
    if (!response.ok) {
      throw new Error('Не удалось загрузить XML файл')
    }

    // Шаг 2: Извлечение имени файла из заголовка Content-Disposition
    const contentDisposition = response.headers.get('Content-Disposition')
    let filename = `1c_processing_export_${new Date().toISOString().split('T')[0].replace(/-/g, '')}.xml`

    if (contentDisposition) {
      const filenameMatch = contentDisposition.match(/filename="?([^"]+)"?/)
      if (filenameMatch) {
        filename = filenameMatch[1]
      }
    }

    // Шаг 3: Преобразование ответа в Blob и создание ссылки для скачивания
    const blob = await response.blob()
    const url = window.URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = url
    link.download = filename
    document.body.appendChild(link)
    link.click()  // Программный клик для скачивания
    document.body.removeChild(link)
    window.URL.revokeObjectURL(url)  // Освобождение памяти
  } catch (error) {
    console.error('Failed to download XML:', error)
    alert('Ошибка при скачивании XML файла. Проверьте подключение к серверу.')
  } finally {
    setDownloadingXML(false)  // Сброс состояния загрузки
  }
}
```

**Кнопка на странице:**
```typescript
// Строка ~200-220: Карточка с кнопкой
<Button 
  onClick={handleDownloadXML}
  disabled={downloadingXML}
  className="w-full"
>
  {downloadingXML ? (
    <>
      <RefreshCw className="mr-2 h-4 w-4 animate-spin" />
      Генерация XML...
    </>
  ) : (
    <>
      <Download className="mr-2 h-4 w-4" />
      Скачать XML обработки
    </>
  )}
</Button>
```

---

## 2. Next.js API Route: Проксирование запроса

### Файл: `frontend/app/api/1c/processing/xml/route.ts`

**Назначение:** Проксирует запрос от фронтенда к Go бэкенду и передает ответ обратно

```typescript
// Строка 5-42: GET обработчик Next.js API route
export async function GET() {
  try {
    // Шаг 1: Получение URL бэкенда из переменных окружения
    const BACKEND_URL = process.env.BACKEND_URL || 'http://localhost:9999'
    
    // Шаг 2: Отправка GET запроса на Go бэкенд
    const response = await fetch(`${BACKEND_URL}/api/1c/processing/xml`, {
      cache: 'no-store',  // Отключаем кеширование для получения актуальной версии
    })

    // Шаг 3: Проверка успешности ответа
    if (!response.ok) {
      const errorText = await response.text().catch(() => 'Unknown error')
      return NextResponse.json(
        { error: `Failed to fetch XML: ${errorText}` },
        { status: response.status }
      )
    }

    // Шаг 4: Получение XML как Blob из ответа бэкенда
    const blob = await response.blob()
    
    // Шаг 5: Извлечение заголовков из ответа бэкенда
    const contentType = response.headers.get('Content-Type') || 'application/xml; charset=utf-8'
    const contentDisposition = response.headers.get('Content-Disposition') || 
      `attachment; filename="1c_processing_export_${new Date().toISOString().split('T')[0].replace(/-/g, '')}.xml"`

    // Шаг 6: Возврат XML файла с правильными заголовками клиенту
    return new NextResponse(blob, {
      status: 200,
      headers: {
        'Content-Type': contentType,
        'Content-Disposition': contentDisposition,
      },
    })
  } catch (error) {
    console.error('Error fetching 1C processing XML:', error)
    return NextResponse.json(
      { error: `Failed to connect to backend: ${error instanceof Error ? error.message : 'Unknown error'}` },
      { status: 500 }
    )
  }
}
```

---

## 3. Backend (Go): Генерация XML

### Файл: `server/server.go`

### 3.1. Регистрация маршрута

```go
// Строка 202: Регистрация endpoint в HTTP роутере
mux.HandleFunc("/api/1c/processing/xml", s.handle1CProcessingXML)
```

### 3.2. Основная функция обработки

**Функция:** `handle1CProcessingXML` (строки 6473-6636)

#### Шаг 1: Проверка метода запроса
```go
// Строка 6475-6478
if r.Method != http.MethodGet {
    http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
    return
}
```

#### Шаг 2: Получение рабочей директории
```go
// Строка 6480-6491
workDir, err := os.Getwd()
if err != nil {
    // Логирование ошибки и возврат HTTP 500
    s.log(LogEntry{...})
    http.Error(w, fmt.Sprintf("Failed to get working directory: %v", err), http.StatusInternalServerError)
    return
}
```

#### Шаг 3: Чтение основного модуля обработки
```go
// Строка 6493-6505
modulePath := filepath.Join(workDir, "1c_processing", "Module", "Module.bsl")
moduleCode, err := os.ReadFile(modulePath)
if err != nil {
    // Логирование ошибки и возврат HTTP 500
    // Файл обязателен, без него генерация невозможна
}
```

#### Шаг 4: Чтение расширений модуля (опционально)
```go
// Строка 6507-6518
extensionsPath := filepath.Join(workDir, "1c_module_extensions.bsl")
extensionsCode, err := os.ReadFile(extensionsPath)
if err != nil {
    // Файл опционален, используем пустую строку
    extensionsCode = []byte("")
    s.log(LogEntry{Level: "WARN", ...})  // Предупреждение, но не ошибка
}
```

#### Шаг 5: Чтение экспортируемых функций (опционально)
```go
// Строка 6520-6531
exportFunctionsPath := filepath.Join(workDir, "1c_export_functions.txt")
exportFunctionsCode, err := os.ReadFile(exportFunctionsPath)
if err != nil {
    // Файл опционален, используем пустую строку
    exportFunctionsCode = []byte("")
    s.log(LogEntry{Level: "WARN", ...})
}
```

#### Шаг 6: Объединение кода модулей
```go
// Строка 6533-6556
fullModuleCode := string(moduleCode)  // Начинаем с основного модуля

// Добавляем область "ПрограммныйИнтерфейс" из export_functions
if len(exportFunctionsCode) > 0 {
    exportCodeStr := string(exportFunctionsCode)
    startMarker := "#Область ПрограммныйИнтерфейс"
    endMarker := "#КонецОбласти"
    
    startPos := strings.Index(exportCodeStr, startMarker)
    if startPos >= 0 {
        endPos := strings.Index(exportCodeStr[startPos+len(startMarker):], endMarker)
        if endPos >= 0 {
            endPos += startPos + len(startMarker)
            programInterfaceCode := exportCodeStr[startPos : endPos+len(endMarker)]
            fullModuleCode += "\n\n" + programInterfaceCode
        }
    }
}

// Добавляем расширения
if len(extensionsCode) > 0 {
    fullModuleCode += "\n\n" + string(extensionsCode)
}
```

#### Шаг 7: Экранирование специальных символов для XML
```go
// Строка 6558-6561
// Экранируем последовательность "]]>" которая может разорвать CDATA секцию
escapedModuleCode := strings.ReplaceAll(fullModuleCode, "]]>", "]]]]><![CDATA[>")
```

#### Шаг 8: Генерация UUID для обработки
```go
// Строка 6563-6564
// Генерируем новый UUID при каждом запросе для уникальности обработки
processingUUID := strings.ToUpper(strings.ReplaceAll(uuid.New().String(), "-", ""))
```

#### Шаг 9: Подготовка кода формы
```go
// Строка 6566-6583
// Статический код формы обработки (процедура инициализации)
formModuleCode := `&НаКлиенте
Процедура ПриСозданииНаСервере(Отказ, СтандартнаяОбработка)
    // Устанавливаем значения по умолчанию
    Если Объект.АдресСервера = "" Тогда
        Объект.АдресСервера = "http://localhost:9999";
    КонецЕсли;
    // ... остальной код
КонецПроцедуры`
```

#### Шаг 10: Формирование XML структуры
```go
// Строка 6585-6612
xmlContent := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<MetaDataObject xmlns="http://v8.1c.ru/8.3/MDClasses" ... version="2.12">
  <DataProcessor>
    <uuid>%s</uuid>
    <name>ВыгрузкаДанныхВСервис</name>
    <synonym>
      <key>ru</key>
      <value>Выгрузка данных в сервис нормализации</value>
    </synonym>
    <comment>Обработка для выгрузки данных из 1С в сервис нормализации и анализа через HTTP</comment>
    <module>
      <text><![CDATA[%s]]></text>
    </module>
    <forms>
      <form>
        <name>Форма</name>
        <synonym>
          <key>ru</key>
          <value>Форма</value>
        </synonym>
        <module>
          <text><![CDATA[%s]]></text>
        </module>
      </form>
    </forms>
  </DataProcessor>
</MetaDataObject>`, processingUUID, escapedModuleCode, formModuleCode)
```

#### Шаг 11: Установка HTTP заголовков
```go
// Строка 6614-6617
w.Header().Set("Content-Type", "application/xml; charset=utf-8")
w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"1c_processing_export_%s.xml\"", 
    time.Now().Format("20060102_150405")))
w.WriteHeader(http.StatusOK)
```

#### Шаг 12: Отправка XML клиенту
```go
// Строка 6619-6628
if _, err := w.Write([]byte(xmlContent)); err != nil {
    s.log(LogEntry{
        Timestamp: time.Now(),
        Level:     "ERROR",
        Message:   fmt.Sprintf("Failed to write XML response: %v", err),
        Endpoint:  "/api/1c/processing/xml",
    })
    return
}
```

#### Шаг 13: Логирование успешной генерации
```go
// Строка 6630-6635
s.log(LogEntry{
    Timestamp: time.Now(),
    Level:     "INFO",
    Message:   fmt.Sprintf("Generated 1C processing XML (UUID: %s, module size: %d chars)", 
        processingUUID, len(fullModuleCode)),
    Endpoint:  "/api/1c/processing/xml",
})
```

---

## Диаграмма потока данных

```
┌─────────────────────────────────────────────────────────────────┐
│ 1. FRONTEND (React/Next.js)                                      │
│    frontend/app/page.tsx                                         │
│                                                                  │
│    Пользователь нажимает кнопку                                 │
│    ↓                                                             │
│    handleDownloadXML()                                          │
│    ↓                                                             │
│    fetch('/api/1c/processing/xml')                              │
└─────────────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────────┐
│ 2. NEXT.JS API ROUTE                                             │
│    frontend/app/api/1c/processing/xml/route.ts                 │
│                                                                  │
│    GET /api/1c/processing/xml                                   │
│    ↓                                                             │
│    fetch(BACKEND_URL + '/api/1c/processing/xml')                │
│    ↓                                                             │
│    Получение Blob и заголовков                                  │
│    ↓                                                             │
│    NextResponse(blob, headers)                                  │
└─────────────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────────┐
│ 3. BACKEND (Go)                                                  │
│    server/server.go                                              │
│                                                                  │
│    handle1CProcessingXML()                                      │
│    ↓                                                             │
│    os.Getwd() → получение рабочей директории                    │
│    ↓                                                             │
│    os.ReadFile('1c_processing/Module/Module.bsl')               │
│    os.ReadFile('1c_module_extensions.bsl') [опционально]       │
│    os.ReadFile('1c_export_functions.txt') [опционально]        │
│    ↓                                                             │
│    Объединение кода:                                            │
│      - Основной модуль                                          │
│      - Область "ПрограммныйИнтерфейс" из export_functions      │
│      - Расширения модуля                                        │
│    ↓                                                             │
│    Экранирование XML (]]> → ]]]]><![CDATA[>)                   │
│    ↓                                                             │
│    uuid.New() → генерация UUID                                  │
│    ↓                                                             │
│    fmt.Sprintf() → формирование XML структуры                    │
│    ↓                                                             │
│    w.Header().Set() → установка заголовков                      │
│    w.Write() → отправка XML                                    │
└─────────────────────────────────────────────────────────────────┘
                            ↓
                    XML файл скачивается
                    в браузере пользователя
```

---

## Структура файлов, используемых при генерации

```
E:\HttpServer\
├── 1c_processing/
│   └── Module/
│       └── Module.bsl              ← Основной модуль обработки (обязателен)
├── 1c_module_extensions.bsl       ← Расширения модуля (опционально)
└── 1c_export_functions.txt          ← Экспортируемые функции (опционально)
```

---

## Ключевые особенности реализации

1. **Динамическая генерация**: XML генерируется на лету при каждом запросе, всегда актуальная версия
2. **Объединение модулей**: Автоматическое объединение основного модуля, расширений и экспортируемых функций
3. **Безопасность XML**: Экранирование специальных символов для корректной работы CDATA секций
4. **Уникальность**: Новый UUID при каждой генерации
5. **Обработка ошибок**: Логирование на всех этапах, graceful handling опциональных файлов
6. **Проксирование**: Next.js API route изолирует фронтенд от прямого обращения к бэкенду

---

## Логирование

Все этапы процесса логируются в Go бэкенде:
- **ERROR**: Критические ошибки (не удалось прочитать обязательный файл)
- **WARN**: Предупреждения (опциональные файлы не найдены)
- **INFO**: Успешная генерация XML с метаданными (UUID, размер модуля)

