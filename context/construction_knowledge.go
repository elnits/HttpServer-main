package context

import "strings"

// ConstructionKnowledge содержит знания о строительных материалах
type ConstructionKnowledge struct {
	MaterialPatterns map[string]string
	ProductTypes     map[string][]string
	BrandMappings    map[string]string
}

// NewConstructionKnowledge создает новую базу знаний о строительных материалах
func NewConstructionKnowledge() *ConstructionKnowledge {
	return &ConstructionKnowledge{
		MaterialPatterns: map[string]string{
			"isowall":     "сэндвич_панель",
			"isopan":      "сэндвич_панель",
			"изопан":      "сэндвич_панель",
			"сэндвич":     "сэндвич_панель",
			"sandwich":    "сэндвич_панель",
			"минеральная": "минеральная_вата",
			"mineral":     "минеральная_вата",
			"fire":        "огнестойкий",
			"файер":       "огнестойкий",
			"isofire":     "огнестойкая_панель",
		},
		ProductTypes: map[string][]string{
			"сэндвич_панель": {
				"25.11.11 Металлические конструкции",
				"23.99.19 Изделия строительные прочие",
			},
		},
		BrandMappings: map[string]string{
			"isowall":    "isopan",
			"изовол":     "минеральная_вата",
			"isofire":    "огнестойкая_панель",
			"isocop":     "сэндвич_панель",
			"isowallbox": "сэндвич_панель",
		},
	}
}

// GetRecommendedCategory возвращает рекомендуемые категории для продукта
func (ck *ConstructionKnowledge) GetRecommendedCategory(productType string) []string {
	if categories, exists := ck.ProductTypes[productType]; exists {
		return categories
	}
	return []string{}
}

// IsSandwichPanel проверяет, является ли продукт сэндвич-панелью
func (ck *ConstructionKnowledge) IsSandwichPanel(name string) bool {
	nameLower := name
	for pattern, productType := range ck.MaterialPatterns {
		if containsIgnoreCase(nameLower, pattern) && productType == "сэндвич_панель" {
			return true
		}
	}
	return false
}

// GetBrandMapping возвращает маппинг бренда
func (ck *ConstructionKnowledge) GetBrandMapping(brand string) string {
	if mapping, exists := ck.BrandMappings[brand]; exists {
		return mapping
	}
	return brand
}

// containsIgnoreCase проверяет, содержит ли строка подстроку (без учета регистра)
func containsIgnoreCase(s, substr string) bool {
	sLower := strings.ToLower(s)
	substrLower := strings.ToLower(substr)
	return strings.Contains(sLower, substrLower)
}

