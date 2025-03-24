package rag

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/golang"
	"github.com/smacker/go-tree-sitter/html"
	"github.com/smacker/go-tree-sitter/javascript"
)

// sourceContent holds the current file's content for tree-sitter operations
var sourceContent []byte

// ParseFile parses a file using tree-sitter based on its extension
func ParseFile(path string) (*sitter.Tree, error) {
	// Read the file
	var err error
	sourceContent, err = os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("error reading file: %w", err)
	}

	// Create a new parser
	parser := sitter.NewParser()

	// Get the appropriate language based on file extension
	var language *sitter.Language
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".go":
		language = golang.GetLanguage()
	case ".html", ".htm":
		language = html.GetLanguage()
	case ".js", ".jsx", ".mjs":
		language = javascript.GetLanguage()
	default:
		return nil, fmt.Errorf("unsupported file type: %s", ext)
	}

	// Set the language for the parser
	parser.SetLanguage(language)

	// Parse the content
	tree, err := parser.ParseCtx(context.Background(), nil, sourceContent)
	if err != nil {
		return nil, fmt.Errorf("failed to parse file: %w", err)
	}
	if tree == nil {
		return nil, fmt.Errorf("failed to parse file")
	}

	return tree, nil
}

// GetCodeStructure returns a slice of strings representing the code structure
func GetCodeStructure(tree *sitter.Tree) []string {
	var structure []string
	rootNode := tree.RootNode()
	walkTree(rootNode, "", &structure)
	return structure
}

// walkTree recursively walks the AST and builds the structure representation
func walkTree(node *sitter.Node, indent string, structure *[]string) {
	// Add node type and content
	*structure = append(*structure, fmt.Sprintf("%s%s", indent, node.Type()))

	// Process child nodes
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		walkTree(child, indent+"  ", structure)
	}
}

// GetFunctions returns a slice of strings representing function declarations
func GetFunctions(tree *sitter.Tree) []string {
	var functions []string
	rootNode := tree.RootNode()

	// Find all function declarations
	queryString := `
		(function_declaration
			name: (identifier) @name
			parameters: (parameter_list) @params
			result: (_)? @result
		) @function
	`

	language := golang.GetLanguage()
	q, err := sitter.NewQuery([]byte(queryString), language)
	if err != nil {
		return functions
	}

	qc := sitter.NewQueryCursor()
	qc.Exec(q, rootNode)

	for {
		m, ok := qc.NextMatch()
		if !ok {
			break
		}

		var name, params, result string
		for _, c := range m.Captures {
			switch c.Node.Type() {
			case "identifier":
				name = string(sourceContent[c.Node.StartByte():c.Node.EndByte()])
			case "parameter_list":
				params = string(sourceContent[c.Node.StartByte():c.Node.EndByte()])
			default:
				if c.Node.Parent().Type() == "function_declaration" {
					result = string(sourceContent[c.Node.StartByte():c.Node.EndByte()])
				}
			}
		}

		functions = append(functions, fmt.Sprintf("func %s%s%s", name, params, result))
	}

	return functions
}

// GetTypes returns a slice of strings representing type declarations
func GetTypes(tree *sitter.Tree) []string {
	var types []string
	rootNode := tree.RootNode()

	// Find all type declarations
	queryString := `
		(type_declaration
			(type_spec
				name: (type_identifier) @name
				type: (_) @type
			) @type_decl
		) @type_group
	`

	language := golang.GetLanguage()
	q, err := sitter.NewQuery([]byte(queryString), language)
	if err != nil {
		return types
	}

	qc := sitter.NewQueryCursor()
	qc.Exec(q, rootNode)

	for {
		m, ok := qc.NextMatch()
		if !ok {
			break
		}

		var name, typeDef string
		for _, c := range m.Captures {
			switch c.Node.Type() {
			case "type_identifier":
				name = string(sourceContent[c.Node.StartByte():c.Node.EndByte()])
			default:
				if c.Node.Parent().Type() == "type_spec" {
					typeDef = string(sourceContent[c.Node.StartByte():c.Node.EndByte()])
				}
			}
		}

		types = append(types, fmt.Sprintf("type %s %s", name, typeDef))
	}

	return types
}

// GetImports returns a slice of strings representing import declarations
func GetImports(tree *sitter.Tree) []string {
	var imports []string
	rootNode := tree.RootNode()

	// Find all import declarations
	queryString := `
		(import_declaration
			(import_spec
				name: (identifier)? @alias
				path: (interpreted_string_literal) @path
			) @import
		) @import_group
	`

	language := golang.GetLanguage()
	q, err := sitter.NewQuery([]byte(queryString), language)
	if err != nil {
		return imports
	}

	qc := sitter.NewQueryCursor()
	qc.Exec(q, rootNode)

	for {
		m, ok := qc.NextMatch()
		if !ok {
			break
		}

		var alias, path string
		for _, c := range m.Captures {
			switch c.Node.Type() {
			case "identifier":
				alias = string(sourceContent[c.Node.StartByte():c.Node.EndByte()])
			case "interpreted_string_literal":
				path = string(sourceContent[c.Node.StartByte():c.Node.EndByte()])
			}
		}

		if alias != "" {
			imports = append(imports, fmt.Sprintf("%s %s", alias, path))
		} else {
			imports = append(imports, path)
		}
	}

	return imports
}

// GetPackage returns the package name
func GetPackage(tree *sitter.Tree) string {
	rootNode := tree.RootNode()

	// Find package declaration
	queryString := `
		(package_clause
			name: (package_identifier) @name
		) @package
	`

	language := golang.GetLanguage()
	q, err := sitter.NewQuery([]byte(queryString), language)
	if err != nil {
		return ""
	}

	qc := sitter.NewQueryCursor()
	qc.Exec(q, rootNode)

	m, ok := qc.NextMatch()
	if !ok {
		return ""
	}

	for _, c := range m.Captures {
		if c.Node.Type() == "package_identifier" {
			return string(sourceContent[c.Node.StartByte():c.Node.EndByte()])
		}
	}

	return ""
}

// GetStructs returns a slice of strings representing struct declarations
func GetStructs(tree *sitter.Tree) []string {
	var structs []string
	rootNode := tree.RootNode()

	// Find all struct declarations
	queryString := `
		(type_declaration
			(type_spec
				name: (type_identifier) @name
				type: (struct_type
					field_declaration_list: (field_declaration_list
						(field_declaration
							name: (field_identifier) @field_name
							type: (_) @field_type
						) @field
					) @fields
				) @struct
			) @struct_decl
		) @struct_group
	`

	language := golang.GetLanguage()
	q, err := sitter.NewQuery([]byte(queryString), language)
	if err != nil {
		return structs
	}

	qc := sitter.NewQueryCursor()
	qc.Exec(q, rootNode)

	for {
		m, ok := qc.NextMatch()
		if !ok {
			break
		}

		var name string
		var fields []string
		for _, c := range m.Captures {
			switch c.Node.Type() {
			case "type_identifier":
				name = string(sourceContent[c.Node.StartByte():c.Node.EndByte()])
			case "field_declaration":
				fieldNode := c.Node
				fieldName := string(sourceContent[fieldNode.ChildByFieldName("name").StartByte():fieldNode.ChildByFieldName("name").EndByte()])
				fieldType := string(sourceContent[fieldNode.ChildByFieldName("type").StartByte():fieldNode.ChildByFieldName("type").EndByte()])
				fields = append(fields, fmt.Sprintf("%s %s", fieldName, fieldType))
			}
		}

		structs = append(structs, fmt.Sprintf("type %s struct {\n  %s\n}", name, strings.Join(fields, "\n  ")))
	}

	return structs
}

// GetInterfaces returns a slice of strings representing interface declarations
func GetInterfaces(tree *sitter.Tree) []string {
	var interfaces []string
	rootNode := tree.RootNode()

	// Find all interface declarations
	queryString := `
		(type_declaration
			(type_spec
				name: (type_identifier) @name
				type: (interface_type
					method_spec_list: (method_spec_list
						(method_spec
							name: (field_identifier) @method_name
							parameters: (parameter_list) @params
							result: (_)? @result
						) @method
					) @methods
				) @interface
			) @interface_decl
		) @interface_group
	`

	language := golang.GetLanguage()
	q, err := sitter.NewQuery([]byte(queryString), language)
	if err != nil {
		return interfaces
	}

	qc := sitter.NewQueryCursor()
	qc.Exec(q, rootNode)

	for {
		m, ok := qc.NextMatch()
		if !ok {
			break
		}

		var name string
		var methods []string
		for _, c := range m.Captures {
			switch c.Node.Type() {
			case "type_identifier":
				name = string(sourceContent[c.Node.StartByte():c.Node.EndByte()])
			case "method_spec":
				methodNode := c.Node
				methodName := string(sourceContent[methodNode.ChildByFieldName("name").StartByte():methodNode.ChildByFieldName("name").EndByte()])
				params := string(sourceContent[methodNode.ChildByFieldName("parameters").StartByte():methodNode.ChildByFieldName("parameters").EndByte()])
				result := ""
				if resultNode := methodNode.ChildByFieldName("result"); resultNode != nil {
					result = " " + string(sourceContent[resultNode.StartByte():resultNode.EndByte()])
				}
				methods = append(methods, fmt.Sprintf("%s%s%s", methodName, params, result))
			}
		}

		interfaces = append(interfaces, fmt.Sprintf("type %s interface {\n  %s\n}", name, strings.Join(methods, "\n  ")))
	}

	return interfaces
}
