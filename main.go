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

// walkDir возвращает слайс структур fileInfo
func walkDir(currentDir, baseRelPath, prefix string) ([]fileInfo, error) {
	f, err := os.Open(currentDir)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	items, err := f.Readdir(-1)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading directory %s: %v\n", currentDir, err)
		// НЕ возвращаем ошибку, чтобы продолжить обход других директорий
	}

	// сортируем элементы для консистентного вывода
	sort.Slice(items, func(i, j int) bool {
		// директории всегда идут первыми
		if items[i].IsDir() != items[j].IsDir() {
			return items[i].IsDir()
		}
		return items[i].Name() < items[j].Name()
	})

	var files []fileInfo
	for i, item := range items {
		// пропускаем .git и temp (temp я использую для всякой всячины, которую НЕ кладу в проект)
		if item.Name() == ".git" {
			continue
		}
		if item.Name() == "temp" {
			continue
		}

		last := i == len(items)-1
		name := item.Name()
		childRelPath := filepath.Join(baseRelPath, name)

		if item.IsDir() {
			// вывод для директории (этап 1)
			if last {
				fmt.Println(prefix + "└── " + name + "/")
			} else {
				fmt.Println(prefix + "├── " + name + "/")
			}

			newPrefix := prefix
			if last {
				newPrefix += "    "
			} else {
				newPrefix += "│   "
			}

			fullPath := filepath.Join(currentDir, name)
			subFiles, err := walkDir(fullPath, childRelPath, newPrefix)
			if err != nil {
				// ошибку логируем, но не прерываем весь процесс
				fmt.Fprintf(os.Stderr, "Error accessing %s: %v\n", fullPath, err)
			} else {
				files = append(files, subFiles...)
			}
		} else {
			// вывод для файла (этап 1)
			if last {
				fmt.Println(prefix + "└── " + name)
			} else {
				fmt.Println(prefix + "├── " + name)
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
	if len(os.Args) != 2 {
		if len(os.Args) < 2 {
			fmt.Fprintln(os.Stderr, "Error: Not enough arguments. Expected: 1 argument\nОшибка: Недостаточно аргументов. Ожидалось: 1 аргумент")
		} else {
			fmt.Fprintln(os.Stderr, "Error: Too Many Arguments. Expected: 1 argument\nОшибка: Слишком много аргументов. Ожидалось: 1 аргумент")
		}
		os.Exit(1)
	}

	root := os.Args[1]
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

	// Этап 1: построение древа директории
	rootName := filepath.Base(root)
	fmt.Println(rootName + "/")

	files, err := walkDir(root, "", "")
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
