package rag

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/fsnotify/fsnotify"
	"github.com/go-logr/logr"
	"github.com/philippgille/chromem-go"
	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/golang"
	"github.com/smacker/go-tree-sitter/html"
	"github.com/smacker/go-tree-sitter/javascript"
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
	MAX_REPOMAP_SIZE = 16384
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
			return fmt.Errorf("error creating collection: %w", err)
		}
		collection = s.Collections[path]
	}
	// get all files in the repository
	files, err := getFiles(path)
	if err != nil {
		return fmt.Errorf("error getting files: %w", err)
	}

	// watch the repository for changes
	for _, file := range files {
		err := s.watcher.Add(file)
		if err != nil {
			return fmt.Errorf("error adding file to watcher: %w", err)
		}
		s.watchedFiles[file] = true
	}

	return s.addDocumentsToCollection(s.ctx, collection, files)
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

func (s *Store) RepoMap(path string) (string, error) {
	return s.generateRepoMap(path)
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
				err := s.addDocumentsToCollection(s.ctx, collection, []string{event.Name})
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
			err := s.watcher.Add(file)
			if err != nil {
				s.logger.Error(err, "error adding file to watcher")
				continue
			}
			s.mu.Lock()
			s.watchedFiles[file] = true
			err = s.addDocumentsToCollection(s.ctx, collection, []string{file})
			if err != nil {
				s.logger.Error(err, "error adding document")
			}
			s.mu.Unlock()
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

func (s *Store) addDocumentsToCollection(ctx context.Context, collection *chromem.Collection, files []string) error {
	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("error reading file: %w", err)
		}

		// Chunk the document
		chunks := s.chunkDocument(string(content), file)
		for _, chunk := range chunks {
			err = collection.AddDocument(ctx, chunk)
			if err != nil {
				return fmt.Errorf("error adding document chunk: %s %w", file, err)
			}
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

func (s *Store) generateRepoMap(path string) (string, error) {
	files, err := getFiles(path)
	if err != nil {
		return "", fmt.Errorf("error getting files: %w", err)
	}

	// Group files by directory
	dirMap := make(map[string][]string)
	for _, file := range files {
		dir := filepath.Dir(file)
		dirMap[dir] = append(dirMap[dir], filepath.Base(file))
	}

	// Generate initial repomap string
	var repomap strings.Builder
	repomap.WriteString("Repo structure:\n")

	// First, add a simplified directory structure
	for dir, files := range dirMap {
		// Calculate relative path
		relPath, err := filepath.Rel(path, dir)
		if err != nil {
			continue
		}
		if relPath == "." {
			relPath = ""
		}

		// Skip empty directories and common non-code directories
		if relPath == "" || strings.Contains(relPath, "test") || strings.Contains(relPath, "vendor") {
			continue
		}

		// Only include directories with Go files
		hasGoFiles := false
		for _, file := range files {
			if strings.HasSuffix(file, ".go") {
				hasGoFiles = true
				break
			}
		}
		if !hasGoFiles {
			continue
		}

		repomap.WriteString(fmt.Sprintf("%s/\n", relPath))
	}

	// Add code structure analysis for Go files
	repomap.WriteString("\nKey files:\n")

	// Rank files based on importance
	type rankedFile struct {
		path    string
		score   float64
		content string
	}

	var rankedFiles []rankedFile
	for _, file := range files {
		if !strings.HasSuffix(file, ".go") {
			continue
		}

		// Get relative path for display
		relPath, err := filepath.Rel(path, file)
		if err != nil {
			continue
		}

		// Skip test files and vendor files
		if strings.Contains(relPath, "_test.go") || strings.Contains(relPath, "vendor/") {
			continue
		}

		// Parse the file with tree-sitter
		tree, err := ParseFile(file)
		if err != nil {
			s.logger.Error(err, "Error parsing file", "file", file)
			continue
		}

		// Calculate file score based on various factors
		score := 0.0

		// Add score for having a package declaration
		if GetPackage(tree) != "" {
			score += 1.0
		}

		// Add score for having imports
		if len(GetImports(tree)) > 0 {
			score += 0.5
		}

		// Add score for having types
		if len(GetTypes(tree)) > 0 {
			score += 0.5
		}

		// Add score for having structs
		if len(GetStructs(tree)) > 0 {
			score += 0.5
		}

		// Add score for having interfaces
		if len(GetInterfaces(tree)) > 0 {
			score += 0.5
		}

		// Add score for having functions
		if len(GetFunctions(tree)) > 0 {
			score += 0.5
		}

		// Generate compact file content
		var content strings.Builder
		content.WriteString(fmt.Sprintf("\n%s:\n", relPath))

		// Get package name
		if pkg := GetPackage(tree); pkg != "" {
			content.WriteString(fmt.Sprintf("  pkg: %s\n", pkg))
		}

		// Get imports (limit to 3 most important)
		imports := GetImports(tree)
		if len(imports) > 0 {
			content.WriteString("  imports: ")
			if len(imports) > 3 {
				content.WriteString(fmt.Sprintf("%s +%d more\n", strings.Join(imports[:3], ", "), len(imports)-3))
			} else {
				content.WriteString(fmt.Sprintf("%s\n", strings.Join(imports, ", ")))
			}
		}

		// Get types (limit to 3 most important)
		types := GetTypes(tree)
		if len(types) > 0 {
			content.WriteString("  types: ")
			if len(types) > 3 {
				content.WriteString(fmt.Sprintf("%s +%d more\n", strings.Join(types[:3], ", "), len(types)-3))
			} else {
				content.WriteString(fmt.Sprintf("%s\n", strings.Join(types, ", ")))
			}
		}

		// Get structs (limit to 3 most important)
		structs := GetStructs(tree)
		if len(structs) > 0 {
			content.WriteString("  structs: ")
			if len(structs) > 3 {
				content.WriteString(fmt.Sprintf("%s +%d more\n", strings.Join(structs[:3], ", "), len(structs)-3))
			} else {
				content.WriteString(fmt.Sprintf("%s\n", strings.Join(structs, ", ")))
			}
		}

		// Get interfaces (limit to 3 most important)
		interfaces := GetInterfaces(tree)
		if len(interfaces) > 0 {
			content.WriteString("  interfaces: ")
			if len(interfaces) > 3 {
				content.WriteString(fmt.Sprintf("%s +%d more\n", strings.Join(interfaces[:3], ", "), len(interfaces)-3))
			} else {
				content.WriteString(fmt.Sprintf("%s\n", strings.Join(interfaces, ", ")))
			}
		}

		// Get functions (limit to 3 most important)
		functions := GetFunctions(tree)
		if len(functions) > 0 {
			content.WriteString("  funcs: ")
			if len(functions) > 3 {
				content.WriteString(fmt.Sprintf("%s +%d more\n", strings.Join(functions[:3], ", "), len(functions)-3))
			} else {
				content.WriteString(fmt.Sprintf("%s\n", strings.Join(functions, ", ")))
			}
		}

		rankedFiles = append(rankedFiles, rankedFile{
			path:    relPath,
			score:   score,
			content: content.String(),
		})
	}

	// Sort files by score in descending order
	sort.Slice(rankedFiles, func(i, j int) bool {
		return rankedFiles[i].score > rankedFiles[j].score
	})

	// Take only the top 5 most important files
	maxFiles := 5
	if len(rankedFiles) > maxFiles {
		rankedFiles = rankedFiles[:maxFiles]
	}

	// Add the top files to the repomap
	for _, file := range rankedFiles {
		repomap.WriteString(file.content)
	}

	return repomap.String(), nil
}

func (s *Store) chunkDocument(content string, path string) []chromem.Document {
	// Parse the file with tree-sitter
	tree, err := ParseFile(path)
	if err != nil {
		s.logger.Error(err, "Error parsing file for chunking", "file", path)
		// Fallback to line-based chunking if parsing fails
		return s.chunkByLines(content, path)
	}

	var chunks []chromem.Document
	rootNode := tree.RootNode()

	// Get file extension to determine language-specific node types
	ext := strings.ToLower(filepath.Ext(path))
	var nodeTypes []string

	switch ext {
	case ".go":
		nodeTypes = []string{
			"function_declaration",
			"type_declaration",
			"interface_type",
			"struct_type",
			"method_declaration",
			"var_declaration",
			"const_declaration",
		}
	case ".html", ".htm":
		nodeTypes = []string{
			"element",
			"script_element",
			"style_element",
			"template_element",
			"custom_element",
		}
	case ".js", ".jsx", ".mjs":
		nodeTypes = []string{
			"function_declaration",
			"method_definition",
			"class_declaration",
			"interface_declaration",
			"type_alias_declaration",
			"enum_declaration",
			"variable_declaration",
			"export_statement",
		}
	default:
		// For unsupported file types, fall back to line-based chunking
		return s.chunkByLines(content, path)
	}

	// Find all nodes of interest
	for _, nodeType := range nodeTypes {
		queryString := fmt.Sprintf("(%s) @node", nodeType)
		var language *sitter.Language
		switch ext {
		case ".go":
			language = golang.GetLanguage()
		case ".html", ".htm":
			language = html.GetLanguage()
		case ".js", ".jsx", ".mjs":
			language = javascript.GetLanguage()
		}

		q, err := sitter.NewQuery([]byte(queryString), language)
		if err != nil {
			continue
		}

		qc := sitter.NewQueryCursor()
		qc.Exec(q, rootNode)

		for {
			m, ok := qc.NextMatch()
			if !ok {
				break
			}

			for _, c := range m.Captures {
				node := c.Node
				// Get the node's content
				nodeContent := string(content[node.StartByte():node.EndByte()])

				// Skip empty nodes
				if strings.TrimSpace(nodeContent) == "" {
					continue
				}

				// Create a chunk for this node
				chunks = append(chunks, chromem.Document{
					ID:      fmt.Sprintf("%s:%d-%d", path, node.StartPoint().Row, node.EndPoint().Row),
					Content: nodeContent,
					Metadata: map[string]string{
						"path":       path,
						"start_line": fmt.Sprintf("%d", node.StartPoint().Row),
						"end_line":   fmt.Sprintf("%d", node.EndPoint().Row),
						"type":       nodeType,
					},
				})
			}
		}
	}

	// If no chunks were created, fall back to line-based chunking
	if len(chunks) == 0 {
		return s.chunkByLines(content, path)
	}

	return chunks
}

// chunkByLines is the original line-based chunking implementation
func (s *Store) chunkByLines(content string, path string) []chromem.Document {
	lines := strings.Split(content, "\n")
	var chunks []chromem.Document
	var currentChunk strings.Builder
	var currentLine int

	// Constants for chunking
	const (
		maxChunkSize = 1000 // characters
		minChunkSize = 100  // characters
		contextLines = 5    // number of lines to include for context
	)

	for i, line := range lines {
		// If adding this line would exceed maxChunkSize and we have enough content
		if currentChunk.Len()+len(line) > maxChunkSize && currentChunk.Len() >= minChunkSize {
			// Add context lines from the next chunk
			endContext := i + contextLines
			if endContext > len(lines) {
				endContext = len(lines)
			}
			nextContext := strings.Join(lines[i:endContext], "\n")

			chunks = append(chunks, chromem.Document{
				ID:      fmt.Sprintf("%s:%d-%d", path, currentLine, i),
				Content: currentChunk.String() + "\n" + nextContext,
				Metadata: map[string]string{
					"path":       path,
					"start_line": fmt.Sprintf("%d", currentLine),
					"end_line":   fmt.Sprintf("%d", i),
					"type":       "line_based",
				},
			})
			currentChunk.Reset()
			currentLine = i
		}
		currentChunk.WriteString(line + "\n")
	}

	// Add the last chunk if it has content
	if currentChunk.Len() > 0 {
		chunks = append(chunks, chromem.Document{
			ID:      fmt.Sprintf("%s:%d-%d", path, currentLine, len(lines)),
			Content: currentChunk.String(),
			Metadata: map[string]string{
				"path":       path,
				"start_line": fmt.Sprintf("%d", currentLine),
				"end_line":   fmt.Sprintf("%d", len(lines)),
				"type":       "line_based",
			},
		})
	}

	return chunks
}
