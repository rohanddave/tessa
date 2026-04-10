package treesitter

import (
	"context"
	"embed"
	"fmt"
	"log"
	"path"
	"strings"

	"github.com/rohandave/tessa-rag/services/chunking-service/internal/chunking/ports"
	sitter "github.com/smacker/go-tree-sitter"
	tsgo "github.com/smacker/go-tree-sitter/golang"
	tsjava "github.com/smacker/go-tree-sitter/java"
	tsjavascript "github.com/smacker/go-tree-sitter/javascript"
	tspython "github.com/smacker/go-tree-sitter/python"
	tstypescript "github.com/smacker/go-tree-sitter/typescript/typescript"
)

//go:embed queries/*.scm
var queryFiles embed.FS

type TreeSitterRepo struct{}

type resolvedLanguage struct {
	name     string
	language *sitter.Language
}

type queryMatch struct {
	patternIndex uint16
	captures     map[string][]*sitter.Node
}

func NewTreeSitterRepo() ports.CodeParser {
	return &TreeSitterRepo{}
}

func (r *TreeSitterRepo) DescribeParser() string {
	return "This is the TreeSitterRepo, responsible for managing tree-sitter parsers and related resources."
}

func (r *TreeSitterRepo) ExtractFileMetadata(fileContent string, language string) (*ports.CodeParseResult, error) {
	resolved, err := r.resolveLanguage(language)
	if err != nil {
		log.Printf("tree-sitter extraction failed language=%s: %v", strings.TrimSpace(language), err)
		return nil, err
	}

	if strings.TrimSpace(fileContent) == "" {
		err := fmt.Errorf("file content cannot be empty")
		log.Printf("tree-sitter extraction failed language=%s: %v", resolved.name, err)
		return nil, err
	}

	log.Printf("tree-sitter extraction started language=%s content_bytes=%d", resolved.name, len(fileContent))

	source := []byte(fileContent)
	parser := sitter.NewParser()
	defer parser.Close()

	if resolved.language == nil {
		err := fmt.Errorf("tree-sitter language is nil for %q", resolved.name)
		log.Printf("tree-sitter extraction failed language=%s: %v", resolved.name, err)
		return nil, err
	}

	parser.SetLanguage(resolved.language)
	tree, err := parser.ParseCtx(context.Background(), nil, source)
	if err != nil {
		log.Printf("tree-sitter parse failed language=%s: %v", resolved.name, err)
		return nil, fmt.Errorf("parse file content: %w", err)
	}
	if tree == nil {
		err := fmt.Errorf("tree-sitter returned nil parse tree")
		log.Printf("tree-sitter extraction failed language=%s: %v", resolved.name, err)
		return nil, err
	}
	defer tree.Close()

	root := tree.RootNode()
	if root == nil {
		err := fmt.Errorf("tree-sitter returned nil root node")
		log.Printf("tree-sitter extraction failed language=%s: %v", resolved.name, err)
		return nil, err
	}

	querySource, err := r.loadQuerySource(resolved.name)
	if err != nil {
		log.Printf("tree-sitter query load failed language=%s: %v", resolved.name, err)
		return nil, err
	}

	matches, err := r.runQuery(resolved, root, querySource)
	if err != nil {
		log.Printf("tree-sitter query execution failed language=%s: %v", resolved.name, err)
		return nil, err
	}

	result := &ports.CodeParseResult{}
	result.Classes = r.extractClasses(matches, source)
	result.Functions = r.extractFunctions(matches, source)
	result.Methods = r.extractMethods(matches, source)
	result.ModuleLevelDeclarations = r.extractModuleLevelDeclarations(matches, source)
	result.Symbols = r.extractSymbols(result)
	result.Containers = r.extractContainers(result, source)
	result.Imports = r.extractImports(matches, source)
	result.DocComments = r.extractDocComments(matches, source)

	log.Printf(
		"tree-sitter extraction completed language=%s query_matches=%d symbols=%d containers=%d imports=%d doc_comments=%d classes=%d functions=%d methods=%d module_level_declarations=%d",
		resolved.name,
		len(matches),
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

func (r *TreeSitterRepo) runQuery(resolved resolvedLanguage, root *sitter.Node, querySource []byte) ([]queryMatch, error) {
	query, err := sitter.NewQuery(querySource, resolved.language)
	if err != nil {
		return nil, fmt.Errorf("compile %s query: %w", resolved.name, err)
	}
	defer query.Close()

	cursor := sitter.NewQueryCursor()
	defer cursor.Close()

	cursor.Exec(query, root)

	matches := make([]queryMatch, 0)
	for {
		match, ok := cursor.NextMatch()
		if !ok {
			break
		}

		filteredMatch := cursor.FilterPredicates(match, nil)
		if filteredMatch == nil {
			continue
		}

		captures := make(map[string][]*sitter.Node)
		for _, capture := range filteredMatch.Captures {
			name := query.CaptureNameForId(capture.Index)
			captures[name] = append(captures[name], capture.Node)
		}

		matches = append(matches, queryMatch{
			patternIndex: filteredMatch.PatternIndex,
			captures:     captures,
		})
	}

	return matches, nil
}

func (r *TreeSitterRepo) extractClasses(matches []queryMatch, source []byte) []ports.CodeDeclaration {
	return r.extractDeclarations(matches, source, "class.definition", "class.name", "class")
}

func (r *TreeSitterRepo) extractFunctions(matches []queryMatch, source []byte) []ports.CodeDeclaration {
	return r.extractDeclarations(matches, source, "function.definition", "function.name", "function")
}

func (r *TreeSitterRepo) extractMethods(matches []queryMatch, source []byte) []ports.CodeDeclaration {
	return r.extractDeclarations(matches, source, "method.definition", "method.name", "method")
}

func (r *TreeSitterRepo) extractModuleLevelDeclarations(matches []queryMatch, source []byte) []ports.CodeDeclaration {
	return r.extractDeclarations(matches, source, "module.declaration", "module.name", "module")
}

func (r *TreeSitterRepo) extractDeclarations(matches []queryMatch, source []byte, declarationCapture string, nameCapture string, kind string) []ports.CodeDeclaration {
	declarations := make([]ports.CodeDeclaration, 0)
	seen := make(map[string]struct{})

	for _, match := range matches {
		declarationNode := firstCapture(match, declarationCapture)
		nameNode := firstCapture(match, nameCapture)
		if declarationNode == nil {
			continue
		}

		name := strings.TrimSpace(r.nodeText(nameNode, source))
		if name == "" {
			name = r.extractIdentifier(declarationNode, source)
		}
		if name == "" {
			continue
		}

		key := fmt.Sprintf("%s:%s:%d:%d", kind, name, declarationNode.StartByte(), declarationNode.EndByte())
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}

		declarations = append(declarations, ports.CodeDeclaration{
			Name:      name,
			Kind:      kind,
			Text:      strings.TrimSpace(r.nodeText(declarationNode, source)),
			StartByte: declarationNode.StartByte(),
			EndByte:   declarationNode.EndByte(),
		})
	}

	return declarations
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

func (r *TreeSitterRepo) extractContainers(result *ports.CodeParseResult, source []byte) []ports.CodeContainer {
	containers := make([]ports.CodeContainer, 0, len(result.Classes)+len(result.Functions)+len(result.Methods))

	appendDeclaration := func(declarations []ports.CodeDeclaration) {
		for _, declaration := range declarations {
			containers = append(containers, ports.CodeContainer{
				Name:       declaration.Name,
				Kind:       declaration.Kind,
				ParentName: r.findParentContainerNameByRange(declaration.StartByte, declaration.EndByte, source),
				StartByte:  declaration.StartByte,
				EndByte:    declaration.EndByte,
			})
		}
	}

	appendDeclaration(result.Classes)
	appendDeclaration(result.Functions)
	appendDeclaration(result.Methods)

	return containers
}

func (r *TreeSitterRepo) extractImports(matches []queryMatch, source []byte) []ports.CodeImport {
	imports := make([]ports.CodeImport, 0)
	seen := make(map[string]struct{})

	for _, match := range matches {
		importNode := firstCapture(match, "import.definition")
		if importNode == nil {
			continue
		}

		importPath := strings.TrimSpace(r.nodeText(firstCapture(match, "import.path"), source))
		if importPath == "" {
			importPath = strings.TrimSpace(r.nodeText(importNode, source))
		}
		importPath = strings.Trim(importPath, "\"'`")

		key := fmt.Sprintf("%s:%d:%d", importPath, importNode.StartByte(), importNode.EndByte())
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}

		imports = append(imports, ports.CodeImport{
			Path:      importPath,
			Kind:      importNode.Type(),
			StartByte: importNode.StartByte(),
			EndByte:   importNode.EndByte(),
		})
	}

	return imports
}

func (r *TreeSitterRepo) extractDocComments(matches []queryMatch, source []byte) []ports.DocComment {
	comments := make([]ports.DocComment, 0)
	seen := make(map[string]struct{})

	for _, match := range matches {
		for _, commentNode := range match.captures["doc.comment"] {
			text := strings.TrimSpace(r.nodeText(commentNode, source))
			if text == "" {
				continue
			}

			key := fmt.Sprintf("%d:%d", commentNode.StartByte(), commentNode.EndByte())
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}

			comments = append(comments, ports.DocComment{
				Text:      text,
				Target:    "",
				StartByte: commentNode.StartByte(),
				EndByte:   commentNode.EndByte(),
			})
		}
	}

	return comments
}

func (r *TreeSitterRepo) resolveLanguage(language string) (resolvedLanguage, error) {
	switch strings.ToLower(strings.TrimSpace(language)) {
	case "go", "golang":
		return resolvedLanguage{name: "go", language: tsgo.GetLanguage()}, nil
	case "javascript", "js", "jsx":
		return resolvedLanguage{name: "javascript", language: tsjavascript.GetLanguage()}, nil
	case "typescript", "ts", "tsx":
		return resolvedLanguage{name: "typescript", language: tstypescript.GetLanguage()}, nil
	case "python", "py":
		return resolvedLanguage{name: "python", language: tspython.GetLanguage()}, nil
	case "java":
		return resolvedLanguage{name: "java", language: tsjava.GetLanguage()}, nil
	default:
		return resolvedLanguage{}, fmt.Errorf("unsupported language %q", language)
	}
}

func (r *TreeSitterRepo) loadQuerySource(language string) ([]byte, error) {
	queryPath := path.Join("queries", language+".scm")
	querySource, err := queryFiles.ReadFile(queryPath)
	if err != nil {
		return nil, fmt.Errorf("read tree-sitter query file %q: %w", queryPath, err)
	}

	return querySource, nil
}

func firstCapture(match queryMatch, captureName string) *sitter.Node {
	nodes := match.captures[captureName]
	if len(nodes) == 0 {
		return nil
	}

	return nodes[0]
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

func (r *TreeSitterRepo) findParentContainerNameByRange(startByte uint32, endByte uint32, source []byte) string {
	// Query captures return declaration ranges but not parent captures. Keep this hook
	// small for now; nested parent names can be added with language-specific captures.
	return ""
}
