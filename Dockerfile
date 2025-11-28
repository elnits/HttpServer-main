# Многоэтапная сборка для Go бэкенда
FROM golang:1.25-alpine AS builder

# Устанавливаем зависимости для сборки
RUN apk add --no-cache gcc musl-dev sqlite-dev git

# Устанавливаем рабочую директорию
WORKDIR /app

# Копируем go mod файлы
COPY go.mod go.sum ./
RUN go mod download

# Копируем исходный код
COPY . .

# Собираем приложение без GUI зависимостей (используем build tag no_gui)
# Компилируем только main_no_gui.go, чтобы избежать конфликта с main_docker.go
RUN CGO_ENABLED=1 GOOS=linux go build -tags no_gui -o httpserver -ldflags="-w -s" main_no_gui.go

# Финальный образ
FROM alpine:latest

# Устанавливаем необходимые пакеты для SQLite
RUN apk add --no-cache ca-certificates sqlite

# Создаем пользователя для запуска приложения
RUN addgroup -S appgroup && adduser -S appuser -G appgroup

# Устанавливаем рабочую директорию
WORKDIR /app

# Копируем бинарник из builder
COPY --from=builder /app/httpserver .

# Копируем необходимые файлы для генерации XML
COPY --from=builder /app/1c_processing ./1c_processing
COPY --from=builder /app/1c_module_extensions.bsl ./
COPY --from=builder /app/1c_export_functions.txt ./
# Копируем файл классификатора КПВЭД
COPY --from=builder /app/КПВЭД.txt ./КПВЭД.txt

# Создаем директории для баз данных
RUN mkdir -p /app/data && chown -R appuser:appgroup /app

# Переключаемся на непривилегированного пользователя
USER appuser

# Открываем порт
EXPOSE 9999

# Запускаем приложение
CMD ["./httpserver"]

