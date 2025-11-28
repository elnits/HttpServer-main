## Обратная выгрузка данных

### Цель
Организовать зеркальный процесс по отношению к текущему приему данных из 1С: сервер инициирует исходящее соединение, подтверждает рукопожатие и отправляет XML-пакеты из SQLite базе обратно в клиент.

### Последовательность
1. Клиент (1С либо внешний сервис) выставляет HTTP endpoint и ожидает входящих POST-запросов в формате текущих XML-схем.
2. Внутренний REST-эндпоинт `POST /api/uploads/{uuid}/export` принимает параметры:
   - `target_url` — базовый URL клиента, куда слать пакеты;
   - `include` — список типов (`metadata`, `constants`, `catalogs`, `nomenclature`);
   - `catalog_names` (опционально) — фильтр справочников.
3. Сервер создает задачу экспорта, присваивает `export_id` и запускает горутину.

### Шаги экспорта
| Шаг | XML | Описание |
| --- | --- | --- |
| Handshake | `handshake` | Отправляем POST `${target_url}/handshake`. Тело формируется из `database.uploads` (поля `version_1c`, `config_name`, `upload_type` и т.д.). Получаем `handshake_response`, валидируем `success=true`. |
| Metadata | `metadata` | Отправляем POST `${target_url}/metadata` с метаданными выгрузки. Выполняется, если запрошен блок `metadata`. |
| Constants | `constant` | Итеративно читаем таблицу `constants` по `upload_uuid` и отправляем каждый элемент POSTом `${target_url}/constant`. Поддерживаем ретраи, логируем ошибки. |
| Catalog meta | `catalog_meta` | Для каждой записи справочника отправляем `catalog_meta`, фиксируем `catalog_id` из ответа (используем локальное сопоставление id ↔ reference). |
| Catalog items | `catalog_item` / `catalog_items` | В зависимости от поддерживаемого формата клиента отправляем либо одиночные, либо батчи. Используем уже определённые структуры `CatalogItemRequest`, `CatalogItemsRequest`. |
| Nomenclature batch | `nomenclature_batch` | Аналогично каталогам, доступно по флагу `include`. |
| Complete | `complete` | После успешной передачи всех данных отправляем `/complete`. |

### Состояние и статусы
Запись о задаче хранится в памяти сервера:
```go
type ExportJob struct {
    ID string
    UploadUUID string
    Status ExportStatus // pending, running, failed, completed
    StartedAt time.Time
    FinishedAt *time.Time
    Progress ExportProgress
    Error string
}
```
`ExportProgress` включает счётчики отправленных пакетов. REST `GET /api/exports/{id}` возвращает структуру задачи. Дополнительно `GET /api/uploads/{uuid}/exports` выдаёт историю по выгрузке.

### Ошибки и повторные попытки
- Для каждого HTTP запроса используем таймаут 30s и до 3 повторов с экспоненциальной задержкой.
- При фатальной ошибке ставим `Status=failed`, сохраняем текст ошибки и время завершения.
- Если целевой endpoint ответил `success=false`, прекращаем процесс и пишем причину в лог.

### Безопасность
- Новые REST-эндпоинты используют те же middleware (API key/Basic/Auth) что и остальной сервер.
- Перед запуском экспорта проверяем, что `upload_uuid` принадлежит активной БД (через `database.DB`).

### Расширения
- В дальнейшем можно добавить SSE/WS события для live-прогресса.
- Поддержка параллельной отправки разных блоков за счёт нескольких горутин, пока не требуется.


