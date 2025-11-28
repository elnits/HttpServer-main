package normalization

import (
	"fmt"
	"strconv"
	"strings"

	"httpserver/database"
)

// AttributeSchema схема атрибута для позиционного извлечения
type AttributeSchema struct {
	Position      int                    // Позиция в массиве токенов
	AttributeName string                 // Имя атрибута
	AttributeType string                 // Тип атрибута (text_value, numeric_value, dimension, etc.)
	Required      bool                   // Обязательное поле
	DefaultValue  string                 // Значение по умолчанию
	Validator     func(string) bool      // Функция валидации значения
	Transformer   func(string) string    // Функция трансформации значения
	Description   string                 // Описание атрибута
}

// PositionalExtractor извлекатель атрибутов по позициям
// Используется для структурированных данных, где позиция токена определяет его смысл
// Аналог извлечения values['module'] = tokens[4] из Python
type PositionalExtractor struct {
	schema map[int]AttributeSchema // Схема: позиция -> атрибут
	name   string                  // Название схемы
}

// NewPositionalExtractor создает новый позиционный извлекатель
func NewPositionalExtractor(name string, schema map[int]AttributeSchema) *PositionalExtractor {
	return &PositionalExtractor{
		schema: schema,
		name:   name,
	}
}

// ExtractByPosition извлекает атрибуты по позициям в массиве токенов
func (pe *PositionalExtractor) ExtractByPosition(tokens []string) ([]*database.ItemAttribute, []error) {
	var attributes []*database.ItemAttribute
	var errors []error

	for position, schema := range pe.schema {
		// Проверка наличия токена на позиции
		if position >= len(tokens) {
			if schema.Required {
				errors = append(errors, fmt.Errorf("missing required field at position %d: %s", position, schema.AttributeName))
			} else if schema.DefaultValue != "" {
				// Используем значение по умолчанию
				attributes = append(attributes, &database.ItemAttribute{
					AttributeType:  schema.AttributeType,
					AttributeName:  schema.AttributeName,
					AttributeValue: schema.DefaultValue,
					Confidence:     0.8, // Средняя уверенность для значений по умолчанию
				})
			}
			continue
		}

		value := strings.TrimSpace(tokens[position])

		// Пропускаем пустые значения
		if value == "" {
			if schema.Required {
				errors = append(errors, fmt.Errorf("empty value at position %d: %s", position, schema.AttributeName))
			}
			continue
		}

		// Трансформация значения
		if schema.Transformer != nil {
			value = schema.Transformer(value)
		}

		// Валидация
		if schema.Validator != nil && !schema.Validator(value) {
			errors = append(errors, fmt.Errorf("validation failed for position %d: %s = %s", position, schema.AttributeName, value))
			continue
		}

		// Создаем атрибут
		attributes = append(attributes, &database.ItemAttribute{
			AttributeType:  schema.AttributeType,
			AttributeName:  schema.AttributeName,
			AttributeValue: value,
			Confidence:     1.0, // Позиционное извлечение всегда уверенное
			OriginalText:   tokens[position],
		})
	}

	return attributes, errors
}

// ExtractFromDelimitedString извлекает атрибуты из строки с разделителями
func (pe *PositionalExtractor) ExtractFromDelimitedString(input string, delimiter string) ([]*database.ItemAttribute, []error) {
	tokens := strings.Split(input, delimiter)
	return pe.ExtractByPosition(tokens)
}

// ExtractFromContextualTokens извлекает атрибуты с учетом контекста (использует ContextualTokenizer)
func (pe *PositionalExtractor) ExtractFromContextualTokens(input string, delimiter rune) ([]*database.ItemAttribute, []error) {
	tokenizer := NewContextualTokenizer()
	tokens := tokenizer.SplitByDelimiter(input, delimiter)
	return pe.ExtractByPosition(tokens)
}

// ValidateSchema валидирует схему перед использованием
func (pe *PositionalExtractor) ValidateSchema() error {
	if len(pe.schema) == 0 {
		return fmt.Errorf("schema is empty")
	}

	// Проверяем, что позиции последовательны
	maxPosition := -1
	for position := range pe.schema {
		if position > maxPosition {
			maxPosition = position
		}
	}

	// Проверяем наличие обязательных полей на всех позициях до максимальной
	for i := 0; i <= maxPosition; i++ {
		if schema, exists := pe.schema[i]; exists {
			if schema.AttributeName == "" {
				return fmt.Errorf("attribute name is empty at position %d", i)
			}
			if schema.AttributeType == "" {
				return fmt.Errorf("attribute type is empty at position %d", i)
			}
		}
	}

	return nil
}

// GetSchemaDescription возвращает описание схемы
func (pe *PositionalExtractor) GetSchemaDescription() string {
	var builder strings.Builder

	builder.WriteString(fmt.Sprintf("Schema: %s\n", pe.name))
	builder.WriteString("Positions:\n")

	// Сортируем позиции
	var positions []int
	for pos := range pe.schema {
		positions = append(positions, pos)
	}
	sortInts(positions)

	for _, pos := range positions {
		schema := pe.schema[pos]
		required := ""
		if schema.Required {
			required = " (required)"
		}
		builder.WriteString(fmt.Sprintf("  [%d] %s (%s)%s - %s\n",
			pos, schema.AttributeName, schema.AttributeType, required, schema.Description))
	}

	return builder.String()
}

// sortInts сортирует массив int
func sortInts(arr []int) {
	for i := 0; i < len(arr); i++ {
		for j := i + 1; j < len(arr); j++ {
			if arr[i] > arr[j] {
				arr[i], arr[j] = arr[j], arr[i]
			}
		}
	}
}

// Предопределенные схемы для различных категорий товаров

// NewStandardProductSchema создает стандартную схему для товаров
func NewStandardProductSchema() *PositionalExtractor {
	return NewPositionalExtractor("StandardProduct", map[int]AttributeSchema{
		0: {
			Position:      0,
			AttributeName: "brand",
			AttributeType: "text_value",
			Required:      false,
			Description:   "Бренд товара",
			Validator:     func(v string) bool { return len(v) > 0 },
		},
		1: {
			Position:      1,
			AttributeName: "model",
			AttributeType: "text_value",
			Required:      false,
			Description:   "Модель товара",
			Validator:     func(v string) bool { return len(v) > 0 },
		},
		2: {
			Position:      2,
			AttributeName: "color",
			AttributeType: "text_value",
			Required:      false,
			Description:   "Цвет товара",
			Validator:     func(v string) bool { return len(v) > 0 },
		},
		3: {
			Position:      3,
			AttributeName: "dimension",
			AttributeType: "dimension",
			Required:      false,
			Description:   "Размер/габариты",
			Validator:     func(v string) bool { return strings.Contains(v, "x") || strings.Contains(v, "х") },
		},
		4: {
			Position:      4,
			AttributeName: "material",
			AttributeType: "text_value",
			Required:      false,
			Description:   "Материал изготовления",
			Validator:     func(v string) bool { return len(v) > 0 },
		},
	})
}

// NewElectronicsSchema создает схему для электроники
func NewElectronicsSchema() *PositionalExtractor {
	return NewPositionalExtractor("Electronics", map[int]AttributeSchema{
		0: {
			Position:      0,
			AttributeName: "brand",
			AttributeType: "text_value",
			Required:      true,
			Description:   "Бренд (Samsung, Apple, LG, etc.)",
		},
		1: {
			Position:      1,
			AttributeName: "model",
			AttributeType: "text_value",
			Required:      true,
			Description:   "Модель (Galaxy S21, iPhone 13, etc.)",
		},
		2: {
			Position:      2,
			AttributeName: "color",
			AttributeType: "text_value",
			Required:      false,
			Description:   "Цвет корпуса",
		},
		3: {
			Position:      3,
			AttributeName: "storage",
			AttributeType: "numeric_value",
			Required:      false,
			Description:   "Объем памяти (128GB, 256GB, etc.)",
			Validator: func(v string) bool {
				return strings.Contains(strings.ToLower(v), "gb") || strings.Contains(strings.ToLower(v), "tb")
			},
		},
		4: {
			Position:      4,
			AttributeName: "battery",
			AttributeType: "numeric_value",
			Required:      false,
			Description:   "Емкость батареи (5000mAh, etc.)",
			Validator: func(v string) bool {
				return strings.Contains(strings.ToLower(v), "mah") || strings.Contains(strings.ToLower(v), "wh")
			},
		},
	})
}

// NewFurnitureSchema создает схему для мебели
func NewFurnitureSchema() *PositionalExtractor {
	return NewPositionalExtractor("Furniture", map[int]AttributeSchema{
		0: {
			Position:      0,
			AttributeName: "type",
			AttributeType: "text_value",
			Required:      true,
			Description:   "Тип мебели (стол, стул, шкаф, etc.)",
		},
		1: {
			Position:      1,
			AttributeName: "material",
			AttributeType: "text_value",
			Required:      false,
			Description:   "Материал (дерево, металл, пластик, etc.)",
		},
		2: {
			Position:      2,
			AttributeName: "color",
			AttributeType: "text_value",
			Required:      false,
			Description:   "Цвет",
		},
		3: {
			Position:      3,
			AttributeName: "dimensions",
			AttributeType: "dimension",
			Required:      false,
			Description:   "Размеры (ДxШxВ)",
			Validator: func(v string) bool {
				return strings.Contains(v, "x") || strings.Contains(v, "х")
			},
		},
		4: {
			Position:      4,
			AttributeName: "weight_capacity",
			AttributeType: "numeric_value",
			Required:      false,
			Description:   "Грузоподъемность",
			Validator: func(v string) bool {
				return strings.Contains(strings.ToLower(v), "кг") || strings.Contains(strings.ToLower(v), "kg")
			},
		},
	})
}

// NewToolsSchema создает схему для инструментов
func NewToolsSchema() *PositionalExtractor {
	return NewPositionalExtractor("Tools", map[int]AttributeSchema{
		0: {
			Position:      0,
			AttributeName: "brand",
			AttributeType: "text_value",
			Required:      false,
			Description:   "Бренд (Bosch, Makita, DeWalt, etc.)",
		},
		1: {
			Position:      1,
			AttributeName: "type",
			AttributeType: "text_value",
			Required:      true,
			Description:   "Тип инструмента (дрель, шуруповерт, etc.)",
		},
		2: {
			Position:      2,
			AttributeName: "power",
			AttributeType: "numeric_value",
			Required:      false,
			Description:   "Мощность (550W, 18V, etc.)",
			Validator: func(v string) bool {
				lowerV := strings.ToLower(v)
				return strings.Contains(lowerV, "w") || strings.Contains(lowerV, "v") || strings.Contains(lowerV, "вт")
			},
		},
		3: {
			Position:      3,
			AttributeName: "size",
			AttributeType: "numeric_value",
			Required:      false,
			Description:   "Размер (диаметр патрона, длина, etc.)",
		},
		4: {
			Position:      4,
			AttributeName: "model",
			AttributeType: "text_value",
			Required:      false,
			Description:   "Модель",
		},
	})
}

// NewBuildingMaterialsSchema создает схему для стройматериалов
func NewBuildingMaterialsSchema() *PositionalExtractor {
	return NewPositionalExtractor("BuildingMaterials", map[int]AttributeSchema{
		0: {
			Position:      0,
			AttributeName: "material_type",
			AttributeType: "text_value",
			Required:      true,
			Description:   "Тип материала (кирпич, цемент, доска, etc.)",
		},
		1: {
			Position:      1,
			AttributeName: "material",
			AttributeType: "text_value",
			Required:      false,
			Description:   "Материал изготовления (керамика, бетон, дерево, etc.)",
		},
		2: {
			Position:      2,
			AttributeName: "dimensions",
			AttributeType: "dimension",
			Required:      false,
			Description:   "Размеры (ДxШxВ или ДxШ)",
			Validator: func(v string) bool {
				return strings.Contains(v, "x") || strings.Contains(v, "х") || strings.Contains(strings.ToLower(v), "мм") || strings.Contains(strings.ToLower(v), "см")
			},
		},
		3: {
			Position:      3,
			AttributeName: "grade",
			AttributeType: "text_value",
			Required:      false,
			Description:   "Марка/класс (М100, М200, etc.)",
		},
		4: {
			Position:      4,
			AttributeName: "volume",
			AttributeType: "numeric_value",
			Required:      false,
			Description:   "Объем/количество в упаковке",
		},
	})
}

// SchemaRegistry реестр схем
type SchemaRegistry struct {
	schemas map[string]*PositionalExtractor
}

// NewSchemaRegistry создает новый реестр схем
func NewSchemaRegistry() *SchemaRegistry {
	registry := &SchemaRegistry{
		schemas: make(map[string]*PositionalExtractor),
	}

	// Регистрируем предопределенные схемы
	registry.Register(NewStandardProductSchema())
	registry.Register(NewElectronicsSchema())
	registry.Register(NewFurnitureSchema())
	registry.Register(NewToolsSchema())
	registry.Register(NewBuildingMaterialsSchema())

	return registry
}

// Register регистрирует схему в реестре
func (sr *SchemaRegistry) Register(extractor *PositionalExtractor) {
	sr.schemas[extractor.name] = extractor
}

// Get возвращает схему по имени
func (sr *SchemaRegistry) Get(name string) (*PositionalExtractor, error) {
	if schema, exists := sr.schemas[name]; exists {
		return schema, nil
	}
	return nil, fmt.Errorf("schema '%s' not found", name)
}

// GetAll возвращает все зарегистрированные схемы
func (sr *SchemaRegistry) GetAll() map[string]*PositionalExtractor {
	return sr.schemas
}

// ListSchemas возвращает список имен всех схем
func (sr *SchemaRegistry) ListSchemas() []string {
	names := make([]string, 0, len(sr.schemas))
	for name := range sr.schemas {
		names = append(names, name)
	}
	return names
}

// Вспомогательные валидаторы

// IsNumeric проверяет, является ли строка числом
func IsNumeric(value string) bool {
	_, err := strconv.ParseFloat(strings.ReplaceAll(value, ",", "."), 64)
	return err == nil
}

// ContainsUnit проверяет, содержит ли строка единицу измерения
func ContainsUnit(value string, units []string) bool {
	lowerValue := strings.ToLower(value)
	for _, unit := range units {
		if strings.Contains(lowerValue, strings.ToLower(unit)) {
			return true
		}
	}
	return false
}

// IsDimension проверяет, является ли строка размером (100x200, 100х200)
func IsDimension(value string) bool {
	return strings.Contains(value, "x") || strings.Contains(value, "х") || strings.Contains(value, "X") || strings.Contains(value, "Х")
}
