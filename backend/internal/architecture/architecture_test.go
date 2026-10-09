package architecture_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestCleanArchitectureDependencies(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve architecture test path")
	}
	internalRoot := filepath.Clean(filepath.Join(filepath.Dir(filename), ".."))
	rules := []struct {
		layer     string
		forbidden []string
	}{
		{
			layer: "domain",
			forbidden: []string{
				"context",
				"database/sql",
				"net/http",
				"avatar-id/internal/application",
				"avatar-id/internal/infrastructure",
				"avatar-id/internal/transport",
			},
		},
		{
			layer: "application",
			forbidden: []string{
				"database/sql",
				"net/http",
				"avatar-id/internal/infrastructure",
				"avatar-id/internal/transport",
			},
		},
		{
			layer: "infrastructure",
			forbidden: []string{
				"avatar-id/internal/transport",
			},
		},
		{
			layer: "transport",
			forbidden: []string{
				"avatar-id/internal/infrastructure",
			},
		},
	}

	for _, rule := range rules {
		rule := rule
		t.Run(rule.layer, func(t *testing.T) {
			layerRoot := filepath.Join(internalRoot, rule.layer)
			err := filepath.Walk(layerRoot, func(path string, info fs.FileInfo, walkErr error) error {
				if walkErr != nil {
					return walkErr
				}
				if info.IsDir() || !strings.HasSuffix(info.Name(), ".go") || strings.HasSuffix(info.Name(), "_test.go") {
					return nil
				}
				file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
				if err != nil {
					return err
				}
				for _, spec := range file.Decls {
					declaration, ok := spec.(*ast.GenDecl)
					if !ok || declaration.Tok != token.IMPORT {
						continue
					}
					for _, item := range declaration.Specs {
						importSpec := item.(*ast.ImportSpec)
						importPath, err := strconv.Unquote(importSpec.Path.Value)
						if err != nil {
							return err
						}
						for _, forbidden := range rule.forbidden {
							if importPath == forbidden || strings.HasPrefix(importPath, forbidden+"/") {
								t.Errorf("%s imports forbidden dependency %q", path, importPath)
							}
						}
					}
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}
