package ports

type CodeParseResult struct {
	Symbols                 []CodeSymbol
	Containers              []CodeContainer
	Imports                 []CodeImport
	DocComments             []DocComment
	Classes                 []CodeDeclaration
	Functions               []CodeDeclaration
	Methods                 []CodeDeclaration
	ModuleLevelDeclarations []CodeDeclaration
}

type CodeSymbol struct {
	Name      string
	Kind      string
	StartByte uint32
	EndByte   uint32
}

type CodeContainer struct {
	Name       string
	Kind       string
	ParentName string
	StartByte  uint32
	EndByte    uint32
}

type CodeImport struct {
	Path      string
	Kind      string
	StartByte uint32
	EndByte   uint32
}

type DocComment struct {
	Text      string
	Target    string
	StartByte uint32
	EndByte   uint32
}

type CodeDeclaration struct {
	Name      string
	Kind      string
	Text      string
	StartByte uint32
	EndByte   uint32
}

type CodeParser interface {
	DescribeParser() string
	ExtractFileMetadata(fileContent string, language string) (*CodeParseResult, error)
}
