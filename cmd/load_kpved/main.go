package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"

	"httpserver/classification"
	"httpserver/database"
)

// KPVEDEntry представляет запись из файла КПВЭД
type KPVEDEntry struct {
	Code string
	Name string
}

// parseKPVEDFile парсит файл КПВЭД и возвращает список записей
func parseKPVEDFile(filename string) ([]KPVEDEntry, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	var entries []KPVEDEntry
	scanner := bufio.NewScanner(file)
	
	// Пропускаем заголовки (первые 3 строки)
	lineNum := 0
	
	// Регулярное выражение для кода КПВЭД
	kpvedCodePattern := regexp.MustCompile(`^([A-Z]|\d+(?:\.\d+)*)\s`)

	for scanner.Scan() {
		lineNum++
		if lineNum <= 3 {
			continue // Пропускаем заголовки
		}

		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}

		// Парсим строку: код и наименование могут быть разделены табуляцией или пробелами
		// Формат: "код\tнаименование" или "код наименование"
		var code, name string
		
		// Проверяем наличие табуляции
		if strings.Contains(line, "\t") {
			parts := strings.Split(line, "\t")
			if len(parts) >= 2 {
				code = strings.TrimSpace(parts[0])
				name = strings.TrimSpace(strings.Join(parts[1:], " "))
			} else {
				// Пробуем через регулярное выражение
				match := kpvedCodePattern.FindStringSubmatch(line)
				if len(match) >= 2 {
					code = strings.TrimSpace(match[1])
					nameStart := len(match[0])
					if nameStart < len(line) {
						name = strings.TrimSpace(line[nameStart:])
					}
				}
			}
		} else {
			// Разделяем по пробелам
			match := kpvedCodePattern.FindStringSubmatch(line)
			if len(match) >= 2 {
				code = strings.TrimSpace(match[1])
				nameStart := len(match[0])
				if nameStart < len(line) {
					name = strings.TrimSpace(line[nameStart:])
				}
			}
		}
		
		if code == "" || name == "" {
			continue
		}
		
		// Если наименование пустое, пропускаем
		if name == "" {
			continue
		}

		// Очищаем код и наименование
		code = strings.TrimSpace(code)
		name = strings.TrimSpace(name)
		
		// Убираем переносы строк из наименования
		name = strings.ReplaceAll(name, "\n", " ")
		name = strings.ReplaceAll(name, "\r", " ")
		name = regexp.MustCompile(`\s+`).ReplaceAllString(name, " ")

		if code != "" && name != "" {
			entries = append(entries, KPVEDEntry{
				Code: code,
				Name: name,
			})
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading file: %w", err)
	}

	return entries, nil
}

// buildCategoryTree строит дерево категорий из списка записей КПВЭД
func buildCategoryTree(entries []KPVEDEntry) (*classification.CategoryNode, error) {
	// Создаем карту для хранения узлов по коду
	nodeMap := make(map[string]*classification.CategoryNode)

	// Сначала создаем все узлы
	for _, entry := range entries {
		depth := calculateDepth(entry.Code)
		parentCode := getParentCode(entry.Code)

		node := &classification.CategoryNode{
			ID:       entry.Code,
			Name:     entry.Name,
			Path:     "", // Будет построен позже
			Level:    depth,
			ParentID: parentCode,
			Children: []classification.CategoryNode{},
		}

		nodeMap[entry.Code] = node
	}

	// Теперь строим иерархию и пути
	root := &classification.CategoryNode{
		ID:       "root",
		Name:     "КПВЭД",
		Path:     "КПВЭД",
		Level:    0,
		ParentID: "",
		Children: []classification.CategoryNode{},
	}

	// Добавляем узлы к родителям
	for code, node := range nodeMap {
		parentCode := node.ParentID
		
		// Строим путь
		node.Path = buildPathFromMap(nodeMap, code)

		if parentCode == "" || parentCode == "root" {
			// Добавляем к корню
			root.Children = append(root.Children, *node)
		} else {
			// Ищем родителя
			parent, exists := nodeMap[parentCode]
			if exists {
				parent.Children = append(parent.Children, *node)
			} else {
				// Родитель не найден - добавляем к корню
				root.Children = append(root.Children, *node)
			}
		}
	}

	return root, nil
}

// buildPathFromMap строит путь категории используя карту узлов
func buildPathFromMap(nodeMap map[string]*classification.CategoryNode, code string) string {
	node, exists := nodeMap[code]
	if !exists {
		return ""
	}

	path := []string{node.Name}
	currentCode := code

	for {
		parentCode := getParentCode(currentCode)
		if parentCode == "" || parentCode == "root" {
			break
		}

		parent, exists := nodeMap[parentCode]
		if !exists {
			break
		}

		path = append([]string{parent.Name}, path...)
		currentCode = parentCode
	}

	return strings.Join(path, " / ")
}

// calculateDepth вычисляет глубину категории по коду
func calculateDepth(code string) int {
	if code == "" {
		return 0
	}
	// Глубина = количество точек + 1
	dots := strings.Count(code, ".")
	if code[0] >= 'A' && code[0] <= 'Z' {
		// Буквенный код (A, B, C...) - уровень 1
		return 1
	}
	return dots + 1
}

// getParentCode получает код родителя
func getParentCode(code string) string {
	if code == "" {
		return ""
	}

	// Если код начинается с буквы - это корневой уровень
	if code[0] >= 'A' && code[0] <= 'Z' {
		return "root"
	}

	// Находим последнюю точку
	lastDot := strings.LastIndex(code, ".")
	if lastDot == -1 {
		// Нет точек - это первый уровень после буквы
		return "root"
	}

	// Возвращаем код до последней точки
	parentCode := code[:lastDot]
	
	// Если родитель - это одна цифра, проверяем, есть ли буква выше
	if len(parentCode) == 2 && parentCode[0] >= '0' && parentCode[0] <= '9' {
		// Это уровень 01, 02 и т.д. - родитель должен быть буквой
		// Но в нашем случае мы используем root
		return "root"
	}

	return parentCode
}


// countNodes подсчитывает общее количество узлов в дереве
func countNodes(node *classification.CategoryNode) int {
	count := 1
	for i := range node.Children {
		count += countNodes(&node.Children[i])
	}
	return count
}

func main() {
	if len(os.Args) < 3 {
		fmt.Println("Использование: load_kpved <путь_к_файлу_КПВЭД.txt> <путь_к_базе.db> [client_id] [project_id]")
		fmt.Println("Пример: load_kpved КПВЭД.txt 1c_data.db 1 1")
		os.Exit(1)
	}

	kpvedFile := os.Args[1]
	dbPath := os.Args[2]

	var clientID *int
	var projectID *int

	if len(os.Args) >= 4 {
		id := 0
		fmt.Sscanf(os.Args[3], "%d", &id)
		if id > 0 {
			clientID = &id
		}
	}

	if len(os.Args) >= 5 {
		id := 0
		fmt.Sscanf(os.Args[4], "%d", &id)
		if id > 0 {
			projectID = &id
		}
	}

	fmt.Printf("Загрузка классификатора КПВЭД из файла: %s\n", kpvedFile)
	fmt.Printf("База данных: %s\n", dbPath)
	if clientID != nil {
		fmt.Printf("Client ID: %d\n", *clientID)
	}
	if projectID != nil {
		fmt.Printf("Project ID: %d\n", *projectID)
	}
	fmt.Println()

	// Парсим файл
	fmt.Println("Парсинг файла КПВЭД...")
	entries, err := parseKPVEDFile(kpvedFile)
	if err != nil {
		log.Fatalf("Ошибка парсинга файла: %v", err)
	}
	fmt.Printf("Найдено записей: %d\n", len(entries))

	// Строим дерево
	fmt.Println("Построение дерева категорий...")
	tree, err := buildCategoryTree(entries)
	if err != nil {
		log.Fatalf("Ошибка построения дерева: %v", err)
	}
	
	// Подсчитываем общее количество узлов
	totalNodes := countNodes(tree)
	fmt.Printf("Дерево построено. Корневых категорий: %d, всего узлов: %d\n", len(tree.Children), totalNodes)

	// Сериализуем дерево в JSON
	treeJSON, err := json.Marshal(tree)
	if err != nil {
		log.Fatalf("Ошибка сериализации дерева: %v", err)
	}

	// Подключаемся к базе
	fmt.Println("Подключение к базе данных...")
	db, err := database.NewDB(dbPath)
	if err != nil {
		log.Fatalf("Ошибка подключения к базе: %v", err)
	}
	defer db.Close()

	// Создаем классификатор
	classifier := &database.CategoryClassifier{
		Name:          "КПВЭД",
		Description:   "Классификатор продукции по видам экономической деятельности",
		MaxDepth:      6,
		TreeStructure: string(treeJSON),
		ClientID:      clientID,
		ProjectID:     projectID,
		IsActive:      true,
	}

	fmt.Println("Сохранение классификатора в базу данных...")
	created, err := db.CreateCategoryClassifier(classifier)
	if err != nil {
		log.Fatalf("Ошибка сохранения классификатора: %v", err)
	}

	fmt.Printf("Классификатор успешно загружен!\n")
	fmt.Printf("ID: %d\n", created.ID)
	fmt.Printf("Название: %s\n", created.Name)
	fmt.Printf("Максимальная глубина: %d\n", created.MaxDepth)
	fmt.Printf("Активен: %v\n", created.IsActive)
	if created.ClientID != nil {
		fmt.Printf("Client ID: %d\n", *created.ClientID)
	}
	if created.ProjectID != nil {
		fmt.Printf("Project ID: %d\n", *created.ProjectID)
	}
}
