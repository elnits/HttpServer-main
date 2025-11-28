# Скрипт для объединения модулей обработки 1С в один файл

$ModulePath = ".\Module\Module.bsl"
$ExtensionsPath = "..\1c_module_extensions.bsl"
$FunctionsPath = "..\1c_export_functions.txt"

Write-Host "Объединение модулей обработки 1С..." -ForegroundColor Cyan

# Читаем файлы
$extensions = Get-Content $ExtensionsPath -Encoding UTF8 -Raw
$functions = Get-Content $FunctionsPath -Encoding UTF8 -Raw

# Объединяем: сначала базовые функции, потом расширения
$combined = $functions + "`n`n" + $extensions

# Сохраняем объединенный модуль
$combined | Out-File -FilePath $ModulePath -Encoding UTF8 -NoNewline

Write-Host "✓ Модуль объединен: $ModulePath" -ForegroundColor Green
Write-Host "Размер файла: $((Get-Item $ModulePath).Length) байт" -ForegroundColor Gray




