package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func walkDir(currentDir, baseRelPath, prefix string) ([]string, error) {
	f, err := os.Open(currentDir)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	items, err := f.Readdir(-1)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading directory %s: %v\n", currentDir, err)
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].Name() < items[j].Name()
	})

	var files []string
	for i, item := range items {
		last := i == len(items)-1
		name := item.Name()
		childRelPath := filepath.Join(baseRelPath, name)

		if item.IsDir() {
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
				fmt.Fprintf(os.Stderr, "Error accessing %s: %v\n", fullPath, err)
			} else {
				files = append(files, subFiles...)
			}
		} else {
			if last {
				fmt.Println(prefix + "└── " + name)
			} else {
				fmt.Println(prefix + "├── " + name)
			}
			files = append(files, childRelPath)
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

	rootName := filepath.Base(root)
	fmt.Println(rootName + "/")

	files, err := walkDir(root, "", "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error walking directory: %v\n", err)
		os.Exit(1)
	}

	for _, relPath := range files {
		displayPath := filepath.Join(rootName, relPath)
		displayPath = filepath.ToSlash(displayPath)
		fmt.Printf("%s:\n", displayPath)
		fmt.Println("```")
		data, err := os.ReadFile(filepath.Join(root, relPath))
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading %s: %v\n", filepath.Join(root, relPath), err)
		} else {
			fmt.Println(string(data))
		}
		fmt.Println("```")
	}
}
