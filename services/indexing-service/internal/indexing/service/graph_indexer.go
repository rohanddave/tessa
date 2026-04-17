package service

import (
	"context"
	"fmt"
	"log"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/rohandave/tessa-rag/services/indexing-service/internal/config"
	"github.com/rohandave/tessa-rag/services/shared/domain"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

type Neo4jIndexer struct {
	logger        *log.Logger
	config        *config.Neo4jConfig
	driver        neo4j.DriverWithContext
	schemaMu      sync.Mutex
	schemaReady   bool
	driverInitErr error
}

func NewNeo4jIndexer(logger *log.Logger, cfg *config.Neo4jConfig) *Neo4jIndexer {
	indexer := &Neo4jIndexer{logger: logger, config: cfg}
	if cfg == nil {
		indexer.driverInitErr = fmt.Errorf("neo4j config is nil")
		return indexer
	}

	driver, err := neo4j.NewDriverWithContext(cfg.URI, neo4j.BasicAuth(cfg.User, cfg.Password, ""))
	if err != nil {
		indexer.driverInitErr = fmt.Errorf("create neo4j driver: %w", err)
		return indexer
	}
	indexer.driver = driver

	return indexer
}

func (i *Neo4jIndexer) IndexChunk(ctx context.Context, chunk *domain.Chunk) error {
	if chunk == nil {
		return fmt.Errorf("index neo4j chunk: chunk is nil")
	}
	if err := i.ensureReady(ctx); err != nil {
		return err
	}

	started := time.Now()
	i.logger.Printf(
		"neo4j index started chunk_id=%s repo=%s branch=%s snapshot=%s file=%s symbol=%s type=%s prev=%t next=%t",
		chunk.ChunkID,
		chunk.RepoURL,
		chunk.Branch,
		chunk.SnapshotID,
		chunk.FilePath,
		chunk.SymbolName,
		chunk.SymbolType,
		chunk.PrevChunkID != "",
		chunk.NextChunkID != "",
	)

	session := i.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer func() {
		if err := session.Close(ctx); err != nil {
			i.logger.Printf("failed to close neo4j session chunk_id=%s: %v", chunk.ChunkID, err)
		}
	}()

	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		stats := &graphWriteStats{}
		params := chunkParams(chunk)
		if err := runNeo4j(ctx, tx, upsertChunkQuery, params, stats); err != nil {
			return nil, err
		}
		if err := runNeo4j(ctx, tx, upsertFileContainmentQuery, params, stats); err != nil {
			return nil, err
		}
		if chunk.PrevChunkID != "" {
			if err := runNeo4j(ctx, tx, linkPrevChunkQuery, params, stats); err != nil {
				return nil, err
			}
		}
		if chunk.NextChunkID != "" {
			if err := runNeo4j(ctx, tx, linkNextChunkQuery, params, stats); err != nil {
				return nil, err
			}
		}
		if isImportChunk(chunk.SymbolType) {
			params["import_candidates"] = importResolutionCandidates(chunk.FilePath, chunk.SymbolName, chunk.Language)
			i.logger.Printf("neo4j import resolution candidates chunk_id=%s candidates=%d", chunk.ChunkID, len(params["import_candidates"].([]string)))
			if err := runNeo4j(ctx, tx, linkImportQuery, params, stats); err != nil {
				return nil, err
			}
		}
		if isClassChunk(chunk.SymbolType) {
			if err := runNeo4j(ctx, tx, linkClassToExistingMethodsQuery, params, stats); err != nil {
				return nil, err
			}
		}
		if isMethodChunk(chunk.SymbolType) {
			if err := runNeo4j(ctx, tx, linkMethodToExistingClassQuery, params, stats); err != nil {
				return nil, err
			}
		}
		if isDocCommentChunk(chunk.SymbolType) {
			if err := runNeo4j(ctx, tx, linkDocToNearestDeclarationQuery, params, stats); err != nil {
				return nil, err
			}
		}
		if isDeclarationChunk(chunk.SymbolType) {
			if err := runNeo4j(ctx, tx, linkNearestDocToDeclarationQuery, params, stats); err != nil {
				return nil, err
			}
			if err := runNeo4j(ctx, tx, linkAliasChunksQuery, params, stats); err != nil {
				return nil, err
			}
		}

		return stats, nil
	})
	if err != nil {
		return fmt.Errorf("index neo4j chunk %s: %w", chunk.ChunkID, err)
	}

	stats, _ := result.(*graphWriteStats)
	if stats == nil {
		stats = &graphWriteStats{}
	}
	i.logger.Printf(
		"neo4j index completed uri=%s chunk_id=%s file=%s symbol=%s type=%s nodes_created=%d relationships_created=%d properties_set=%d labels_added=%d duration_ms=%d",
		i.config.URI,
		chunk.ChunkID,
		chunk.FilePath,
		chunk.SymbolName,
		chunk.SymbolType,
		stats.nodesCreated,
		stats.relationshipsCreated,
		stats.propertiesSet,
		stats.labelsAdded,
		time.Since(started).Milliseconds(),
	)
	return nil
}

func (i *Neo4jIndexer) DeleteChunk(ctx context.Context, chunkID string) error {
	if err := i.ensureReady(ctx); err != nil {
		return err
	}

	session := i.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer func() {
		if err := session.Close(ctx); err != nil {
			i.logger.Printf("failed to close neo4j session chunk_id=%s: %v", chunkID, err)
		}
	}()

	started := time.Now()
	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		stats := &graphWriteStats{}
		return stats, runNeo4j(ctx, tx, deleteChunkQuery, map[string]any{"chunk_id": chunkID}, stats)
	})
	if err != nil {
		return fmt.Errorf("delete neo4j chunk %s: %w", chunkID, err)
	}

	stats, _ := result.(*graphWriteStats)
	if stats == nil {
		stats = &graphWriteStats{}
	}
	i.logger.Printf(
		"deleted chunk from neo4j uri=%s chunk_id=%s nodes_deleted=%d relationships_deleted=%d duration_ms=%d",
		i.config.URI,
		chunkID,
		stats.nodesDeleted,
		stats.relationshipsDeleted,
		time.Since(started).Milliseconds(),
	)
	return nil
}

func (i *Neo4jIndexer) ensureReady(ctx context.Context) error {
	if i.driverInitErr != nil {
		return i.driverInitErr
	}
	if i.driver == nil {
		return fmt.Errorf("neo4j driver is nil")
	}

	i.schemaMu.Lock()
	defer i.schemaMu.Unlock()
	if i.schemaReady {
		return nil
	}

	session := i.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer func() {
		if err := session.Close(ctx); err != nil {
			i.logger.Printf("failed to close neo4j schema session: %v", err)
		}
	}()

	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		if err := runNeo4j(ctx, tx, createChunkIDConstraintQuery, nil, nil); err != nil {
			return nil, err
		}
		return nil, nil
	})
	if err != nil {
		return fmt.Errorf("ensure neo4j schema: %w", err)
	}

	i.schemaReady = true
	return nil
}

func runNeo4j(ctx context.Context, tx neo4j.ManagedTransaction, cypher string, params map[string]any, stats *graphWriteStats) error {
	result, err := tx.Run(ctx, cypher, params)
	if err != nil {
		return err
	}
	summary, err := result.Consume(ctx)
	if err != nil {
		return err
	}
	if stats != nil {
		stats.add(summary)
	}
	return nil
}

type graphWriteStats struct {
	nodesCreated         int
	nodesDeleted         int
	relationshipsCreated int
	relationshipsDeleted int
	propertiesSet        int
	labelsAdded          int
}

func (s *graphWriteStats) add(summary neo4j.ResultSummary) {
	if summary == nil {
		return
	}
	counters := summary.Counters()
	s.nodesCreated += counters.NodesCreated()
	s.nodesDeleted += counters.NodesDeleted()
	s.relationshipsCreated += counters.RelationshipsCreated()
	s.relationshipsDeleted += counters.RelationshipsDeleted()
	s.propertiesSet += counters.PropertiesSet()
	s.labelsAdded += counters.LabelsAdded()
}

func chunkParams(chunk *domain.Chunk) map[string]any {
	return map[string]any{
		"chunk_id":       chunk.ChunkID,
		"repo_url":       chunk.RepoURL,
		"branch":         chunk.Branch,
		"commit_sha":     chunk.CommitSHA,
		"snapshot_id":    chunk.SnapshotID,
		"file_path":      chunk.FilePath,
		"file_name":      chunk.FileName,
		"file_chunk_id":  fileChunkID(chunk),
		"document_type":  chunk.DocumentType,
		"language":       chunk.Language,
		"content_hash":   chunk.ContentHash,
		"symbol_name":    chunk.SymbolName,
		"symbol_type":    chunk.SymbolType,
		"canonical_type": canonicalSymbolType(chunk.SymbolType),
		"start_line":     chunk.StartLine,
		"end_line":       chunk.EndLine,
		"start_char":     chunk.StartChar,
		"end_char":       chunk.EndChar,
		"prev_chunk_id":  chunk.PrevChunkID,
		"next_chunk_id":  chunk.NextChunkID,
		"created_at":     chunk.CreatedAt,
	}
}

func fileChunkID(chunk *domain.Chunk) string {
	return fmt.Sprintf("file:%s:%s:%s", chunk.RepoURL, chunk.SnapshotID, chunk.FilePath)
}

func canonicalSymbolType(symbolType string) string {
	switch strings.TrimSpace(symbolType) {
	case "symbol:class", "container:class":
		return "class"
	case "symbol:method", "container:method":
		return "method"
	case "symbol:module":
		return "module"
	default:
		return symbolType
	}
}

func isClassChunk(symbolType string) bool {
	return canonicalSymbolType(symbolType) == "class"
}

func isMethodChunk(symbolType string) bool {
	return canonicalSymbolType(symbolType) == "method"
}

func isImportChunk(symbolType string) bool {
	return symbolType == "import"
}

func isDocCommentChunk(symbolType string) bool {
	return symbolType == "doc_comment"
}

func isDeclarationChunk(symbolType string) bool {
	switch canonicalSymbolType(symbolType) {
	case "class", "method", "module":
		return true
	default:
		return false
	}
}

func importResolutionCandidates(filePath string, importPath string, language string) []string {
	importPath = strings.TrimSpace(strings.Trim(importPath, "\"'`"))
	if importPath == "" {
		return nil
	}

	candidates := make([]string, 0)
	add := func(candidate string) {
		candidate = path.Clean(candidate)
		for _, existing := range candidates {
			if existing == candidate {
				return
			}
		}
		candidates = append(candidates, candidate)
	}

	basePath := importPath
	if strings.HasPrefix(importPath, ".") {
		basePath = path.Join(path.Dir(filePath), importPath)
	}

	add(basePath)
	extensions := languageExtensions(language)
	if path.Ext(basePath) == "" {
		for _, extension := range extensions {
			add(basePath + extension)
		}
		for _, extension := range extensions {
			add(path.Join(basePath, "index"+extension))
		}
	}

	return candidates
}

func languageExtensions(language string) []string {
	switch strings.ToLower(strings.TrimSpace(language)) {
	case "typescript":
		return []string{".ts", ".tsx", ".js", ".jsx"}
	case "javascript":
		return []string{".js", ".jsx", ".ts", ".tsx"}
	case "python":
		return []string{".py"}
	case "go":
		return []string{".go"}
	case "java":
		return []string{".java"}
	default:
		return []string{".ts", ".tsx", ".js", ".jsx", ".py", ".go", ".java"}
	}
}

const createChunkIDConstraintQuery = `
CREATE CONSTRAINT chunk_id_unique IF NOT EXISTS
FOR (c:Chunk)
REQUIRE c.chunk_id IS UNIQUE
`

const upsertChunkQuery = `
MERGE (c:Chunk {chunk_id: $chunk_id})
SET
  c.repo_url = $repo_url,
  c.branch = $branch,
  c.commit_sha = $commit_sha,
  c.snapshot_id = $snapshot_id,
  c.file_path = $file_path,
  c.file_name = $file_name,
  c.document_type = $document_type,
  c.language = $language,
  c.content_hash = $content_hash,
  c.symbol_name = $symbol_name,
  c.symbol_type = $symbol_type,
  c.canonical_type = $canonical_type,
  c.start_line = $start_line,
  c.end_line = $end_line,
  c.start_char = $start_char,
  c.end_char = $end_char,
  c.prev_chunk_id = $prev_chunk_id,
  c.next_chunk_id = $next_chunk_id,
  c.created_at = $created_at,
  c.synthetic = false
`

const upsertFileContainmentQuery = `
MATCH (c:Chunk {chunk_id: $chunk_id})
MERGE (f:Chunk {chunk_id: $file_chunk_id})
SET
  f.repo_url = $repo_url,
  f.branch = $branch,
  f.commit_sha = $commit_sha,
  f.snapshot_id = $snapshot_id,
  f.file_path = $file_path,
  f.file_name = $file_name,
  f.document_type = $document_type,
  f.language = $language,
  f.symbol_name = $file_path,
  f.symbol_type = "file",
  f.canonical_type = "file",
  f.synthetic = true
MERGE (f)-[:CONTAINS]->(c)
`

const linkPrevChunkQuery = `
MATCH (c:Chunk {chunk_id: $chunk_id})
MERGE (prev:Chunk {chunk_id: $prev_chunk_id})
MERGE (prev)-[:NEXT]->(c)
MERGE (c)-[:PREV]->(prev)
`

const linkNextChunkQuery = `
MATCH (c:Chunk {chunk_id: $chunk_id})
MERGE (next:Chunk {chunk_id: $next_chunk_id})
MERGE (c)-[:NEXT]->(next)
MERGE (next)-[:PREV]->(c)
`

const linkImportQuery = `
MATCH (f:Chunk {chunk_id: $file_chunk_id})
MATCH (i:Chunk {chunk_id: $chunk_id})
MERGE (f)-[:HAS_IMPORT]->(i)
WITH i
MATCH (target:Chunk {
  repo_url: $repo_url,
  snapshot_id: $snapshot_id,
  symbol_type: "file"
})
WHERE target.file_path IN $import_candidates
MERGE (i)-[:RESOLVES_TO_FILE]->(target)
`

const linkClassToExistingMethodsQuery = `
MATCH (cls:Chunk {chunk_id: $chunk_id})
MATCH (m:Chunk {
  repo_url: $repo_url,
  snapshot_id: $snapshot_id,
  file_path: $file_path,
  canonical_type: "method"
})
WHERE m.chunk_id <> $chunk_id
  AND cls.start_char <= m.start_char
  AND cls.end_char >= m.end_char
MERGE (cls)-[:CONTAINS]->(m)
`

const linkMethodToExistingClassQuery = `
MATCH (m:Chunk {chunk_id: $chunk_id})
MATCH (cls:Chunk {
  repo_url: $repo_url,
  snapshot_id: $snapshot_id,
  file_path: $file_path,
  canonical_type: "class"
})
WHERE cls.chunk_id <> $chunk_id
  AND cls.start_char <= m.start_char
  AND cls.end_char >= m.end_char
MERGE (cls)-[:CONTAINS]->(m)
`

const linkDocToNearestDeclarationQuery = `
MATCH (doc:Chunk {chunk_id: $chunk_id})
MATCH (decl:Chunk {
  repo_url: $repo_url,
  snapshot_id: $snapshot_id,
  file_path: $file_path
})
WHERE decl.canonical_type IN ["class", "method", "module"]
  AND decl.start_char >= doc.end_char
WITH doc, decl
ORDER BY decl.start_char ASC
LIMIT 1
MERGE (doc)-[:DOCUMENTS]->(decl)
`

const linkNearestDocToDeclarationQuery = `
MATCH (decl:Chunk {chunk_id: $chunk_id})
MATCH (doc:Chunk {
  repo_url: $repo_url,
  snapshot_id: $snapshot_id,
  file_path: $file_path,
  symbol_type: "doc_comment"
})
WHERE doc.end_char <= decl.start_char
WITH decl, doc
ORDER BY doc.end_char DESC
LIMIT 1
MERGE (doc)-[:DOCUMENTS]->(decl)
`

const linkAliasChunksQuery = `
MATCH (c:Chunk {chunk_id: $chunk_id})
MATCH (other:Chunk {
  repo_url: $repo_url,
  snapshot_id: $snapshot_id,
  file_path: $file_path,
  symbol_name: $symbol_name,
  canonical_type: $canonical_type,
  start_char: $start_char,
  end_char: $end_char
})
WHERE other.chunk_id <> $chunk_id
MERGE (c)-[:ALIAS_OF]->(other)
`

const deleteChunkQuery = `
MATCH (c:Chunk {chunk_id: $chunk_id})
DETACH DELETE c
`
