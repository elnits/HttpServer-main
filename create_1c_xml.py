#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
Скрипт для создания XML файла обработки 1С для импорта в конфигуратор
"""

import xml.sax.saxutils
import uuid

def escape_xml(text):
    """Экранирование XML символов в тексте модуля"""
    # Экранируем специальные символы XML
    text = text.replace('&', '&amp;')
    text = text.replace('<', '&lt;')
    text = text.replace('>', '&gt;')
    # Кавычки экранируем только если они внутри XML атрибутов
    # В коде модуля оставляем как есть, так как они внутри CDATA или текста
    return text

def read_file_content(filepath):
    """Чтение содержимого файла"""
    try:
        with open(filepath, 'r', encoding='utf-8') as f:
            return f.read()
    except Exception as e:
        print(f"Ошибка чтения файла {filepath}: {e}")
        return ""

def create_1c_processing_xml():
    """Создание XML файла обработки 1С"""
    
    # Читаем основной модуль
    module_code = read_file_content('1c_processing/Module/Module.bsl')
    
    # Читаем расширения
    extensions_code = read_file_content('1c_module_extensions.bsl')
    
    # Читаем полный код из export_functions
    export_functions_code = read_file_content('1c_export_functions.txt')
    
    # Объединяем код модуля
    # Сначала базовый модуль, потом расширения, потом полный код
    full_module_code = module_code
    
    # Добавляем код из export_functions, исключая дубликаты
    # Добавляем только код из области ПрограммныйИнтерфейс
    if export_functions_code:
        # Находим область ПрограммныйИнтерфейс
        start_marker = "#Область ПрограммныйИнтерфейс"
        end_marker = "#КонецОбласти"
        
        start_pos = export_functions_code.find(start_marker)
        if start_pos >= 0:
            end_pos = export_functions_code.find(end_marker, start_pos + len(start_marker))
            if end_pos >= 0:
                program_interface_code = export_functions_code[start_pos:end_pos + len(end_marker)]
                full_module_code += "\n\n" + program_interface_code
    
    # Добавляем расширения
    if extensions_code:
        full_module_code += "\n\n" + extensions_code
    
    # Экранируем код модуля для XML
    escaped_module_code = escape_xml(full_module_code)
    
    # Генерируем UUID для обработки
    processing_uuid = str(uuid.uuid4()).upper().replace('-', '')
    
    # Создаем XML
    xml_content = f'''<?xml version="1.0" encoding="UTF-8"?>
<MetaDataObject xmlns="http://v8.1c.ru/8.3/MDClasses" xmlns:app="http://v8.1c.ru/8.2/managed-application/core" xmlns:cfg="http://v8.1c.ru/8.1/data/enterprise/current-config" xmlns:cmi="http://v8.1c.ru/8.2/managed-application/cmi" xmlns:ent="http://v8.1c.ru/8.1/data/enterprise/current-config" xmlns:lf="http://v8.1c.ru/8.2/managed-application/logform" xmlns:style="http://v8.1c.ru/8.1/data/ui/style" xmlns:sys="http://v8.1c.ru/8.1/data/ui/fonts/system" xmlns:v8="http://v8.1c.ru/8.1/data/core" xmlns:v8ui="http://v8.1c.ru/8.1/data/ui" xmlns:web="http://v8.1c.ru/8.1/data/ui/colors/web" xmlns:win="http://v8.1c.ru/8.1/data/ui/colors/windows" xmlns:xen="http://v8.1c.ru/8.3/xcf/enums" xmlns:xpr="http://v8.1c.ru/8.3/xcf/predef" xmlns:xr="http://v8.1c.ru/8.3/xcf/readable" xmlns:xs="http://www.w3.org/2001/XMLSchema" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" version="2.12">
  <DataProcessor>
    <uuid>{processing_uuid}</uuid>
    <name>ВыгрузкаДанныхВСервис</name>
    <synonym>
      <key>ru</key>
      <value>Выгрузка данных в сервис нормализации</value>
    </synonym>
    <comment>Обработка для выгрузки данных из 1С в сервис нормализации и анализа через HTTP</comment>
    <module>
      <text><![CDATA[{full_module_code}]]></text>
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
</MetaDataObject>'''
    
    # Сохраняем XML файл
    output_file = '1c_processing_export.xml'
    with open(output_file, 'w', encoding='utf-8') as f:
        f.write(xml_content)
    
    print(f"XML файл обработки создан: {output_file}")
    print(f"UUID обработки: {processing_uuid}")
    print(f"Размер модуля: {len(full_module_code)} символов")

if __name__ == '__main__':
    create_1c_processing_xml()

