// Package backup provides knowledge backup and restore functionality.
package backup

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/joss/urp/internal/graph"
)

// KnowledgeType represents different types of stored knowledge.
type KnowledgeType string

const (
	TypeSolutions  KnowledgeType = "solutions"
	TypeMemories   KnowledgeType = "memories"
	TypeKnowledge  KnowledgeType = "knowledge"
	TypeSkills     KnowledgeType = "skills"
	TypeSessions   KnowledgeType = "sessions"
	TypeVectors    KnowledgeType = "vectors"
	TypeAll        KnowledgeType = "all"
)

// BackupMetadata contains backup information.
type BackupMetadata struct {
	Version     string            `json:"version"`
	CreatedAt   time.Time         `json:"created_at"`
	Project     string            `json:"project"`
	Description string            `json:"description"`
	Types       []KnowledgeType   `json:"types"`
	Counts      map[string]int    `json:"counts"`
	Checksums   map[string]string `json:"checksums"`
}

// BackupManager handles backup operations.
type BackupManager struct {
	db      graph.Driver
	dataDir string
}

// NewBackupManager creates a backup manager.
func NewBackupManager(db graph.Driver, dataDir string) *BackupManager {
	return &BackupManager{
		db:      db,
		dataDir: dataDir,
	}
}

// Export creates a compressed backup of specified knowledge types.
func (m *BackupManager) Export(ctx context.Context, types []KnowledgeType, outputPath string, description string) (*BackupMetadata, error) {
	// Create output file
	file, err := os.Create(outputPath)
	if err != nil {
		return nil, fmt.Errorf("creating backup file: %w", err)
	}
	defer file.Close()

	// Create gzip writer
	gzw := gzip.NewWriter(file)
	defer gzw.Close()

	// Create tar writer
	tw := tar.NewWriter(gzw)
	defer tw.Close()

	metadata := &BackupMetadata{
		Version:     "1.0",
		CreatedAt:   time.Now(),
		Project:     os.Getenv("PROJECT_NAME"),
		Description: description,
		Types:       types,
		Counts:      make(map[string]int),
		Checksums:   make(map[string]string),
	}

	// Check if "all" is requested
	if contains(types, TypeAll) {
		types = []KnowledgeType{TypeSolutions, TypeMemories, TypeKnowledge, TypeSkills, TypeSessions, TypeVectors}
	}

	// Export each type
	for _, t := range types {
		data, count, err := m.exportType(ctx, t)
		if err != nil {
			return nil, fmt.Errorf("exporting %s: %w", t, err)
		}

		if count > 0 {
			filename := fmt.Sprintf("%s.json", t)
			if err := addToTar(tw, filename, data); err != nil {
				return nil, fmt.Errorf("adding %s to tar: %w", t, err)
			}
			metadata.Counts[string(t)] = count
		}
	}

	// Add vector store file if exists and vectors requested
	if contains(types, TypeVectors) {
		vecPath := filepath.Join(m.dataDir, "vectors.json")
		if _, err := os.Stat(vecPath); err == nil {
			data, err := os.ReadFile(vecPath)
			if err == nil {
				if err := addToTar(tw, "vectors_store.json", data); err != nil {
					return nil, fmt.Errorf("adding vector store: %w", err)
				}
			}
		}
	}

	// Add metadata
	metaJSON, _ := json.MarshalIndent(metadata, "", "  ")
	if err := addToTar(tw, "metadata.json", metaJSON); err != nil {
		return nil, fmt.Errorf("adding metadata: %w", err)
	}

	return metadata, nil
}

// Import restores knowledge from a backup file.
func (m *BackupManager) Import(ctx context.Context, inputPath string, types []KnowledgeType, merge bool) (*BackupMetadata, error) {
	file, err := os.Open(inputPath)
	if err != nil {
		return nil, fmt.Errorf("opening backup: %w", err)
	}
	defer file.Close()

	// Create gzip reader
	gzr, err := gzip.NewReader(file)
	if err != nil {
		return nil, fmt.Errorf("gzip reader: %w", err)
	}
	defer gzr.Close()

	// Create tar reader
	tr := tar.NewReader(gzr)

	var metadata *BackupMetadata
	dataFiles := make(map[string][]byte)

	// Read all files from tar
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("reading tar: %w", err)
		}

		data, err := io.ReadAll(tr)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", header.Name, err)
		}

		if header.Name == "metadata.json" {
			metadata = &BackupMetadata{}
			if err := json.Unmarshal(data, metadata); err != nil {
				return nil, fmt.Errorf("parsing metadata: %w", err)
			}
		} else {
			dataFiles[header.Name] = data
		}
	}

	if metadata == nil {
		return nil, fmt.Errorf("backup missing metadata")
	}

	// Filter types if specified
	importTypes := types
	if len(types) == 0 || contains(types, TypeAll) {
		importTypes = metadata.Types
	}

	// Clear existing data if not merging
	if !merge {
		for _, t := range importTypes {
			if err := m.clearType(ctx, t); err != nil {
				return nil, fmt.Errorf("clearing %s: %w", t, err)
			}
		}
	}

	// Import each type
	for _, t := range importTypes {
		filename := fmt.Sprintf("%s.json", t)
		if data, ok := dataFiles[filename]; ok {
			if err := m.importType(ctx, t, data); err != nil {
				return nil, fmt.Errorf("importing %s: %w", t, err)
			}
		}
	}

	// Restore vector store if present
	if vecData, ok := dataFiles["vectors_store.json"]; ok && contains(importTypes, TypeVectors) {
		vecPath := filepath.Join(m.dataDir, "vectors.json")
		if err := os.WriteFile(vecPath, vecData, 0644); err != nil {
			return nil, fmt.Errorf("restoring vector store: %w", err)
		}
	}

	return metadata, nil
}

// List shows contents of a backup without importing.
func (m *BackupManager) List(inputPath string) (*BackupMetadata, error) {
	file, err := os.Open(inputPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	gzr, err := gzip.NewReader(file)
	if err != nil {
		return nil, err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		if header.Name == "metadata.json" {
			data, _ := io.ReadAll(tr)
			var meta BackupMetadata
			if err := json.Unmarshal(data, &meta); err != nil {
				return nil, err
			}
			return &meta, nil
		}
	}

	return nil, fmt.Errorf("metadata not found")
}

// exportType extracts data for a specific knowledge type.
func (m *BackupManager) exportType(ctx context.Context, t KnowledgeType) ([]byte, int, error) {
	var query string
	switch t {
	case TypeSolutions:
		query = `MATCH (s:Solution) RETURN s`
	case TypeMemories:
		query = `MATCH (m:Memo) RETURN m`
	case TypeKnowledge:
		query = `MATCH (k:KnowledgeEntry) RETURN k`
	case TypeSkills:
		query = `MATCH (sk:Skill) RETURN sk`
	case TypeSessions:
		query = `MATCH (s:Session:OpenCode) OPTIONAL MATCH (s)-[:HAS_MESSAGE]->(msg:Message) RETURN s, collect(msg) as messages`
	case TypeVectors:
		query = `MATCH (v:Vector) RETURN v`
	default:
		return nil, 0, fmt.Errorf("unknown type: %s", t)
	}

	records, err := m.db.Execute(ctx, query, nil)
	if err != nil {
		return nil, 0, err
	}

	data, err := json.MarshalIndent(records, "", "  ")
	return data, len(records), err
}

// importType restores data for a specific knowledge type.
func (m *BackupManager) importType(ctx context.Context, t KnowledgeType, data []byte) error {
	var records []map[string]any
	if err := json.Unmarshal(data, &records); err != nil {
		return err
	}

	if len(records) == 0 {
		return nil
	}

	// Build UNWIND query based on type
	var query string
	switch t {
	case TypeSolutions:
		query = `
			UNWIND $items AS item
			CREATE (s:Solution)
			SET s = item.s
		`
	case TypeMemories:
		query = `
			UNWIND $items AS item
			CREATE (m:Memo)
			SET m = item.m
		`
	case TypeKnowledge:
		query = `
			UNWIND $items AS item
			CREATE (k:KnowledgeEntry)
			SET k = item.k
		`
	case TypeSkills:
		query = `
			UNWIND $items AS item
			CREATE (sk:Skill)
			SET sk = item.sk
		`
	case TypeSessions:
		// Sessions need special handling due to relationships
		for _, r := range records {
			sessData := r["s"]
			if sessData == nil {
				continue
			}
			// Create session
			if err := m.db.ExecuteWrite(ctx, `CREATE (s:Session:OpenCode) SET s = $props`, map[string]any{"props": sessData}); err != nil {
				return err
			}
			// Create messages
			if msgs, ok := r["messages"].([]any); ok {
				for _, msg := range msgs {
					if msgMap, ok := msg.(map[string]any); ok {
						if err := m.db.ExecuteWrite(ctx, `
							MATCH (s:Session:OpenCode {id: $sessId})
							CREATE (m:Message:OpenCode)
							SET m = $props
							CREATE (s)-[:HAS_MESSAGE]->(m)
						`, map[string]any{
							"sessId": sessData.(map[string]any)["id"],
							"props":  msgMap,
						}); err != nil {
							return err
						}
					}
				}
			}
		}
		return nil
	case TypeVectors:
		query = `
			UNWIND $items AS item
			CREATE (v:Vector)
			SET v = item.v
		`
	default:
		return fmt.Errorf("unknown type: %s", t)
	}

	return m.db.ExecuteWrite(ctx, query, map[string]any{"items": records})
}

// clearType removes all data of a specific type.
func (m *BackupManager) clearType(ctx context.Context, t KnowledgeType) error {
	var query string
	switch t {
	case TypeSolutions:
		query = `MATCH (s:Solution) DETACH DELETE s`
	case TypeMemories:
		query = `MATCH (m:Memo) DETACH DELETE m`
	case TypeKnowledge:
		query = `MATCH (k:KnowledgeEntry) DETACH DELETE k`
	case TypeSkills:
		query = `MATCH (sk:Skill) DETACH DELETE sk`
	case TypeSessions:
		query = `MATCH (s:Session:OpenCode) OPTIONAL MATCH (s)-[:HAS_MESSAGE]->(m) DETACH DELETE s, m`
	case TypeVectors:
		query = `MATCH (v:Vector) DETACH DELETE v`
	default:
		return nil
	}
	return m.db.ExecuteWrite(ctx, query, nil)
}

func addToTar(tw *tar.Writer, name string, data []byte) error {
	header := &tar.Header{
		Name:    name,
		Mode:    0644,
		Size:    int64(len(data)),
		ModTime: time.Now(),
	}
	if err := tw.WriteHeader(header); err != nil {
		return err
	}
	_, err := tw.Write(data)
	return err
}

func contains(types []KnowledgeType, t KnowledgeType) bool {
	for _, kt := range types {
		if kt == t {
			return true
		}
	}
	return false
}
