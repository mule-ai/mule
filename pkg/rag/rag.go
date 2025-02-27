package rag

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/philippgille/chromem-go"
)

var (
	EXCLUDE_DIRS = []string{
		"node_modules",
		"vendor",
		"dist",
		"build",
		".git",
	}
)

type Store struct {
	ctx         context.Context
	DB          *chromem.DB
	Collections map[string]*chromem.Collection
}

func NewStore() *Store {
	return &Store{
		ctx:         context.Background(),
		DB:          chromem.NewDB(),
		Collections: make(map[string]*chromem.Collection),
	}
}

func (s *Store) NewCollection(name string) error {
	collection, err := s.DB.CreateCollection(name, nil, chromem.NewEmbeddingFuncOllama("nomic-embed-text", "http://10.10.199.29:11434/api"))
	if err != nil {
		return err
	}
	s.Collections[name] = collection
	return nil
}

func (s *Store) AddRepository(path string) error {
	collection, ok := s.Collections[path]
	if !ok {
		if err := s.NewCollection(path); err != nil {
			return err
		}
		collection = s.Collections[path]
	}
	// get all files in the repository
	files, err := getFiles(path)
	if err != nil {
		return err
	}

	return addDocumentsToCollection(s.ctx, collection, files)
}

func (s *Store) Query(path string, query string, nResults int) (string, error) {
	collection, ok := s.Collections[path]
	if !ok {
		return "", fmt.Errorf("collection %s not found", path)
	}
	results, err := collection.Query(s.ctx, query, nResults, nil, nil)
	if err != nil {
		return "", err
	}
	resultsString := make([]string, len(results))
	for i, result := range results {
		resultsString[i] = toString(result)
	}
	return strings.Join(resultsString, "\n"), nil
}

func addDocumentsToCollection(ctx context.Context, collection *chromem.Collection, files []string) error {
	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		err = collection.AddDocument(ctx, chromem.Document{
			ID:      file,
			Content: string(content),
			Metadata: map[string]string{
				"path": file,
			},
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func getFiles(dir string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			for _, excludeDir := range EXCLUDE_DIRS {
				if d.Name() == excludeDir {
					return filepath.SkipDir
				}
			}
			return nil
		}
		files = append(files, path)
		return nil
	})
	return files, err
}

func toString(result chromem.Result) string {
	return fmt.Sprintf("File: %s\n"+
		"Content: %s\n",
		result.Metadata["path"],
		result.Content,
	)
}
