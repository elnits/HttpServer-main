package normalization

import (
	"database/sql"
	"fmt"
	"log"
	"sort"
	"strings"
)

// KpvedDB интерфейс для работы с базой данных КПВЭД
// Может быть реализован как *database.DB или *database.ServiceDB
type KpvedDB interface {
	Query(query string, args ...interface{}) (*sql.Rows, error)
	QueryRow(query string, args ...interface{}) *sql.Row
}

// KpvedLevel уровень иерархии КПВЭД
type KpvedLevel string

const (
	LevelSection  KpvedLevel = "section"  // A, B, C... (21 раздел)
	LevelClass    KpvedLevel = "class"    // 01, 02, 03... (88 классов)
	LevelSubclass KpvedLevel = "subclass" // 01.1, 01.2...
	LevelGroup    KpvedLevel = "group"    // 01.11, 01.12...
	LevelSubgroup KpvedLevel = "subgroup" // 01.11.1, 01.11.2...
)

// KpvedNode узел дерева КПВЭД
type KpvedNode struct {
	Code       string       `json:"code"`
	Name       string       `json:"name"`
	Level      KpvedLevel   `json:"level"`
	ParentCode string       `json:"parent_code,omitempty"`
	Children   []*KpvedNode `json:"children,omitempty"`
}

// KpvedTree дерево классификатора КПВЭД
type KpvedTree struct {
	Root    *KpvedNode
	NodeMap map[string]*KpvedNode
}

// NewKpvedTree создает новое дерево КПВЭД
func NewKpvedTree() *KpvedTree {
	return &KpvedTree{
		Root: &KpvedNode{
			Code:     "root",
			Name:     "КПВЭД Root",
			Level:    "root",
			Children: make([]*KpvedNode, 0),
		},
		NodeMap: make(map[string]*KpvedNode),
	}
}

// BuildFromDatabase строит дерево из базы данных
func (t *KpvedTree) BuildFromDatabase(db KpvedDB) error {
	query := `SELECT code, name, parent_code, level FROM kpved_classifier ORDER BY code`

	rows, err := db.Query(query)
	if err != nil {
		return fmt.Errorf("failed to query kpved: %w", err)
	}
	defer rows.Close()

	// Сначала создаем все узлы
	allNodes := make([]*KpvedNode, 0)
	for rows.Next() {
		var code, name string
		var parentCode sql.NullString
		var level int

		if err := rows.Scan(&code, &name, &parentCode, &level); err != nil {
			log.Printf("Error scanning row: %v", err)
			continue
		}

		node := &KpvedNode{
			Code:     code,
			Name:     name,
			Level:    t.determineLevel(code),
			Children: make([]*KpvedNode, 0),
		}

		if parentCode.Valid {
			node.ParentCode = parentCode.String
		} else {
			node.ParentCode = t.getParentCodeFromFormat(code)
		}

		allNodes = append(allNodes, node)
		t.NodeMap[code] = node
	}

	// Строим иерархию
	for _, node := range allNodes {
		if node.ParentCode == "" || node.ParentCode == "root" {
			// Корневой узел (секция)
			t.Root.Children = append(t.Root.Children, node)
		} else {
			// Добавляем к родителю
			if parent, exists := t.NodeMap[node.ParentCode]; exists {
				parent.Children = append(parent.Children, node)
			} else {
				// Если родитель не найден, добавляем к корню
				log.Printf("Warning: parent %s not found for node %s", node.ParentCode, node.Code)
				t.Root.Children = append(t.Root.Children, node)
			}
		}
	}

	// Сортируем детей в каждом узле
	t.sortTree(t.Root)

	log.Printf("Built KPVED tree with %d nodes", len(t.NodeMap))
	return nil
}

// determineLevel определяет уровень по формату кода
func (t *KpvedTree) determineLevel(code string) KpvedLevel {
	// Убираем пробелы
	code = strings.TrimSpace(code)

	// Секция: A, B, C... (одна буква)
	if len(code) == 1 && code[0] >= 'A' && code[0] <= 'Z' {
		return LevelSection
	}

	// Класс: 01, 02... (две цифры)
	if len(code) == 2 && isDigits(code) {
		return LevelClass
	}

	// Считаем точки
	dotCount := strings.Count(code, ".")

	switch dotCount {
	case 1:
		// Подкласс: 01.1, 01.2...
		return LevelSubclass
	case 2:
		// Группа: 01.11.1, 01.12.2...
		// Но может быть и подгруппа, зависит от длины
		parts := strings.Split(code, ".")
		if len(parts) == 3 && len(parts[2]) == 1 {
			return LevelGroup
		}
		return LevelSubgroup
	case 3:
		// Подгруппа: 01.11.11.1
		return LevelSubgroup
	default:
		return LevelSection
	}
}

// getParentCodeFromFormat определяет код родителя по формату
func (t *KpvedTree) getParentCodeFromFormat(code string) string {
	level := t.determineLevel(code)

	switch level {
	case LevelSection:
		return "root"
	case LevelClass:
		// Родитель - секция (первый символ)
		// Но у нас классы начинаются с цифр, а секции - буквы
		// Нужно найти секцию по диапазону классов
		// Для простоты возвращаем пустую строку
		return ""
	case LevelSubclass:
		// Родитель - класс (до точки)
		parts := strings.Split(code, ".")
		return parts[0]
	case LevelGroup:
		// Родитель - подкласс (первые 2 части)
		parts := strings.Split(code, ".")
		if len(parts) >= 2 {
			return parts[0] + "." + parts[1]
		}
		return ""
	case LevelSubgroup:
		// Родитель - группа (первые 3 части)
		parts := strings.Split(code, ".")
		if len(parts) >= 3 {
			return parts[0] + "." + parts[1] + "." + parts[2]
		}
		return ""
	default:
		return ""
	}
}

// GetNodesAtLevel возвращает все узлы указанного уровня
func (t *KpvedTree) GetNodesAtLevel(level KpvedLevel, parentCode string) []*KpvedNode {
	var nodes []*KpvedNode

	// Для секций (уровень 1) возвращаем напрямую детей Root
	if level == LevelSection && parentCode == "" {
		return t.Root.Children
	}

	// Если указан родитель, ищем детей этого родителя
	if parentCode != "" {
		parentNode, exists := t.NodeMap[parentCode]
		if !exists {
			return nodes // Пустой список
		}
		// Возвращаем детей, которые соответствуют указанному уровню
		for _, child := range parentNode.Children {
			if child.Level == level {
				nodes = append(nodes, child)
			}
		}
		return nodes
	}

	// Иначе ищем все узлы указанного уровня
	var collect func(node *KpvedNode)
	collect = func(node *KpvedNode) {
		if node.Level == level {
			nodes = append(nodes, node)
		}
		// Рекурсивно обходим детей
		for _, child := range node.Children {
			collect(child)
		}
	}

	collect(t.Root)
	return nodes
}

// GetNode возвращает узел по коду
func (t *KpvedTree) GetNode(code string) (*KpvedNode, bool) {
	node, exists := t.NodeMap[code]
	return node, exists
}

// sortTree сортирует детей в дереве
func (t *KpvedTree) sortTree(node *KpvedNode) {
	if len(node.Children) == 0 {
		return
	}

	// Сортируем детей по коду
	sort.Slice(node.Children, func(i, j int) bool {
		return node.Children[i].Code < node.Children[j].Code
	})

	// Рекурсивно сортируем детей
	for _, child := range node.Children {
		t.sortTree(child)
	}
}

// GetNextLevel возвращает следующий уровень иерархии
func GetNextLevel(current KpvedLevel) KpvedLevel {
	switch current {
	case LevelSection:
		return LevelClass
	case LevelClass:
		return LevelSubclass
	case LevelSubclass:
		return LevelGroup
	case LevelGroup:
		return LevelSubgroup
	default:
		return current
	}
}

// isDigits проверяет, состоит ли строка только из цифр
func isDigits(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// GetLevelName возвращает название уровня на русском
func GetLevelName(level KpvedLevel) string {
	switch level {
	case LevelSection:
		return "Раздел"
	case LevelClass:
		return "Класс"
	case LevelSubclass:
		return "Подкласс"
	case LevelGroup:
		return "Группа"
	case LevelSubgroup:
		return "Подгруппа"
	default:
		return "Неизвестный уровень"
	}
}
