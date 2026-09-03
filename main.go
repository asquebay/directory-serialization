package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/asquebay/directory-serialization/detector"
)

// fileInfo содержит путь к файлу и флаг, является ли он текстовым
type fileInfo struct {
	relPath string
	isText  bool
}

// buildIgnoreMap создает словарь абсолютных путей для быстрого поиска O(1).
// Она обрабатывает пути как относительно текущей директории (CWD),
// так и относительно корня сканирования
func buildIgnoreMap(root string, ignorePaths []string) map[string]bool {
	ignoreMap := make(map[string]bool)

	// Получаем и нормализуем абсолютный путь к корню сканирования
	absRoot, err := filepath.Abs(root)
	if err != nil {
		absRoot = filepath.Clean(root)
	} else {
		absRoot = filepath.Clean(absRoot)
	}

	for _, p := range ignorePaths {
		if p == "" {
			continue
		}

		// Путь относительно текущей рабочей директории (CWD).
		// Полезно, если пользователь запускает утилиту из папки проекта и пишет: -i ./temp
		if absCwd, err := filepath.Abs(p); err == nil {
			ignoreMap[filepath.Clean(absCwd)] = true
		} else {
			ignoreMap[filepath.Clean(p)] = true
		}

		// Путь относительно корня сканирования (сокращённый путь).
		// Полезно, если пользователь пишет просто имя папки: -i folder1
		// Это спасает, когда пользователь пишет `--ignore temp`, находясь в другой директории
		if !filepath.IsAbs(p) {
			absRootRel := filepath.Join(absRoot, p)
			ignoreMap[filepath.Clean(absRootRel)] = true
		}
	}

	return ignoreMap
}

// walkDir возвращает слайс структур fileInfo
func walkDir(currentDir, baseRelPath, prefix string, ignoreMap map[string]bool) ([]fileInfo, error) {
	f, err := os.Open(currentDir)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	items, err := f.Readdir(-1)
	if err != nil {
		// НЕ возвращаем ошибку, чтобы продолжить обход других директорий
		fmt.Fprintf(os.Stderr, "Error reading directory %s: %v\n", currentDir, err)
	}

	// сортируем элементы для консистентного вывода
	sort.Slice(items, func(i, j int) bool {
		// директории всегда идут первыми
		if items[i].IsDir() != items[j].IsDir() {
			return items[i].IsDir()
		}
		return items[i].Name() < items[j].Name()
	})

	// Фильтруем элементы ДО отрисовки, чтобы корректно работать с "last" (ветвлением дерева)
	var filteredItems []os.FileInfo
	for _, item := range items {
		// пропускаем .git и temp (temp я использую для всякой всячины, которую НЕ кладу в проект)
		if item.Name() == ".git" || item.Name() == "temp" {
			continue
		}

		// Проверяем, находится ли путь в списке игнорируемых
		fullPath := filepath.Join(currentDir, item.Name())

		// filepath.Abs уже делает Clean внутри, но явный Clean гарантирует
		// защиту от любых пограничных случаев со слешами
		absPath, err := filepath.Abs(fullPath)
		if err == nil && ignoreMap[filepath.Clean(absPath)] {
			continue
		}

		filteredItems = append(filteredItems, item)
	}

	var files []fileInfo
	for i, item := range filteredItems {
		last := i == len(filteredItems)-1
		name := item.Name()
		childRelPath := filepath.Join(baseRelPath, name)

		if item.IsDir() {
			// вывод для директории (этап 1)
			if last {
				fmt.Println(prefix + "\\-- " + name + "/")
			} else {
				fmt.Println(prefix + "+-- " + name + "/")
			}

			newPrefix := prefix
			if last {
				newPrefix += "    "
			} else {
				newPrefix += "|   "
			}

			fullPath := filepath.Join(currentDir, name)
			subFiles, err := walkDir(fullPath, childRelPath, newPrefix, ignoreMap)
			if err != nil {
				// ошибку логируем, но не прерываем весь процесс
				fmt.Fprintf(os.Stderr, "Error accessing %s: %v\n", fullPath, err)
			} else {
				files = append(files, subFiles...)
			}
		} else {
			// вывод для файла (этап 1)
			if last {
				fmt.Println(prefix + "\\-- " + name)
			} else {
				fmt.Println(prefix + "+-- " + name)
			}

			// определяем, является ли файл текстовым
			// (имеется в виду проверка, является ли файл "читабельным", а не бинарником или картинкой)
			fullPath := filepath.Join(currentDir, name)
			data, err := os.ReadFile(fullPath)
			isTextFile := false
			if err == nil {
				// используем функцию-обёртку для ответа (текстовый ли файл, али бинарник кракозябрный)
				isTextFile = detector.IsText(data)
			} else {
				fmt.Fprintf(os.Stderr, "Could not read file %s to determine type: %v\n", fullPath, err)
			}

			files = append(files, fileInfo{relPath: childRelPath, isText: isTextFile})
		}
	}

	return files, nil
}

func main() {
	var root string
	var ignorePaths []string

	// Ручной парсинг аргументов для поддержки флага --ignore/-i со множеством значений
	args := os.Args[1:]
	ignoreMode := false
	for _, arg := range args {
		if arg == "-i" || arg == "--ignore" {
			ignoreMode = true
			continue
		}
		if ignoreMode {
			ignorePaths = append(ignorePaths, arg)
		} else {
			if root == "" {
				root = arg
			} else {
				fmt.Fprintln(os.Stderr, "Error: Too Many Arguments. Expected: 1 argument\nОшибка: Слишком много аргументов. Ожидалось: 1 аргумент")
				os.Exit(1)
			}
		}
	}

	if root == "" {
		fmt.Fprintln(os.Stderr, "Error: Not enough arguments. Expected: 1 argument\nОшибка: Недостаточно аргументов. Ожидалось: 1 аргумент")
		os.Exit(1)
	}

	info, err := os.Stat(root)
	if os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Error: The directory %s does not exist\nОшибка: Директория %s не существует\n", root, root)
		os.Exit(1)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error accessing %s: %v\n", root, err)
		os.Exit(1)
	}
	if !info.IsDir() {
		fmt.Fprintf(os.Stderr, "Error: %s is not a directory\n", root)
		os.Exit(1)
	}

	// Формируем мапу игнорируемых путей
	ignoreMap := buildIgnoreMap(root, ignorePaths)

	// Этап 1: построение древа директории
	rootName := filepath.Base(root)
	fmt.Println(rootName + "/")

	files, err := walkDir(root, "", "", ignoreMap)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error walking directory: %v\n", err)
		os.Exit(1)
	}

	// добавляем пустую строку для визуального разделения
	fmt.Println()

	// Этап 2: вывод содержимого только текстовых файлов
	for _, file := range files {
		// пропускаем нетекстовые файлы
		if !file.isText {
			continue
		}

		fullPath := filepath.Join(root, file.relPath)
		displayPath := filepath.Join(rootName, file.relPath)
		displayPath = filepath.ToSlash(displayPath) // для вывода на Windows

		fmt.Printf("%s:\n", displayPath)
		fmt.Println("```")
		data, err := os.ReadFile(fullPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading %s: %v\n", fullPath, err)
			fmt.Printf("Error reading file: %v\n", err)
		} else {
			fmt.Println(string(data))
		}
		fmt.Println("```")
	}
}
