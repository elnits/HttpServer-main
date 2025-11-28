package normalization

import (
	"regexp"
	"strings"
)

// ObjectType тип объекта (товар или услуга)
type ObjectType string

const (
	ObjectTypeProduct ObjectType = "product"  // Товар
	ObjectTypeService ObjectType = "service"   // Услуга
	ObjectTypeUnknown ObjectType = "unknown"  // Неопределено
)

// DetectionResult результат определения типа объекта
type DetectionResult struct {
	Type       ObjectType
	Confidence float64
	Reasoning  string
}

// ProductServiceDetector детектор для определения товар/услуга
type ProductServiceDetector struct {
	productIndicators  []string
	serviceIndicators  []string
	productPatterns    []*regexp.Regexp
	servicePatterns    []*regexp.Regexp
}

// NewProductServiceDetector создает новый детектор товар/услуга
func NewProductServiceDetector() *ProductServiceDetector {
	detector := &ProductServiceDetector{
		productIndicators: []string{
			"кабель", "датчик", "преобразователь", "элемент",
			"панель", "оборудование", "материал", "изделие",
			"марка", "модель", "размер", "диаметр", "длина",
			"ширина", "высота", "вес", "толщина", "артикул",
			"болт", "винт", "гайка", "шайба", "саморез",
			"муфта", "тройник", "фильтр", "редуктор",
			"подшипник", "клапан", "насос", "двигатель",
			"трансформатор", "автомат", "выключатель",
			"розетка", "вилка", "разъем", "коннектор",
			"провод", "шнур", "жгут", "лента", "пленка",
			"лист", "плита", "блок", "кирпич", "бетон",
			"цемент", "песок", "щебень", "арматура",
			"профиль", "труба", "швеллер", "уголок",
			"балка", "рейка", "доска", "брус", "бревно",
			"краска", "лак", "грунтовка", "шпаклевка",
			"герметик", "клей", "мастика", "изоляция",
			"утеплитель", "пароизоляция", "гидроизоляция",
			"фанера", "дсп", "двп", "осб", "мдф",
			"металл", "сталь", "алюминий", "медь",
			"пластик", "полиэтилен", "полипропилен",
			"резина", "силикон", "текстиль", "ткань",
			"фасонные", "комплектующие", "запчасти",
			"компонент", "деталь", "узел", "блок",
			"модуль", "система", "комплект", "набор",
		},
		serviceIndicators: []string{
			"услуга", "услуги", "работы", "работа",
			"выполнение", "оказание", "предоставление",
			"монтаж", "установка", "демонтаж", "сборка",
			"разборка", "ремонт", "обслуживание",
			"техобслуживание", "настройка", "регулировка",
			"калибровка", "поверка", "аттестация",
			"сертификация", "испытание", "тестирование",
			"проверка", "контроль", "аудит", "экспертиза",
			"консультация", "консультирование", "совет",
			"помощь", "поддержка", "обучение", "тренинг",
			"доставка", "транспортировка", "перевозка",
			"грузоперевозка", "логистика", "складирование",
			"хранение", "упаковка", "фасовка", "маркировка",
			"проектирование", "проект", "разработка",
			"дизайн", "планирование", "проектирование",
			"строительство", "строительные работы",
			"отделочные работы", "ремонтные работы",
			"монтажные работы", "пусконаладочные работы",
			"наладка", "пусконаладка", "ввод в эксплуатацию",
		},
	}

	// Компилируем паттерны для более точного определения
	detector.productPatterns = []*regexp.Regexp{
		regexp.MustCompile(`\b(кабель|провод|шнур|жгут)\b`),
		regexp.MustCompile(`\b(датчик|преобразователь|измеритель|сенсор)\b`),
		regexp.MustCompile(`\b(фасонные\s+элементы?|комплектующие|запчасти)\b`),
		regexp.MustCompile(`\b(панель|плита|лист|блок|кирпич)\b`),
		regexp.MustCompile(`\b(оборудование|аппарат|прибор|устройство|механизм)\b`),
		regexp.MustCompile(`\b(материал|изделие|продукция|товар)\b`),
		regexp.MustCompile(`\b(артикул|арт\.?|art\.?|модель|марка|тип)\s*[:\-]?\s*[a-zA-Z0-9]+\b`),
		regexp.MustCompile(`\b(размер|диаметр|длина|ширина|высота|толщина|вес)\s*[:\-]?\s*[\d.,xх]+\b`),
		regexp.MustCompile(`\b(ral|din|iso|gost|гост)\s*[a-zA-Z0-9]+\b`),
		regexp.MustCompile(`\b\d+\s*(мм|см|м|кг|г|л|мл|шт|шт\.|штук)\b`),
	}

	detector.servicePatterns = []*regexp.Regexp{
		regexp.MustCompile(`\b(услуг[аи]|услуги|услуг)\b`),
		regexp.MustCompile(`\b(работ[аы]|работа|работ)\b`),
		regexp.MustCompile(`\b(выполнение|оказание|предоставление)\s+(услуг|работ)\b`),
		regexp.MustCompile(`\b(монтаж|установка|демонтаж|сборка|разборка)\b`),
		regexp.MustCompile(`\b(ремонт|обслуживание|техобслуживание|настройка)\b`),
		regexp.MustCompile(`\b(испытание|тестирование|проверка|контроль|аудит|экспертиза)\b`),
		regexp.MustCompile(`\b(консультация|консультирование|совет|помощь|поддержка)\b`),
		regexp.MustCompile(`\b(обучение|тренинг|курс|семинар)\b`),
		regexp.MustCompile(`\b(доставка|транспортировка|перевозка|грузоперевозка|логистика)\b`),
		regexp.MustCompile(`\b(проектирование|проект|разработка|дизайн|планирование)\b`),
		regexp.MustCompile(`\b(строительство|строительные\s+работы|отделочные\s+работы|ремонтные\s+работы)\b`),
	}

	return detector
}

// DetectProductOrService определяет тип объекта (товар или услуга)
func (d *ProductServiceDetector) DetectProductOrService(name, description string) *DetectionResult {
	// Объединяем название и описание для анализа
	input := strings.ToLower(name + " " + description)
	
	// Подсчитываем индикаторы товаров и услуг
	productScore := 0.0
	serviceScore := 0.0
	var reasoning []string

	// Проверяем паттерны товаров
	for _, pattern := range d.productPatterns {
		if pattern.MatchString(input) {
			productScore += 1.5 // Паттерны имеют больший вес
			reasoning = append(reasoning, "найдены признаки товара")
		}
	}

	// Проверяем паттерны услуг
	for _, pattern := range d.servicePatterns {
		if pattern.MatchString(input) {
			serviceScore += 1.5
			reasoning = append(reasoning, "найдены признаки услуги")
		}
	}

	// Проверяем ключевые слова товаров
	for _, indicator := range d.productIndicators {
		if strings.Contains(input, indicator) {
			productScore += 0.5
		}
	}

	// Проверяем ключевые слова услуг
	for _, indicator := range d.serviceIndicators {
		if strings.Contains(input, indicator) {
			serviceScore += 0.5
		}
	}

	// Дополнительные признаки товара
	if d.hasProductCharacteristics(input) {
		productScore += 1.0
		reasoning = append(reasoning, "найдены технические характеристики товара")
	}

	// Определяем результат
	var resultType ObjectType
	var confidence float64
	var finalReasoning string

	if productScore > serviceScore && productScore > 0 {
		resultType = ObjectTypeProduct
		confidence = d.calculateConfidence(productScore, serviceScore)
		finalReasoning = "определен как товар: " + strings.Join(reasoning, ", ")
	} else if serviceScore > productScore && serviceScore > 0 {
		resultType = ObjectTypeService
		confidence = d.calculateConfidence(serviceScore, productScore)
		finalReasoning = "определен как услуга: " + strings.Join(reasoning, ", ")
	} else {
		resultType = ObjectTypeUnknown
		confidence = 0.5
		finalReasoning = "не удалось однозначно определить тип"
	}

	return &DetectionResult{
		Type:       resultType,
		Confidence: confidence,
		Reasoning:  finalReasoning,
	}
}

// hasProductCharacteristics проверяет наличие технических характеристик товара
func (d *ProductServiceDetector) hasProductCharacteristics(input string) bool {
	// Паттерны технических характеристик
	characteristicPatterns := []*regexp.Regexp{
		regexp.MustCompile(`\b\d+\s*(мм|см|м|кг|г|л|мл|шт)\b`),                    // Размеры и единицы измерения
		regexp.MustCompile(`\b\d+[xх]\d+`),                                      // Размеры типа 120x70
		regexp.MustCompile(`\b\d+[.,]\d+\s*(мм|см|м|кг|г)\b`),                   // Десятичные размеры
		regexp.MustCompile(`\b(арт\.?|art\.?|№)\s*[a-zA-Z0-9.-]+\b`),            // Артикулы
		regexp.MustCompile(`\b(ral|din|iso|gost|гост)\s*[a-zA-Z0-9]+\b`),       // Стандарты
		regexp.MustCompile(`\b(марка|модель|тип|серия)\s*[:\-]?\s*[a-zA-Z0-9]+\b`), // Марки и модели
		regexp.MustCompile(`\b[a-zA-Z]{2,}\d+\b`),                                // Коды типа AKS32R, HELUKABEL
	}

	for _, pattern := range characteristicPatterns {
		if pattern.MatchString(input) {
			return true
		}
	}

	return false
}

// calculateConfidence вычисляет уверенность на основе разницы в баллах
func (d *ProductServiceDetector) calculateConfidence(primaryScore, secondaryScore float64) float64 {
	if primaryScore == 0 {
		return 0.5
	}

	diff := primaryScore - secondaryScore
	if diff < 0 {
		diff = 0
	}

	// Нормализуем уверенность от 0.6 до 0.95
	confidence := 0.6 + (diff / (primaryScore + 1)) * 0.35
	if confidence > 0.95 {
		confidence = 0.95
	}
	if confidence < 0.6 {
		confidence = 0.6
	}

	return confidence
}

// IsLikelyProduct проверяет, является ли объект вероятно товаром
func (d *ProductServiceDetector) IsLikelyProduct(name, description string) bool {
	result := d.DetectProductOrService(name, description)
	return result.Type == ObjectTypeProduct && result.Confidence > 0.7
}

// IsLikelyService проверяет, является ли объект вероятно услугой
func (d *ProductServiceDetector) IsLikelyService(name, description string) bool {
	result := d.DetectProductOrService(name, description)
	return result.Type == ObjectTypeService && result.Confidence > 0.7
}

