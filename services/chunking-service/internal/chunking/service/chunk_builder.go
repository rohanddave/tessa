package service

import (
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/rohandave/tessa-rag/services/chunking-service/internal/chunking/ports"
	shareddomain "github.com/rohandave/tessa-rag/services/shared/domain"
	sharedutil "github.com/rohandave/tessa-rag/services/shared/util"
)

type chunkUnit struct {
	Name      string
	Kind      string
	StartByte uint32
	EndByte   uint32
}

type ChunkBuilder struct {
}

func NewChunkBuilder() *ChunkBuilder {
	return &ChunkBuilder{}
}

func (b *ChunkBuilder) Build(snapshot *shareddomain.Snapshot, fileJob *shareddomain.FileJob, extractedData *ports.CodeParseResult) ([]*shareddomain.Chunk, error) {
	if snapshot == nil || fileJob == nil || extractedData == nil {
		return nil, fmt.Errorf("One of the dependencies is nil. Cannot build chunk")
	}

	chunkUnits := b.collectChunkUnits(extractedData)
	if len(chunkUnits) == 0 {
		return []*shareddomain.Chunk{}, nil
	}

	chunks := make([]*shareddomain.Chunk, 0, len(chunkUnits))
	for _, unit := range chunkUnits {
		chunkID := sharedutil.GenerateUUID()
		chunks = append(chunks, b.buildChunk(snapshot, fileJob, chunkID, unit))
	}

	for i := range chunks {
		if i > 0 {
			chunks[i].PrevChunkID = chunks[i-1].ChunkID
		}
		if i < len(chunks)-1 {
			chunks[i].NextChunkID = chunks[i+1].ChunkID
		}
	}

	return chunks, nil
}

func (b *ChunkBuilder) buildChunk(snapshot *shareddomain.Snapshot, fileJob *shareddomain.FileJob, chunkID string, unit chunkUnit) *shareddomain.Chunk {
	filePath := fileJob.Path
	fileName := path.Base(fileJob.Path)
	language := b.normalizeLanguage(fileJob.Extension)
	startByte := unit.StartByte
	endByte := unit.EndByte
	content := b.extractChunkText(fileJob.Content, startByte, endByte)
	startLine, endLine := b.byteRangeToLineRange(fileJob.Content, startByte, endByte)

	chunk := shareddomain.Chunk{
		ChunkID:                chunkID,
		RepoURL:                snapshot.RepoURL,
		FileName:               fileName,
		Branch:                 snapshot.Branch,
		CommitSHA:              snapshot.CommitSHA,
		SnapshotID:             snapshot.Id,
		FilePath:               filePath,
		DocumentType:           "code",
		Language:               language,
		Content:                content,
		ContentHash:            sharedutil.HashString(content),
		SymbolName:             unit.Name,
		SymbolType:             unit.Kind,
		StartLine:              startLine,
		EndLine:                endLine,
		StartChar:              int(startByte),
		EndChar:                int(endByte),
		Status:                 "pending_index",
		TextSearchEngineStatus: "pending",
		VectorDbStatus:         "pending",
		PrevChunkID:            "pending",
		NextChunkID:            "pending",
		CreatedAt:              time.Now().UTC().Unix(),
	}

	return &chunk
}

func (b *ChunkBuilder) extractChunkText(content []byte, startByte uint32, endByte uint32) string {
	if len(content) == 0 {
		return ""
	}

	if startByte >= uint32(len(content)) || endByte > uint32(len(content)) || startByte >= endByte {
		return string(content)
	}

	return string(content[startByte:endByte])
}

func (b *ChunkBuilder) byteRangeToLineRange(content []byte, startByte uint32, endByte uint32) (int, int) {
	if len(content) == 0 {
		return 0, 0
	}

	if startByte >= uint32(len(content)) || endByte > uint32(len(content)) || startByte >= endByte {
		return 1, strings.Count(string(content), "\n") + 1
	}

	startLine := 1
	for _, b := range content[:startByte] {
		if b == '\n' {
			startLine++
		}
	}

	endLine := startLine
	for _, b := range content[startByte:endByte] {
		if b == '\n' {
			endLine++
		}
	}

	return startLine, endLine
}

func (b *ChunkBuilder) normalizeLanguage(extension string) string {
	switch strings.ToLower(strings.TrimPrefix(extension, ".")) {
	case "go":
		return "go"
	case "js":
		return "javascript"
	case "jsx":
		return "javascript"
	case "ts":
		return "typescript"
	case "tsx":
		return "typescript"
	case "py":
		return "python"
	case "java":
		return "java"
	default:
		return "unknown"
	}
}

func (b *ChunkBuilder) collectChunkUnits(extractedData *ports.CodeParseResult) []chunkUnit {
	if extractedData == nil {
		return nil
	}

	units := make([]chunkUnit, 0, len(extractedData.Classes)+len(extractedData.Functions)+len(extractedData.Methods)+len(extractedData.ModuleLevelDeclarations)+len(extractedData.Symbols)+len(extractedData.Containers)+len(extractedData.Imports)+len(extractedData.DocComments))
	seen := make(map[string]struct{})

	appendUnit := func(unit chunkUnit) {
		if unit.StartByte >= unit.EndByte {
			return
		}

		key := fmt.Sprintf("%s:%s:%d:%d", unit.Kind, unit.Name, unit.StartByte, unit.EndByte)
		if _, exists := seen[key]; exists {
			return
		}
		seen[key] = struct{}{}
		units = append(units, unit)
	}

	appendDeclarations := func(items []ports.CodeDeclaration) {
		for _, declaration := range items {
			appendUnit(chunkUnit{
				Name:      declaration.Name,
				Kind:      declaration.Kind,
				StartByte: declaration.StartByte,
				EndByte:   declaration.EndByte,
			})
		}
	}

	appendDeclarations(extractedData.Classes)
	appendDeclarations(extractedData.Functions)
	appendDeclarations(extractedData.Methods)
	appendDeclarations(extractedData.ModuleLevelDeclarations)

	for _, symbol := range extractedData.Symbols {
		appendUnit(chunkUnit{
			Name:      symbol.Name,
			Kind:      "symbol:" + symbol.Kind,
			StartByte: symbol.StartByte,
			EndByte:   symbol.EndByte,
		})
	}

	for _, container := range extractedData.Containers {
		appendUnit(chunkUnit{
			Name:      container.Name,
			Kind:      "container:" + container.Kind,
			StartByte: container.StartByte,
			EndByte:   container.EndByte,
		})
	}

	for _, item := range extractedData.Imports {
		appendUnit(chunkUnit{
			Name:      item.Path,
			Kind:      "import",
			StartByte: item.StartByte,
			EndByte:   item.EndByte,
		})
	}

	for _, comment := range extractedData.DocComments {
		name := comment.Target
		if name == "" {
			name = "doc_comment"
		}

		appendUnit(chunkUnit{
			Name:      name,
			Kind:      "doc_comment",
			StartByte: comment.StartByte,
			EndByte:   comment.EndByte,
		})
	}

	return units
}
