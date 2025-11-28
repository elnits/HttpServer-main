# Скрипт для создания XML файла обработки 1С

$uuid = [guid]::NewGuid().ToString().ToUpper().Replace('-','')
$moduleCode = Get-Content "combined_module.bsl" -Raw -Encoding UTF8

# Заменяем ]]> на ]]]]><![CDATA[> если встречается в коде (для безопасности CDATA)
$moduleCode = $moduleCode -replace ']]>', ']]]]><![CDATA[>'

$xml = @"
<?xml version="1.0" encoding="UTF-8"?>
<MetaDataObject xmlns="http://v8.1c.ru/8.3/MDClasses" xmlns:app="http://v8.1c.ru/8.2/managed-application/core" xmlns:cfg="http://v8.1c.ru/8.1/data/enterprise/current-config" xmlns:cmi="http://v8.1c.ru/8.2/managed-application/cmi" xmlns:ent="http://v8.1c.ru/8.1/data/enterprise/current-config" xmlns:lf="http://v8.1c.ru/8.2/managed-application/logform" xmlns:style="http://v8.1c.ru/8.1/data/ui/style" xmlns:sys="http://v8.1c.ru/8.1/data/ui/fonts/system" xmlns:v8="http://v8.1c.ru/8.1/data/core" xmlns:v8ui="http://v8.1c.ru/8.1/data/ui" xmlns:web="http://v8.1c.ru/8.1/data/ui/colors/web" xmlns:win="http://v8.1c.ru/8.1/data/ui/colors/windows" xmlns:xen="http://v8.1c.ru/8.3/xcf/enums" xmlns:xpr="http://v8.1c.ru/8.3/xcf/predef" xmlns:xr="http://v8.1c.ru/8.3/xcf/readable" xmlns:xs="http://www.w3.org/2001/XMLSchema" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" version="2.12">
  <DataProcessor>
    <uuid>$uuid</uuid>
    <name>ВыгрузкаДанныхВСервис</name>
    <synonym>
      <key>ru</key>
      <value>Выгрузка данных в сервис нормализации</value>
    </synonym>
    <comment>Обработка для выгрузки данных из 1С в сервис нормализации и анализа через HTTP</comment>
    <module>
      <text><![CDATA[$moduleCode]]></text>
    </module>
    <forms>
      <form>
        <name>Форма</name>
        <synonym>
          <key>ru</key>
          <value>Форма</value>
        </synonym>
        <module>
          <text><![CDATA[&НаКлиенте
Процедура ПриСозданииНаСервере(Отказ, СтандартнаяОбработка)
	
	// Устанавливаем значения по умолчанию
	Если Объект.АдресСервера = "" Тогда
		Объект.АдресСервера = "http://localhost:9999";
	КонецЕсли;
	
	Если Объект.РазмерПакета = 0 Тогда
		Объект.РазмерПакета = 50;
	КонецЕсли;
	
	Если Объект.ИспользоватьПакетнуюВыгрузку = Неопределено Тогда
		Объект.ИспользоватьПакетнуюВыгрузку = Истина;
	КонецЕсли;
	
КонецПроцедуры]]></text>
        </module>
      </form>
    </forms>
  </DataProcessor>
</MetaDataObject>
"@

$xml | Out-File -FilePath "1c_processing_export.xml" -Encoding UTF8 -NoNewline

Write-Host "XML file created: 1c_processing_export.xml"
Write-Host "UUID: $uuid"
Write-Host "Module size: $($moduleCode.Length) characters"

