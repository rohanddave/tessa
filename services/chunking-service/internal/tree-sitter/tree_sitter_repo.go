package treesitter

import "github.com/rohandave/tessa-rag/services/chunking-service/internal/chunking/ports"

type TreeSitterRepo struct{}

func NewTreeSitterRepo() ports.CodeParser {
	return &TreeSitterRepo{}
}

func (r *TreeSitterRepo) DescribeParser() string {
	return "This is the TreeSitterRepo, responsible for managing tree-sitter parsers and related resources."
}
