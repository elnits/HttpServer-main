package nomenclature

import (
	"bufio"
	"os"
	"regexp"
	"strings"
	"sync"
)

// KpvedProcessor обработчик справочника КПВЭД
type KpvedProcessor struct {
	data   string
	codes  map[string]bool
	mu     sync.RWMutex
	loaded bool
}

// NewKpvedProcessor создает новый процессор КПВЭД
func NewKpvedProcessor() *KpvedProcessor {
	return &KpvedProcessor{
		codes: make(map[string]bool),
	}
}

// LoadKpved загружает данные КПВЭД из файла один раз
func (k *KpvedProcessor) LoadKpved(filePath string) error {
	k.mu.Lock()
	defer k.mu.Unlock()

	if k.loaded {
		return nil
	}

	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	var content strings.Builder
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := scanner.Text()
		content.WriteString(line)
		content.WriteString("\n")

		// Извлекаем коды КПВЭД
		k.extractKpvedCodes(line)
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	k.data = content.String()
	k.loaded = true

	return nil
}

// extractKpvedCodes извлекает коды КПВЭД из строки
func (k *KpvedProcessor) extractKpvedCodes(line string) {
	// Регулярное выражение для кодов КПВЭД типа "01.11.1", "01.12.10", "10.51.11"
	// Код может начинаться с 1-2 цифр, затем точки и еще 1-2 цифр, повторяющиеся до 3 раз
	re := regexp.MustCompile(`^(\d{1,2}(?:\.\d{1,2}){0,3})`)
	trimmed := strings.TrimSpace(line)
	matches := re.FindStringSubmatch(trimmed)

	if len(matches) > 1 {
		code := matches[1]
		k.codes[code] = true
	}
}

// GetData возвращает полный текст справочника КПВЭД
func (k *KpvedProcessor) GetData() string {
	k.mu.RLock()
	defer k.mu.RUnlock()
	return k.data
}

// CodeExists проверяет существование кода КПВЭД в справочнике
func (k *KpvedProcessor) CodeExists(code string) bool {
	k.mu.RLock()
	defer k.mu.RUnlock()
	return k.codes[code]
}

