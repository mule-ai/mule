package agent

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/go-logr/logr"
)

// UDiffSettings stores configuration for udiff application
type UDiffSettings struct {
	Enabled bool `json:"enabled"`
}

// UDiffFile represents a file change in a unified diff
type UDiffFile struct {
	OldFile   string
	NewFile   string
	Hunks     []*UDiffHunk
	IsNewFile bool
	IsDeleted bool
}

// UDiffHunk represents a change section within a file
type UDiffHunk struct {
	StartLine int
	Lines     []string
}

// ParseUDiffs extracts udiffs from agent response text
func ParseUDiffs(text string) ([]*UDiffFile, error) {
	diffFiles := []*UDiffFile{}

	// Regular expression to match diff header
	// This pattern needs to match both standalone headers and headers followed by hunk info
	diffHeaderRegex := regexp.MustCompile(`(?m)^---\s+(\S+).*?\n\+\+\+\s+(\S+)`)

	// Find all diff blocks in the text
	diffMatches := diffHeaderRegex.FindAllStringIndex(text, -1)
	if len(diffMatches) == 0 {
		return nil, nil // No diffs found
	}

	// Process each diff block
	for i, match := range diffMatches {
		startIdx := match[0]
		endIdx := len(text)
		if i < len(diffMatches)-1 {
			endIdx = diffMatches[i+1][0]
		}

		diffBlock := text[startIdx:endIdx]
		diffFile, err := parseUDiffBlock(diffBlock)
		if err != nil {
			return nil, err
		}

		diffFiles = append(diffFiles, diffFile)
	}

	return diffFiles, nil
}

// parseUDiffBlock parses a single udiff block
func parseUDiffBlock(diffBlock string) (*UDiffFile, error) {
	// Find file paths
	diffHeaderRegex := regexp.MustCompile(`(?m)^---\s+(\S+).*?\n\+\+\+\s+(\S+)`)
	matches := diffHeaderRegex.FindStringSubmatch(diffBlock)
	if len(matches) != 3 {
		return nil, fmt.Errorf("invalid diff header format")
	}

	oldFile := matches[1]
	newFile := matches[2]

	// Create the diff file struct
	diffFile := &UDiffFile{
		OldFile: oldFile,
		NewFile: newFile,
		Hunks:   []*UDiffHunk{},
	}

	// Check if this is a new file or deletion
	if oldFile == "/dev/null" {
		diffFile.IsNewFile = true
	} else if newFile == "/dev/null" {
		diffFile.IsDeleted = true
	}

	// Parse the hunks
	scanner := bufio.NewScanner(strings.NewReader(diffBlock))
	var currentHunk *UDiffHunk
	var inHunk bool

	// Skip the header lines
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "@@") {
			inHunk = true
			// Start of a hunk
			// Handle both standard format @@ -a,b +c,d @@ and line-only format @@ -a +b @@
			lineInfoRegex := regexp.MustCompile(`@@ -\d+(?:,\d+)? \+(\d+)(?:,\d+)? @@`)
			hunckMatches := lineInfoRegex.FindStringSubmatch(line)
			if len(hunckMatches) >= 2 {
				startLine := 0
				_, err := fmt.Sscanf(hunckMatches[1], "%d", &startLine)
				if err != nil {
					return nil, fmt.Errorf("failed to parse start line: %w", err)
				}

				currentHunk = &UDiffHunk{
					StartLine: startLine,
					Lines:     []string{},
				}
				diffFile.Hunks = append(diffFile.Hunks, currentHunk)
			}
			continue
		}

		// If we're in a hunk section but haven't found the actual @@ line yet, continue
		if !inHunk {
			continue
		}

		if currentHunk != nil {
			if strings.HasPrefix(line, "+") || strings.HasPrefix(line, " ") {
				// Only keep added or context lines for applying to new file
				content := line[1:] // Remove the prefix (+ or space)
				currentHunk.Lines = append(currentHunk.Lines, content)
			}
		}
	}

	// If we have no hunks but we found a valid diff header,
	// this might be a line-only diff with a specific format
	if len(diffFile.Hunks) == 0 {
		// Try to find line-only hunks like @@ -53,0 +54,1 @@
		hunkRegex := regexp.MustCompile(`@@ -(\d+),(\d+) \+(\d+),(\d+) @@`)
		scanner = bufio.NewScanner(strings.NewReader(diffBlock))
		for scanner.Scan() {
			line := scanner.Text()
			hunckMatches := hunkRegex.FindStringSubmatch(line)
			if len(hunckMatches) == 5 {
				// Format: @@ -oldLineStart,oldLineCount +newLineStart,newLineCount @@
				newLineStart, _ := strconv.Atoi(hunckMatches[3])

				// Skip to the next line to get content
				if scanner.Scan() {
					contentLine := scanner.Text()
					if strings.HasPrefix(contentLine, "+") {
						// Found an added line
						currentHunk = &UDiffHunk{
							StartLine: newLineStart,
							Lines:     []string{contentLine[1:]}, // Remove the "+" prefix
						}
						diffFile.Hunks = append(diffFile.Hunks, currentHunk)
					}
				}
			}
		}
	}

	return diffFile, nil
}

// validateTargetPath ensures the target path is within the allowed base path
func validateTargetPath(targetPath, basePath string, logger logr.Logger) (string, error) {
	// Clean paths to resolve any . or .. components
	basePath = filepath.Clean(basePath)
	targetPath = filepath.Clean(targetPath)

	// For a/b style paths, extract just the filename
	if strings.HasPrefix(targetPath, "a/") || strings.HasPrefix(targetPath, "b/") {
		targetPath = targetPath[2:]
	}

	// Remove any leading slashes to prevent absolute path tricks
	targetPath = strings.TrimPrefix(targetPath, "/")

	// Join paths and get absolute paths
	absBasePath, err := filepath.Abs(basePath)
	if err != nil {
		logger.Error(err, "failed to get absolute base path", "basePath", basePath)
		return "", fmt.Errorf("failed to get absolute base path: %w", err)
	}

	absTargetPath := filepath.Join(absBasePath, targetPath)

	// Get canonical paths (resolves symlinks)
	canonicalBase, err := filepath.EvalSymlinks(absBasePath)
	if err != nil {
		// If base doesn't exist, that's a serious error
		logger.Error(err, "base path does not exist or cannot be evaluated", "basePath", absBasePath)
		return "", fmt.Errorf("base path does not exist or cannot be evaluated: %w", err)
	}

	// For the target, we'll use the parent directory since the file might not exist yet
	targetDir := filepath.Dir(absTargetPath)
	canonicalTargetDir, err := filepath.EvalSymlinks(targetDir)
	if err != nil {
		// If parent directory doesn't exist, create it
		if os.IsNotExist(err) {
			canonicalTargetDir = targetDir
		} else {
			logger.Error(err, "target directory cannot be evaluated", "targetDir", targetDir)
			return "", fmt.Errorf("target directory cannot be evaluated: %w", err)
		}
	}

	// Check if target directory is within base path
	if !strings.HasPrefix(canonicalTargetDir, canonicalBase) {
		logger.Error(nil, "invalid target path: outside base directory",
			"targetDir", canonicalTargetDir,
			"basePath", canonicalBase)
		return "", fmt.Errorf("invalid target path: %s is outside of base path %s", targetPath, basePath)
	}

	// Additional security check using Rel
	relPath, err := filepath.Rel(canonicalBase, canonicalTargetDir)
	if err != nil || strings.HasPrefix(relPath, "..") {
		logger.Error(err, "invalid target path: relative path check failed",
			"targetPath", targetPath,
			"basePath", basePath)
		return "", fmt.Errorf("invalid target path: relative path check failed for %s", targetPath)
	}

	// Return the validated absolute target path
	return absTargetPath, nil
}

// ApplyUDiffs applies the parsed udiffs to the specified base path
func ApplyUDiffs(diffs []*UDiffFile, basePath string, logger logr.Logger) error {
	for _, diff := range diffs {
		targetPath := diff.NewFile

		// Validate and get absolute target path
		absTargetPath, err := validateTargetPath(targetPath, basePath, logger)
		if err != nil {
			return err
		}

		if diff.IsDeleted {
			// Handle file deletion
			if err := os.Remove(absTargetPath); err != nil && !os.IsNotExist(err) {
				logger.Error(err, "failed to delete file", "targetPath", targetPath)
				return fmt.Errorf("failed to delete file %s: %w", absTargetPath, err)
			}

			// Check if the directory is now empty, if so remove it
			dir := filepath.Dir(absTargetPath)
			files, err := os.ReadDir(dir)
			if err == nil && len(files) == 0 {
				// Directory is empty, remove it
				if err := os.Remove(dir); err != nil {
					// This is not critical, so just log it
					logger.Error(err, "failed to remove empty directory", "directory", dir)
				}
			}

			continue
		}

		// For new files or modified files, ensure directory exists
		dir := filepath.Dir(absTargetPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			logger.Error(err, "failed to create directory", "directory", dir)
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}

		// For entirely new files or complete rewrites, we can just write the content directly
		if diff.IsNewFile {
			// Combine all lines from all hunks
			var content strings.Builder
			for _, hunk := range diff.Hunks {
				for _, line := range hunk.Lines {
					content.WriteString(line)
					content.WriteString("\n")
				}
			}

			if err := os.WriteFile(absTargetPath, []byte(content.String()), 0644); err != nil {
				logger.Error(err, "failed to write file", "targetPath", targetPath)
				return fmt.Errorf("failed to write file %s: %w", absTargetPath, err)
			}
			continue
		}

		// For existing files that need modification, read the current content
		fileContent, err := os.ReadFile(absTargetPath)
		if err != nil && !os.IsNotExist(err) {
			logger.Error(err, "failed to read file", "targetPath", targetPath)
			return fmt.Errorf("failed to read file %s: %w", absTargetPath, err)
		}

		// If the file doesn't exist but we're trying to modify it, treat it as a new file
		if os.IsNotExist(err) {
			// Create an empty file
			var content strings.Builder
			for _, hunk := range diff.Hunks {
				for _, line := range hunk.Lines {
					content.WriteString(line)
					content.WriteString("\n")
				}
			}

			if err := os.WriteFile(absTargetPath, []byte(content.String()), 0644); err != nil {
				logger.Error(err, "failed to write file", "targetPath", targetPath)
				return fmt.Errorf("failed to write file %s: %w", absTargetPath, err)
			}
			continue
		}

		// We have an existing file that needs modification
		currentLines := strings.Split(string(fileContent), "\n")
		// Remove trailing empty line if present
		if len(currentLines) > 0 && currentLines[len(currentLines)-1] == "" {
			currentLines = currentLines[:len(currentLines)-1]
		}

		// Apply the hunks to the current content
		newContent, err := applyHunksByContent(currentLines, diff.Hunks, logger)
		if err != nil {
			logger.Error(err, "failed to apply hunks", "targetPath", targetPath)
			return fmt.Errorf("failed to apply hunks to %s: %w", absTargetPath, err)
		}

		// Check if the file is now empty
		if len(newContent) == 0 {
			// Delete the file if it's now empty
			if err := os.Remove(absTargetPath); err != nil && !os.IsNotExist(err) {
				logger.Error(err, "failed to delete file", "targetPath", targetPath)
				return fmt.Errorf("failed to delete file %s: %w", absTargetPath, err)
			}

			// Check if the directory is now empty, if so remove it
			dir := filepath.Dir(absTargetPath)
			files, err := os.ReadDir(dir)
			if err == nil && len(files) == 0 {
				// Directory is empty, remove it
				if err := os.Remove(dir); err != nil {
					// This is not critical, so just log it
					logger.Error(err, "failed to remove empty directory", "directory", dir)
				}
			}
		} else {
			// Write the updated content back to the file
			if err := os.WriteFile(absTargetPath, []byte(strings.Join(newContent, "\n")), 0644); err != nil {
				logger.Error(err, "failed to write file", "targetPath", targetPath)
				return fmt.Errorf("failed to write file %s: %w", absTargetPath, err)
			} else {
				logger.Info("wrote file", "targetPath", targetPath)
			}
		}
	}

	return nil
}

// applyHunksByContent applies hunks by scanning the file content to find the right location
// rather than relying on line numbers from the diff
func applyHunksByContent(currentLines []string, hunks []*UDiffHunk, logger logr.Logger) ([]string, error) {
	if len(hunks) == 0 {
		return currentLines, nil
	}

	// Copy the current content to avoid modifying the original
	result := make([]string, len(currentLines))
	copy(result, currentLines)

	// Process each hunk
	for _, hunk := range hunks {
		// First, extract the context lines from the hunk (lines that should already exist)
		contextLines, addedLines := extractContextAndAddedLines(hunk.Lines)

		// If there are no context lines, we can't do smart matching
		if len(contextLines) == 0 {
			// Fall back to line number from diff
			logger.Info("no context lines found in hunk, using line number from diff",
				"lineNumber", hunk.StartLine)
			result = applyHunkAtPosition(result, hunk, hunk.StartLine-1)
			continue
		}

		// Try to find the best match for the context lines
		positions, err := findContextMatches(result, contextLines)
		if err != nil || len(positions) == 0 {
			// If we can't find a match, fall back to the line number from the hunk
			logger.Info("could not find context match for hunk, using line number from diff",
				"error", err, "lineNumber", hunk.StartLine)
			position := hunk.StartLine - 1
			if position < 0 {
				position = 0
			}
			if position > len(result) {
				position = len(result)
			}
			result = applyHunkAtPosition(result, hunk, position)
			continue
		}

		// If we found multiple matches, log a warning but use the best one
		if len(positions) > 1 {
			logger.Info("multiple matches found for context lines, using best match",
				"matchCount", len(positions), "usingPosition", positions[0])
		}

		// Create a hunk with just the added lines to insert at the matched position
		addedHunk := &UDiffHunk{
			StartLine: 0, // Not used for insertion
			Lines:     addedLines,
		}

		// Find the insertion point based on the context match
		// We need to find where within the context match we should insert the new lines
		insertPosition := findInsertPosition(result, positions[0], contextLines, addedLines)

		// Apply just the added lines at the calculated position
		result = applyHunkAtPosition(result, addedHunk, insertPosition)
	}

	return result, nil
}

// extractContextAndAddedLines separates the hunk into context lines (unmodified) and added lines
func extractContextAndAddedLines(lines []string) ([]string, []string) {
	var contextLines []string
	var addedLines []string

	// When parsing the hunk, we kept only context lines (unchanged) and added lines
	// So, any lines already in the array are either context or added lines
	// However, we need to differentiate them for smart matching

	// For now, we'll assume all lines are context lines
	// In a real diff, context lines would be marked with a space prefix and added with a + prefix
	// But these are already parsed out in our case

	// This is a naive approach, but can be improved with additional metadata during parsing
	for _, line := range lines {
		// For example, we might look for certain patterns to identify added lines
		if strings.TrimSpace(line) == "" ||
			strings.HasPrefix(line, "//") ||
			strings.HasPrefix(line, "/*") ||
			strings.HasPrefix(line, " *") ||
			strings.HasPrefix(line, "# ") {
			// Common comment prefixes and whitespace are likely context lines
			contextLines = append(contextLines, line)
		} else {
			// Assume non-comment, non-whitespace lines are the added content
			addedLines = append(addedLines, line)
		}
	}

	// If we couldn't identify any added lines, treat them all as added
	// This will fall back to simpler behavior
	if len(addedLines) == 0 {
		return nil, lines
	}

	return contextLines, addedLines
}

// findContextMatches finds all positions in the file where the context lines match
func findContextMatches(fileLines []string, contextLines []string) ([]int, error) {
	if len(contextLines) == 0 {
		return nil, fmt.Errorf("no context lines provided")
	}

	matches := []int{}
	// We require at least 2 context lines for a good match
	minMatchRequired := 2
	if len(contextLines) < minMatchRequired {
		minMatchRequired = len(contextLines)
	}

	// Look for sequences of context lines in the file
	for i := 0; i <= len(fileLines)-minMatchRequired; i++ {
		matchCount := 0
		// Count how many consecutive lines match
		for j := 0; j < len(contextLines) && i+j < len(fileLines); j++ {
			if fileLines[i+j] == contextLines[j] {
				matchCount++
			} else {
				// Break on first non-match
				break
			}
		}

		// Consider it a match if we meet the minimum requirement
		if matchCount >= minMatchRequired {
			matches = append(matches, i)
		}
	}

	// If we found matches, sort by quality (number of matching lines)
	if len(matches) > 0 {
		// Calculate match quality for each position
		type matchQuality struct {
			position int
			quality  int
		}

		qualities := make([]matchQuality, len(matches))
		for i, pos := range matches {
			matchCount := 0
			for j := 0; j < len(contextLines) && pos+j < len(fileLines); j++ {
				if fileLines[pos+j] == contextLines[j] {
					matchCount++
				} else {
					break
				}
			}
			qualities[i] = matchQuality{pos, matchCount}
		}

		// Sort by quality (descending)
		sort.Slice(qualities, func(i, j int) bool {
			return qualities[i].quality > qualities[j].quality
		})

		// Convert back to positions
		sortedMatches := make([]int, len(matches))
		for i, q := range qualities {
			sortedMatches[i] = q.position
		}

		return sortedMatches, nil
	}

	return nil, fmt.Errorf("no context match found")
}

// findInsertPosition determines where within the context match to insert the added lines
func findInsertPosition(fileLines []string, matchPos int, contextLines []string, addedLines []string) int {
	// For now, we'll insert after the matched context
	// This is a simple heuristic but can be improved based on the specific diff format
	return matchPos + len(contextLines)
}

// applyHunkAtPosition applies a hunk at the specified position in the file
func applyHunkAtPosition(fileLines []string, hunk *UDiffHunk, position int) []string {
	// Handle out-of-bounds positions
	if position < 0 {
		position = 0
	}
	if position > len(fileLines) {
		position = len(fileLines)
	}

	// Create the result by splicing in the hunk's lines
	result := make([]string, 0, len(fileLines)+len(hunk.Lines))

	// Add lines before the hunk
	result = append(result, fileLines[:position]...)

	// Add the hunk's lines
	result = append(result, hunk.Lines...)

	// Add lines after the hunk
	if position < len(fileLines) {
		result = append(result, fileLines[position:]...)
	}

	return result
}
