package rag

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/fsnotify/fsnotify"
	"github.com/go-logr/logr"
	"github.com/philippgille/chromem-go"
)

var (
	EXCLUDE_DIRS = []string{
		"node_modules",
		"vendor",
		"dist",
		"build",
		"bin",
		".git",
	}
	EXCLUDE_FILES = []string{
		"README.md",
		"LICENSE",
		"CHANGELOG.md",
		"CONTRIBUTING.md",
		"CODE_OF_CONDUCT.md",
		"SECURITY.md",
		"CODEOWNERS",
		"go.mod",
		"go.sum",
		".gitignore",
		".air.toml",
		"mule.log",
		"Makefile",
	}
)

type Store struct {
	ctx          context.Context
	DB           *chromem.DB
	Collections  map[string]*chromem.Collection
	watcher      *fsnotify.Watcher
	logger       logr.Logger
	watchedFiles map[string]bool
	mu           sync.RWMutex
}

func NewStore(logger logr.Logger) *Store {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		panic(err)
	}
	store := &Store{
		ctx:          context.Background(),
		DB:           chromem.NewDB(),
		Collections:  make(map[string]*chromem.Collection),
		watcher:      watcher,
		logger:       logger,
		watchedFiles: make(map[string]bool),
		mu:           sync.RWMutex{},
	}
	go store.Watch()
	return store
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

	// watch the repository for changes
	for _, file := range files {
		err := s.watcher.Add(file)
		if err != nil {
			return err
		}
		s.watchedFiles[file] = true
	}

	return addDocumentsToCollection(s.ctx, collection, files)
}

func (s *Store) Query(path string, query string, nResults int) (string, error) {
	if s.Collections == nil {
		return "", fmt.Errorf("collections not initialized")
	}
	collection, ok := s.Collections[path]
	if !ok {
		return "", fmt.Errorf("collection %s not found", path)
	}
	s.checkFiles(path, collection)
	s.mu.RLock()
	results, err := collection.Query(s.ctx, query, nResults, nil, nil)
	s.mu.RUnlock()
	if err != nil {
		return "", err
	}
	resultsString := make([]string, len(results))
	for i, result := range results {
		resultsString[i] = toString(result)
	}
	return strings.Join(resultsString, "\n"), nil
}

func (s *Store) Watch() {
	for {
		select {
		case event, ok := <-s.watcher.Events:
			if !ok {
				return
			}
			collection, err := s.getCollection(event.Name)
			if err != nil {
				s.logger.Error(err, "error getting collection")
				continue
			}
			if event.Has(fsnotify.Write) {
				s.mu.Lock()
				err := addDocumentsToCollection(s.ctx, collection, []string{event.Name})
				s.mu.Unlock()
				if err != nil {
					s.logger.Error(err, "error adding document")
					continue
				}
			}
			if event.Has(fsnotify.Remove) {
				s.logger.Info("removing document should be implemented", "path", event.Name)
				// collection.Delete(s.ctx, map[string]string{
				// 	"path": event.Name,
				// })
			}
		case err, ok := <-s.watcher.Errors:
			if !ok {
				return
			}
			s.logger.Error(err, "error watching repository")
		}
	}
}

// this could be done differently by watching the directories
// maybe later
func (s *Store) checkFiles(path string, collection *chromem.Collection) {
	files, err := getFiles(path)
	if err != nil {
		s.logger.Error(err, "error getting files")
		return
	}
	for _, file := range files {
		watched, ok := s.watchedFiles[file]
		if !watched || !ok {
			s.mu.Lock()
			defer s.mu.Unlock()
			err := s.watcher.Add(file)
			if err != nil {
				s.logger.Error(err, "error adding file to watcher")
				continue
			}
			s.watchedFiles[file] = true
			err = addDocumentsToCollection(s.ctx, collection, []string{file})
			if err != nil {
				s.logger.Error(err, "error adding document")
			}
		}
	}
}

func (s *Store) getCollection(path string) (*chromem.Collection, error) {
	for name, collection := range s.Collections {
		if strings.HasPrefix(path, name) {
			return collection, nil
		}
	}
	return nil, fmt.Errorf("collection not found")
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
		for _, excludeFile := range EXCLUDE_FILES {
			if d.Name() == excludeFile {
				return nil
			}
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
