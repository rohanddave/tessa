package treesitter

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/rohandave/tessa-rag/services/chunking-service/internal/chunking/ports"
	sitter "github.com/smacker/go-tree-sitter"
	tsgo "github.com/smacker/go-tree-sitter/golang"
	tsjava "github.com/smacker/go-tree-sitter/java"
	tsjavascript "github.com/smacker/go-tree-sitter/javascript"
	tspython "github.com/smacker/go-tree-sitter/python"
	tstypescript "github.com/smacker/go-tree-sitter/typescript/typescript"
)

type TreeSitterRepo struct{}

func NewTreeSitterRepo() ports.CodeParser {
	return &TreeSitterRepo{}
}

func (r *TreeSitterRepo) DescribeParser() string {
	return "This is the TreeSitterRepo, responsible for managing tree-sitter parsers and related resources."
}

func (r *TreeSitterRepo) ExtractFileMetadata(fileContent string, language string) (*ports.CodeParseResult, error) {
	trimmedLanguage := strings.ToLower(strings.TrimSpace(language))
	if trimmedLanguage == "" {
		err := fmt.Errorf("language cannot be empty")
		log.Printf("tree-sitter extraction failed: %v", err)
		return nil, err
	}

	if strings.TrimSpace(fileContent) == "" {
		err := fmt.Errorf("file content cannot be empty")
		log.Printf("tree-sitter extraction failed language=%s: %v", trimmedLanguage, err)
		return nil, err
	}

	log.Printf("tree-sitter extraction started language=%s content_bytes=%d", trimmedLanguage, len(fileContent))

	lang, err := r.resolveLanguage(language)
	if err != nil {
		log.Printf("tree-sitter extraction failed language=%s: %v", trimmedLanguage, err)
		return nil, err
	}

	source := []byte(fileContent)
	parser := sitter.NewParser()
	defer parser.Close()

	if lang == nil {
		err := fmt.Errorf("tree-sitter language is nil for %q", trimmedLanguage)
		log.Printf("tree-sitter extraction failed language=%s: %v", trimmedLanguage, err)
		return nil, err
	}

	log.Printf("tree-sitter setting language=%s", trimmedLanguage)
	parser.SetLanguage(lang)
	tree, err := parser.ParseCtx(context.Background(), nil, source)
	if err != nil {
		log.Printf("tree-sitter parse failed language=%s: %v", trimmedLanguage, err)
		return nil, fmt.Errorf("parse file content: %w", err)
	}
	if tree == nil {
		err := fmt.Errorf("tree-sitter returned nil parse tree")
		log.Printf("tree-sitter extraction failed language=%s: %v", trimmedLanguage, err)
		return nil, err
	}
	defer tree.Close()

	root := tree.RootNode()
	if root == nil {
		err := fmt.Errorf("tree-sitter returned nil root node")
		log.Printf("tree-sitter extraction failed language=%s: %v", trimmedLanguage, err)
		return nil, err
	}

	result := &ports.CodeParseResult{}
	result.Classes = r.extractClasses(root, source)
	result.Functions = r.extractFunctions(root, source)
	result.Methods = r.extractMethods(root, source)
	result.ModuleLevelDeclarations = r.extractModuleLevelDeclarations(root, source)
	result.Symbols = r.extractSymbols(result)
	result.Containers = r.extractContainers(root, source)
	result.Imports = r.extractImports(root, source)
	result.DocComments = r.extractDocComments(root, source)

	log.Printf(
		"tree-sitter extraction completed language=%s symbols=%d containers=%d imports=%d doc_comments=%d classes=%d functions=%d methods=%d module_level_declarations=%d",
		trimmedLanguage,
		len(result.Symbols),
		len(result.Containers),
		len(result.Imports),
		len(result.DocComments),
		len(result.Classes),
		len(result.Functions),
		len(result.Methods),
		len(result.ModuleLevelDeclarations),
	)

	return result, nil
}

func (r *TreeSitterRepo) extractSymbols(result *ports.CodeParseResult) []ports.CodeSymbol {
	symbols := make([]ports.CodeSymbol, 0, len(result.Classes)+len(result.Functions)+len(result.Methods)+len(result.ModuleLevelDeclarations))
	seen := make(map[string]struct{})

	appendDeclaration := func(declarations []ports.CodeDeclaration) {
		for _, declaration := range declarations {
			key := fmt.Sprintf("%s:%s:%d:%d", declaration.Kind, declaration.Name, declaration.StartByte, declaration.EndByte)
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			symbols = append(symbols, ports.CodeSymbol{
				Name:      declaration.Name,
				Kind:      declaration.Kind,
				StartByte: declaration.StartByte,
				EndByte:   declaration.EndByte,
			})
		}
	}

	appendDeclaration(result.Classes)
	appendDeclaration(result.Functions)
	appendDeclaration(result.Methods)
	appendDeclaration(result.ModuleLevelDeclarations)

	return symbols
}

func (r *TreeSitterRepo) extractContainers(root *sitter.Node, source []byte) []ports.CodeContainer {
	containers := make([]ports.CodeContainer, 0)

	r.walk(root, func(node *sitter.Node) {
		if !r.isContainerNode(node) {
			return
		}

		name := r.extractNodeName(node, source)
		if name == "" {
			name = node.Type()
		}

		containers = append(containers, ports.CodeContainer{
			Name:       name,
			Kind:       node.Type(),
			ParentName: r.findParentContainerName(node, source),
			StartByte:  node.StartByte(),
			EndByte:    node.EndByte(),
		})
	})

	return containers
}

func (r *TreeSitterRepo) extractImports(root *sitter.Node, source []byte) []ports.CodeImport {
	imports := make([]ports.CodeImport, 0)

	r.walk(root, func(node *sitter.Node) {
		if !r.isImportNode(node) {
			return
		}

		importPath := r.extractImportPath(node, source)
		if importPath == "" {
			importPath = r.nodeText(node, source)
		}

		imports = append(imports, ports.CodeImport{
			Path:      importPath,
			Kind:      node.Type(),
			StartByte: node.StartByte(),
			EndByte:   node.EndByte(),
		})
	})

	return imports
}

func (r *TreeSitterRepo) extractDocComments(root *sitter.Node, source []byte) []ports.DocComment {
	comments := make([]ports.DocComment, 0)

	r.walk(root, func(node *sitter.Node) {
		if !r.isDocCommentNode(node, source) {
			return
		}

		comments = append(comments, ports.DocComment{
			Text:      strings.TrimSpace(r.nodeText(node, source)),
			Target:    r.findNextDeclarationName(node, source),
			StartByte: node.StartByte(),
			EndByte:   node.EndByte(),
		})
	})

	return comments
}

func (r *TreeSitterRepo) extractClasses(root *sitter.Node, source []byte) []ports.CodeDeclaration {
	return r.extractDeclarationsByKind(root, source, r.isClassNode, "class")
}

func (r *TreeSitterRepo) extractFunctions(root *sitter.Node, source []byte) []ports.CodeDeclaration {
	return r.extractDeclarationsByKind(root, source, r.isFunctionNode, "function")
}

func (r *TreeSitterRepo) extractMethods(root *sitter.Node, source []byte) []ports.CodeDeclaration {
	return r.extractDeclarationsByKind(root, source, r.isMethodNode, "method")
}

func (r *TreeSitterRepo) extractModuleLevelDeclarations(root *sitter.Node, source []byte) []ports.CodeDeclaration {
	declarations := make([]ports.CodeDeclaration, 0)

	r.walk(root, func(node *sitter.Node) {
		if !r.isModuleLevelDeclaration(node) {
			return
		}

		name := r.extractNodeName(node, source)
		if name == "" {
			return
		}

		declarations = append(declarations, ports.CodeDeclaration{
			Name:      name,
			Kind:      node.Type(),
			Text:      strings.TrimSpace(r.nodeText(node, source)),
			StartByte: node.StartByte(),
			EndByte:   node.EndByte(),
		})
	})

	return declarations
}

func (r *TreeSitterRepo) extractDeclarationsByKind(root *sitter.Node, source []byte, match func(*sitter.Node) bool, kind string) []ports.CodeDeclaration {
	declarations := make([]ports.CodeDeclaration, 0)

	r.walk(root, func(node *sitter.Node) {
		if !match(node) {
			return
		}

		name := r.extractNodeName(node, source)
		if name == "" {
			return
		}

		declarations = append(declarations, ports.CodeDeclaration{
			Name:      name,
			Kind:      kind,
			Text:      strings.TrimSpace(r.nodeText(node, source)),
			StartByte: node.StartByte(),
			EndByte:   node.EndByte(),
		})
	})

	return declarations
}

func (r *TreeSitterRepo) resolveLanguage(language string) (*sitter.Language, error) {
	switch strings.ToLower(strings.TrimSpace(language)) {
	case "go", "golang":
		return tsgo.GetLanguage(), nil
	case "javascript", "js":
		return tsjavascript.GetLanguage(), nil
	case "typescript", "ts":
		return tstypescript.GetLanguage(), nil
	case "python", "py":
		return tspython.GetLanguage(), nil
	case "java":
		return tsjava.GetLanguage(), nil
	default:
		return nil, fmt.Errorf("unsupported language %q", language)
	}
}

func (r *TreeSitterRepo) walk(node *sitter.Node, visit func(*sitter.Node)) {
	if node == nil {
		return
	}

	visit(node)
	for i := uint32(0); i < node.ChildCount(); i++ {
		r.walk(node.Child(int(i)), visit)
	}
}

func (r *TreeSitterRepo) nodeText(node *sitter.Node, source []byte) string {
	if node == nil {
		return ""
	}

	start := node.StartByte()
	end := node.EndByte()
	if start >= uint32(len(source)) || end > uint32(len(source)) || start >= end {
		return ""
	}

	return string(source[start:end])
}

func (r *TreeSitterRepo) extractNodeName(node *sitter.Node, source []byte) string {
	if node == nil {
		return ""
	}

	for _, fieldName := range []string{"name", "declarator", "field", "property", "alias"} {
		child := node.ChildByFieldName(fieldName)
		if child != nil {
			if name := r.extractIdentifier(child, source); name != "" {
				return name
			}
		}
	}

	return r.extractIdentifier(node, source)
}

func (r *TreeSitterRepo) extractIdentifier(node *sitter.Node, source []byte) string {
	if node == nil {
		return ""
	}

	nodeType := node.Type()
	if strings.Contains(nodeType, "identifier") || nodeType == "type_identifier" || nodeType == "property_identifier" {
		return strings.TrimSpace(r.nodeText(node, source))
	}

	for i := uint32(0); i < node.ChildCount(); i++ {
		if name := r.extractIdentifier(node.Child(int(i)), source); name != "" {
			return name
		}
	}

	return ""
}

func (r *TreeSitterRepo) findParentContainerName(node *sitter.Node, source []byte) string {
	for parent := node.Parent(); parent != nil; parent = parent.Parent() {
		if r.isContainerNode(parent) {
			name := r.extractNodeName(parent, source)
			if name != "" {
				return name
			}
		}
	}

	return ""
}

func (r *TreeSitterRepo) findNextDeclarationName(node *sitter.Node, source []byte) string {
	parent := node.Parent()
	if parent == nil {
		return ""
	}

	foundCurrent := false
	for i := uint32(0); i < parent.ChildCount(); i++ {
		child := parent.Child(int(i))
		if child == nil {
			continue
		}

		if child == node {
			foundCurrent = true
			continue
		}

		if !foundCurrent {
			continue
		}

		if r.isDeclarationNode(child) {
			return r.extractNodeName(child, source)
		}
	}

	return ""
}

func (r *TreeSitterRepo) extractImportPath(node *sitter.Node, source []byte) string {
	for i := uint32(0); i < node.ChildCount(); i++ {
		child := node.Child(int(i))
		if child == nil {
			continue
		}

		childType := child.Type()
		if childType == "interpreted_string_literal" || childType == "raw_string_literal" || childType == "string" || childType == "string_literal" {
			return strings.Trim(r.nodeText(child, source), "\"`'")
		}
	}

	return ""
}

func (r *TreeSitterRepo) isContainerNode(node *sitter.Node) bool {
	return r.isClassNode(node) || r.isFunctionNode(node) || r.isMethodNode(node)
}

func (r *TreeSitterRepo) isDeclarationNode(node *sitter.Node) bool {
	return r.isClassNode(node) || r.isFunctionNode(node) || r.isMethodNode(node) || r.isModuleLevelDeclaration(node)
}

func (r *TreeSitterRepo) isClassNode(node *sitter.Node) bool {
	if node == nil {
		return false
	}

	switch node.Type() {
	case "class_declaration", "class_definition", "interface_declaration", "struct_type", "type_spec":
		return true
	default:
		return false
	}
}

func (r *TreeSitterRepo) isFunctionNode(node *sitter.Node) bool {
	if node == nil {
		return false
	}

	switch node.Type() {
	case "function_declaration", "function_definition", "function_item", "func_literal":
		return true
	default:
		return false
	}
}

func (r *TreeSitterRepo) isMethodNode(node *sitter.Node) bool {
	if node == nil {
		return false
	}

	switch node.Type() {
	case "method_declaration", "method_definition":
		return true
	default:
		return false
	}
}

func (r *TreeSitterRepo) isImportNode(node *sitter.Node) bool {
	if node == nil {
		return false
	}

	switch node.Type() {
	case "import_declaration", "import_spec", "import_statement", "include", "include_clause":
		return true
	default:
		return false
	}
}

func (r *TreeSitterRepo) isDocCommentNode(node *sitter.Node, source []byte) bool {
	if node == nil || node.Type() != "comment" {
		return false
	}

	text := strings.TrimSpace(r.nodeText(node, source))
	return strings.HasPrefix(text, "//") || strings.HasPrefix(text, "/*") || strings.HasPrefix(text, "#")
}

func (r *TreeSitterRepo) isModuleLevelDeclaration(node *sitter.Node) bool {
	if node == nil {
		return false
	}

	parent := node.Parent()
	if parent == nil {
		return false
	}

	parentType := parent.Type()
	if parentType != "source_file" && parentType != "program" && parentType != "module" {
		return false
	}

	switch node.Type() {
	case "const_declaration", "var_declaration", "lexical_declaration", "type_declaration", "type_alias_declaration", "class_declaration", "function_declaration", "function_definition", "method_declaration":
		return true
	default:
		return false
	}
}
