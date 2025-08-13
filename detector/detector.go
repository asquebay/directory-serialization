package detector

import (
	"bytes"
	"regexp"
	"strings"
	"unicode/utf8"
)

// код в этом файле является портом нескольких файлов из исходников KDE/Kate
// (см. https://api.kde.org/legacy/4.14-api/kdelibs-apidocs/kdecore/html/kencodingdetector_8cpp_source.html и его зависимости)

// EncodingChoiceSource указывает источник, из которого была определена кодировка
// Аналог KEncodingDetector::EncodingChoiceSource
type EncodingChoiceSource int

const (
	DefaultEncoding       EncodingChoiceSource = iota // кодировка по умолчанию
	AutoDetectedEncoding                              // кодировка определена эвристически
	BOM                                               // обнаружен BOM (Byte Order Mark)
	EncodingFromXMLHeader                             // из заголовка XML
	EncodingFromMetaTag                               // из мета-тега HTML
	UserChosenEncoding                                // указано пользователем (для полноты API)
)

// AutoDetectScript указывает языковую группу для эвристического анализа
// Аналог KEncodingDetector::AutoDetectScript
type AutoDetectScript int

const (
	None AutoDetectScript = iota
	// SemiautomaticDetection // полуавтоматическое определение
	Arabic
	Baltic
	CentralEuropean
	ChineseSimplified
	ChineseTraditional
	Cyrillic
	Greek
	Hebrew
	Japanese
	Korean
	Turkish
	WesternEuropean
	Unicode
)

// DetectorResult содержит результат анализа
type DetectorResult struct {
	Encoding string
	Source   EncodingChoiceSource
	Script   AutoDetectScript
	IsBinary bool
}

const maxBuffer = 16 * 1024 // максимальный размер буфера для анализа

// isBinary проверяет, является ли файл бинарным, ища нулевые байты
// Аналог KEncodingDetector::processNull.
func isBinary(data []byte) bool {
	// для UTF-16 нулевые байты — норма, но мы проверяем их на этапе анализа BOM
	// для других кодировок наличие \0 — сильный признак бинарного файла
	// проверяем первые 8000 байт, как это делают многие утилиты (например, git)
	checkLen := len(data)
	if checkLen > 8000 {
		checkLen = 8000
	}
	return bytes.Contains(data[:checkLen], []byte{0})
}

// errorsIfUtf8 проверяет, содержит ли срез байт ошибки, если его считать UTF-8
// Аналог KEncodingDetector::errorsIfUtf8.\
func errorsIfUtf8(data []byte) bool {
	return !utf8.Valid(data)
}

// EncodingDetector — ключевая функция, которая анализирует содержимое файла
// она пытается определить кодировку, используя ту же последовательность проверок, что и Kate
func EncodingDetector(data []byte, scriptHint AutoDetectScript) *DetectorResult {
	result := &DetectorResult{
		Encoding: "binary", // по умолчанию считаем бинарным
		Source:   DefaultEncoding,
		IsBinary: true,
	}

	if len(data) == 0 {
		result.Encoding = "us-ascii" // пустой файл можно считать текстовым
		result.IsBinary = false
		return result
	}

	// 1. Проверка на BOM
	if enc, ok := checkBOM(data); ok {
		result.Encoding = enc
		result.Source = BOM
		// файлы с BOM определенно текстовые
		result.IsBinary = false
		return result
	}

	// 2. Проверка на бинарность (поиск нулевых байтов)
	// это самый быстрый и эффективный способ отсеять исполняемые файлы, изображения и т.д.
	if isBinary(data) {
		return result // возвращаем результат по умолчанию: binary=true
	}

	// если дошли сюда, файл скорее всего текстовый. Угадываем кодировку
	result.IsBinary = false
	result.Encoding = "us-ascii" // предварительное предположение

	// 3. Эвристический анализ на основе языковой группы
	// в Kate здесь еще есть парсинг XML/HTML, но он мне не нужен
	checkLen := len(data)
	if checkLen > maxBuffer {
		checkLen = maxBuffer
	}
	sample := data[:checkLen]

	// если есть подсказка, используем её
	// в реальном KEncodingDetector есть еще и autoDetectLanguage,
	// который может быть SemiautomaticDetection, но эта муторная логика нафиг не нужна
	if scriptHint != None {
		detectedEnc := runHeuristics(sample, scriptHint)
		if detectedEnc != "" {
			result.Encoding = detectedEnc
			result.Source = AutoDetectedEncoding
			result.Script = scriptHint
			return result
		}
	}

	// если подсказки нет или она не помогла, попробуем угадать сами
	// пробуем для WesternEuropean, так как это частый случай
	if enc := automaticDetectionForWesternEuropean(sample); enc != "" {
		result.Encoding = enc
		result.Source = AutoDetectedEncoding
		result.Script = WesternEuropean
		return result
	}

	// пробуем кириллицу
	if enc := automaticDetectionForCyrillic(sample); enc != "" {
		result.Encoding = enc
		result.Source = AutoDetectedEncoding
		result.Script = Cyrillic
		return result
	}

	// пробуем японский
	if enc := automaticDetectionForJapanese(sample); enc != "" {
		result.Encoding = enc
		result.Source = AutoDetectedEncoding
		result.Script = Japanese
		return result
	}

	// если ничего не подошло, остаётся наше первоначальное предположение "us-ascii"
	// проверим, валиден ли файл как UTF-8. Если да, то это UTF-8 без BOM
	if !errorsIfUtf8(data) {
		result.Encoding = "UTF-8"
		result.Source = AutoDetectedEncoding
	}

	return result
}

// IsText — это простая обертка над EncodingDetector для ответа на вопрос — текстовый/нетекстовый
// (что значит текстовый/нетекстовый — см. main.go)
// IsText возвращает true, если файл похож на текстовый, и false в противном случае
func IsText(data []byte) bool {
	// для простоты и производительности, можно ограничиться проверкой на нулевые байты
	// это покрывает 99% случаев определения бинарных файлов (исполняемые файлы, архивы, изображения)
	if len(data) == 0 {
		return true // пустой файл считаем текстовым
	}
	return !isBinary(data)
}

func checkBOM(data []byte) (string, bool) {
	if bytes.HasPrefix(data, []byte{0xEF, 0xBB, 0xBF}) {
		return "UTF-8-BOM", true
	}
	if bytes.HasPrefix(data, []byte{0xFE, 0xFF}) {
		return "UTF-16BE", true
	}
	if bytes.HasPrefix(data, []byte{0xFF, 0xFE}) {
		return "UTF-16LE", true
	}
	// другие BOM (UTF-32 и т.д.) встречаются реже, для простоты опускаем
	return "", false
}

func runHeuristics(sample []byte, script AutoDetectScript) string {
	switch script {
	case Arabic:
		return automaticDetectionForArabic(sample)
	case Baltic:
		return automaticDetectionForBaltic(sample)
	case CentralEuropean:
		return automaticDetectionForCentralEuropean(sample)
	case Cyrillic:
		return automaticDetectionForCyrillic(sample)
	case Greek:
		return automaticDetectionForGreek(sample)
	case Hebrew:
		return automaticDetectionForHebrew(sample)
	case Japanese:
		return automaticDetectionForJapanese(sample)
	case Turkish:
		return automaticDetectionForTurkish(sample)
	case WesternEuropean:
		return automaticDetectionForWesternEuropean(sample)
	}
	return ""
}

// ниже идут портированные эвристические функции из kencodingdetector.cpp

func automaticDetectionForWesternEuropean(ptr []byte) string {
	size := len(ptr)
	if size == 0 {
		return ""
	}
	nonANSICount := 0
	for i := 0; i < size-1; i++ {
		if ptr[i] > 0x79 {
			nonANSICount++
			if ptr[i] > 0xc1 && ptr[i] < 0xf0 && ptr[i+1] > 0x7f && ptr[i+1] < 0xc0 {
				return "UTF-8"
			}
			if ptr[i] >= 0x78 && ptr[i] <= 0x9F {
				return "cp1252"
			}
		}
	}
	if nonANSICount > 0 {
		return "iso-8859-15"
	}
	return "" // Could be plain ASCII
}

func automaticDetectionForCyrillic(ptr []byte) string {
	size := len(ptr)
	var utf8Mark, koiScore, cp1251Score int
	var koiSt, cp1251St int
	var cp1251SmallRange, koiSmallRange, ibm866SmallRange int

	limit := size
	if cp1251SmallRange+koiSmallRange < 1000 {
		if limit > 1000 {
			limit = 1000
		}
	}

	for i := 1; i < limit; i++ {
		p := ptr[i]
		switch {
		case p > 0xdf:
			cp1251SmallRange++
			if p == 0xee {
				cp1251Score++
			} else if p == 0xf2 && ptr[i-1] == 0xf1 {
				cp1251St++
			}
		case p > 0xbf:
			koiSmallRange++
			if p == 0xd0 || p == 0xd1 {
				utf8Mark++
			}
			if p == 0xcf {
				koiScore++
			} else if p == 0xd4 && ptr[i-1] == 0xd3 {
				koiSt++
			}
		case p > 0x9f && p < 0xb0:
			ibm866SmallRange++
		}
	}

	if cp1251SmallRange+koiSmallRange+ibm866SmallRange < 8 {
		return ""
	}
	if 3*utf8Mark > cp1251SmallRange+koiSmallRange+ibm866SmallRange {
		return "UTF-8"
	}
	if ibm866SmallRange > cp1251SmallRange+koiSmallRange {
		return "ibm866"
	}

	if cp1251St == 0 && koiSt > 1 {
		koiScore += 10
	} else if koiSt == 0 && cp1251St > 1 {
		cp1251Score += 10
	}

	if cp1251Score > koiScore {
		return "cp1251"
	}
	return "koi8-u"
}

func automaticDetectionForJapanese(ptr []byte) string {
	// эта функция вызывает сложную логику из guess_ja.go
	kc := newJapaneseCode()
	code := kc.guessJP(ptr)

	switch code {
	case JapaneseCodeJIS:
		return "jis7"
	case JapaneseCodeEUC:
		return "eucjp"
	case JapaneseCodeSJIS:
		return "sjis"
	case JapaneseCodeUTF8:
		return "utf8"
	default:
		return ""
	}
}

// остальные эвристики: Arabic, Baltic, CentralEuropean, Greek, Hebrew, Turkish
// Я их реализую по аналогии с WesternEuropean, чтобы было как в Kate, но хз зачем
// (может в других проектах пригодятся)

func automaticDetectionForArabic(ptr []byte) string {
	for _, p := range ptr {
		if (p >= 0x80 && p <= 0x9F) || p == 0xA1 || p == 0xA2 || p == 0xA3 || (p >= 0xA5 && p <= 0xAB) || (p >= 0xAE && p <= 0xBA) || p == 0xBC || p == 0xBD || p == 0xBE || p == 0xC0 || (p >= 0xDB && p <= 0xDF) || (p >= 0xF3) {
			return "cp1256"
		}
	}
	return "iso-8859-6"
}

func automaticDetectionForBaltic(ptr []byte) string {
	for _, p := range ptr {
		if p >= 0x80 && p <= 0x9E {
			return "cp1257"
		}
		if p == 0xA1 || p == 0xA5 {
			return "iso-8859-13"
		}
	}
	return "iso-8859-13"
}

func automaticDetectionForCentralEuropean(ptr []byte) string {
	charset := ""
	for i, p := range ptr {
		if p >= 0x80 && p <= 0x9F {
			if p == 0x81 || p == 0x83 || p == 0x90 || p == 0x98 {
				return "ibm852"
			}
			if i+1 > len(ptr) {
				return "cp1250"
			}
			charset = "cp1250"
			continue
		}
		if p == 0xA5 || p == 0xAE || p == 0xBE || p == 0xC3 || p == 0xD0 || p == 0xE3 || p == 0xF0 {
			if i+1 > len(ptr) {
				return "iso-8859-2"
			}
			if charset == "" {
				charset = "iso-8859-2"
			}
			continue
		}
	}
	if charset == "" {
		return "iso-8859-3"
	}
	return charset
}

func automaticDetectionForGreek(ptr []byte) string {
	for _, p := range ptr {
		if p == 0x80 || (p >= 0x82 && p <= 0x87) || p == 0x89 || p == 0x8B || (p >= 0x91 && p <= 0x97) || p == 0x99 || p == 0x9B || p == 0xA4 || p == 0xA5 || p == 0xAE {
			return "cp1253"
		}
	}
	return "iso-8859-7"
}

func automaticDetectionForHebrew(ptr []byte) string {
	for _, p := range ptr {
		if p == 0x80 || (p >= 0x82 && p <= 0x89) || p == 0x8B || (p >= 0x91 && p <= 0x99) || p == 0x9B || p == 0xA1 || (p >= 0xBF && p <= 0xC9) || (p >= 0xCB && p <= 0xD8) {
			return "cp1255"
		}
		if p == 0xDF {
			return "iso-8859-8-i"
		}
	}
	return "iso-8859-8-i"
}

func automaticDetectionForTurkish(ptr []byte) string {
	for _, p := range ptr {
		if p == 0x80 || (p >= 0x82 && p <= 0x8C) || (p >= 0x91 && p <= 0x9C) || p == 0x9F {
			return "cp1254"
		}
	}
	return "iso-8859-9"
}

// регулярное выражение для поиска кодировки
var xmlEncodingRegex = regexp.MustCompile(`encoding=["']([^"']+)["']`)

// заглушки для функций, которые в Kate сложнее из-за интеграции с грёбаным Qt
// тут они не нужны, но названия сохранил для сопоставления
func findXMLEncoding(data []byte) string {
	s := string(data)
	match := strings.Index(s, `<?xml`)
	if match != -1 {
		end := strings.Index(s[match:], `?>`)
		if end != -1 {
			header := s[match : match+end]
			submatches := xmlEncodingRegex.FindStringSubmatch(header)
			if len(submatches) > 1 {
				return submatches[1] // возвращаем первую захваченную группу
			}
		}
	}
	return ""
}
