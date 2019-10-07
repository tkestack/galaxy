package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/ioutil"
	"os"
	"strings"
)

type ImportData struct {
	name *ast.Ident
	path string
}

func usage() {
	fmt.Printf("Usage: \n")
	fmt.Printf("\timport_checker <files>...\n")
}

func check(selfPackageName string, path string, data *ast.File) error {
	var (
		buf                       bytes.Buffer
		system, self, thirrdparty []ImportData
	)

	for _, importData := range data.Imports {
		if strings.Contains(importData.Path.Value, selfPackageName) {
			self = append(self, ImportData{
				name: importData.Name,
				path: importData.Path.Value,
			})
		} else if strings.Contains(importData.Path.Value, ".") {
			thirrdparty = append(thirrdparty, ImportData{
				name: importData.Name,
				path: importData.Path.Value,
			})
		} else {
			system = append(system, ImportData{
				name: importData.Name,
				path: importData.Path.Value,
			})
		}
	}

	array := [][]ImportData{}
	if len(system) > 0 {
		array = append(array, system)
	}

	if len(self) > 0 {
		array = append(array, self)
	}

	if len(thirrdparty) > 0 {
		array = append(array, thirrdparty)
	}

	if len(array) == 0 {
		return nil
	}

	buf.WriteString("import (\n")
	for i, idata := range array {
		for _, pkg := range idata {
			buf.WriteString(generatePath(pkg.name, pkg.path))
		}

		if i+1 < len(array) {
			buf.WriteString("\n")
		}
	}

	buf.WriteString(")\n")

	standard := buf.String()
	originFile, err := os.Open(path)
	if err != nil {
		return err
	}
	defer originFile.Close()

	contents, err := ioutil.ReadAll(originFile)
	if err != nil {
		return err
	}

	if !strings.Contains(string(contents), standard) {
		return fmt.Errorf("%s\nImport is not standardized, should be\n%s", path, standard)
	}

	return nil
}

func generatePath(name *ast.Ident, path string) string {
	if name == nil {
		return fmt.Sprintf("\t%s\n", path)
	}

	return fmt.Sprintf("\t%s %s\n", name, path)
}

func main() {
	if len(os.Args) < 1 {
		usage()
		os.Exit(0)
	}

	selfPackage := os.Getenv("PACKAGE_NAME")

	if selfPackage == "" {
		fmt.Printf("Please set PACKAGE_NAME\n")
		os.Exit(1)
	}

	for _, file := range os.Args[1:] {
		fset := token.NewFileSet()

		data, err := parser.ParseFile(fset, file, nil, parser.ImportsOnly)
		if err != nil {
			fmt.Printf("Can not parse %s, %+v\n", file, err)
			os.Exit(1)
		}

		if err := check(selfPackage, file, data); err != nil {
			fmt.Printf("You need to format your code\n%+v\n", err)
			os.Exit(1)
		}
	}

	os.Exit(0)
}
