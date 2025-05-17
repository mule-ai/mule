package rag

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/fsnotify/fsnotify"
	"github.com/go-logr/logr"
	"github.com/philippgille/chromem-go"
	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/css"
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

type AggregatedResults struct {
	Results map[string][]chromem.Result
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
	collection := s.DB.GetCollection(name, chromem.NewEmbeddingFuncOpenAICompat("https://10.10.199.29:8080/v1", "api-key", "text-embedding-nomic-embed-text-v1.5", nil))
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

// deduplicateResults merges overlapping results and keeps the one with the largest range
func deduplicateResults(results []chromem.Result) []chromem.Result {
	if len(results) <= 1 {
		return results
	}

	// Sort results by start line
	sort.Slice(results, func(i, j int) bool {
		return results[i].Metadata["start_line"] < results[j].Metadata["start_line"]
	})

	var deduped []chromem.Result
	current := results[0]

	for i := 1; i < len(results); i++ {
		next := results[i]
		currentStart, _ := strconv.Atoi(current.Metadata["start_line"])
		currentEnd, _ := strconv.Atoi(current.Metadata["end_line"])
		nextStart, _ := strconv.Atoi(next.Metadata["start_line"])
		nextEnd, _ := strconv.Atoi(next.Metadata["end_line"])

		// If the next result overlaps with the current one
		if nextStart <= currentEnd {
			// Keep the result with the larger range
			if nextEnd-currentStart > currentEnd-currentStart {
				current = next
			}
		} else {
			// No overlap, add current and move to next
			deduped = append(deduped, current)
			current = next
		}
	}
	// Add the last result
	deduped = append(deduped, current)

	return deduped
}

func (s *Store) GetNResults(path string, query string, nResults int) (AggregatedResults, error) {
	if s.Collections == nil {
		return AggregatedResults{}, fmt.Errorf("collections not initialized")
	}
	collection, ok := s.Collections[path]
	if !ok {
		return AggregatedResults{}, fmt.Errorf("collection %s not found", path)
	}
	s.checkFiles(path, collection)
	s.mu.RLock()
	// Request more results initially to account for potential duplicates
	// We request 2x the desired number to ensure we have enough after deduplication
	results, err := collection.Query(s.ctx, query, nResults*2, nil, nil)
	s.mu.RUnlock()
	if err != nil {
		return AggregatedResults{}, err
	}
	aggregatedResults := AggregatedResults{
		Results: make(map[string][]chromem.Result),
	}

	// Group results by file path
	for _, result := range results {
		aggregatedResults.Results[result.Metadata["path"]] = append(aggregatedResults.Results[result.Metadata["path"]], result)
	}

	// Deduplicate results for each file
	for filePath, fileResults := range aggregatedResults.Results {
		aggregatedResults.Results[filePath] = deduplicateResults(fileResults)
	}

	// If we have more results than requested after deduplication, trim them
	totalResults := 0
	for _, fileResults := range aggregatedResults.Results {
		totalResults += len(fileResults)
	}

	if totalResults > nResults {
		// Sort files by number of results to prioritize files with more results
		type fileResultCount struct {
			path    string
			count   int
			results []chromem.Result
		}
		var fileCounts []fileResultCount
		for path, results := range aggregatedResults.Results {
			fileCounts = append(fileCounts, fileResultCount{path, len(results), results})
		}
		sort.Slice(fileCounts, func(i, j int) bool {
			return fileCounts[i].count > fileCounts[j].count
		})

		// Reset aggregated results
		aggregatedResults.Results = make(map[string][]chromem.Result)
		remainingResults := nResults

		// Distribute results across files while maintaining the requested total
		for _, fc := range fileCounts {
			if remainingResults <= 0 {
				break
			}
			// Calculate how many results to take from this file
			// Use a proportional distribution based on the original count
			proportion := float64(fc.count) / float64(totalResults)
			resultsToTake := int(float64(nResults) * proportion)
			if resultsToTake < 1 {
				resultsToTake = 1 // Ensure at least one result per file
			}
			if resultsToTake > remainingResults {
				resultsToTake = remainingResults
			}
			aggregatedResults.Results[fc.path] = fc.results[:resultsToTake]
			remainingResults -= resultsToTake
		}
	}

	return aggregatedResults, nil
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
		"Lines: %s-%s\n"+
		"Content: %s\n",
		result.Metadata["path"],
		result.Metadata["start_line"],
		result.Metadata["end_line"],
		result.Content,
	)
}

// GenerateRepoMap generates a map of the repository structure and key files
func (s *Store) GenerateRepoMap(path string, keyFiles AggregatedResults) (string, error) {
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

		// Include directory if it contains a key file, otherwise skip empty/test/vendor directories
		hasKeyFile := false
		for keyFilePath := range keyFiles.Results {
			if strings.HasPrefix(keyFilePath, dir) {
				hasKeyFile = true
				break
			}
		}

		if !hasKeyFile && (relPath == "" || strings.Contains(relPath, "test") || strings.Contains(relPath, "vendor")) {
			continue
		}

		// Add directory
		repomap.WriteString(fmt.Sprintf("%s/\n", relPath))

		// Add only key files in this directory
		for _, file := range files {
			fullPath := filepath.Join(dir, file)
			if _, ok := keyFiles.Results[fullPath]; ok {
				repomap.WriteString(fmt.Sprintf("  %s\n", file))
			}
		}
	}

	// Add key files section
	repomap.WriteString("\nKey files:\n")

	for filePath, results := range keyFiles.Results {

		// Get relative path for display
		relPath, err := filepath.Rel(path, filePath)
		if err != nil {
			continue
		}

		// Skip test files and vendor files
		if strings.Contains(relPath, "_test.go") || strings.Contains(relPath, "vendor/") {
			continue
		}

		// Parse the file with tree-sitter
		tree, err := ParseFile(filePath)
		if err != nil {
			s.logger.Error(err, "Error parsing file", "file", filePath)
			continue
		}

		content, err := getFileContent(relPath, tree, results)
		if err != nil {
			s.logger.Error(err, "Error getting file content", "file", filePath)
			continue
		}
		repomap.WriteString(content)
	}

	return repomap.String(), nil
}

func getFileContent(path string, tree *sitter.Tree, results []chromem.Result) (string, error) {
	// Generate compact file content
	var content strings.Builder
	content.WriteString(fmt.Sprintf("\n%s:\n", path))

	// sort results by start_line
	sort.Slice(results, func(i, j int) bool {
		return results[i].Metadata["start_line"] < results[j].Metadata["start_line"]
	})

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

	for _, result := range results {
		// Add the line number range from the key file
		content.WriteString(fmt.Sprintf("  lines: %s-%s\n", result.Metadata["start_line"], result.Metadata["end_line"]))
		// Add the content from the key file
		content.WriteString("  content:\n")
		content.WriteString(fmt.Sprintf("    %s\n", result.Content))
	}

	return content.String(), nil
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
	case ".css", ".scss", ".sass", ".less":
		nodeTypes = []string{
			"rule_set",
			"at_rule",
			"keyframes",
			"media_query",
			"supports",
			"import",
			"namespace",
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
		case ".css", ".scss", ".sass", ".less":
			language = css.GetLanguage()
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

				// Skip nodes with invalid byte ranges
				if (node.StartByte() > uint32(len(content)) || node.EndByte() > uint32(len(content))) ||
					(node.EndByte() < node.StartByte()) {
					s.logger.Error(fmt.Errorf("invalid byte range: %d-%d", node.StartByte(), node.EndByte()), "skipping context due to invalid byte range")
					continue
				}

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
