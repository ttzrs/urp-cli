// Package ingest provides table schema parsing for CSV/TSV/JSON files.
package ingest

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/joss/urp/internal/domain"
)

// TableParser extracts schema from tabular data files.
// Only reads headers and first few rows to infer types.
type TableParser struct{}

func NewTableParser() *TableParser {
	return &TableParser{}
}

func (p *TableParser) Extensions() []string {
	return []string{".csv", ".tsv", ".json"}
}

// ColumnSchema represents a table column's metadata.
type ColumnSchema struct {
	Name    string `json:"name"`
	Type    string `json:"type"`    // string, int, float, bool, datetime, null
	Example string `json:"example"` // First non-null value
}

// TableSchema represents the structure of a data file.
type TableSchema struct {
	Path        string         `json:"path"`
	Columns     []ColumnSchema `json:"columns"`
	RowCount    int            `json:"row_count"`    // Estimated or exact
	FileSizeKB  int64          `json:"file_size_kb"` // File size in KB
	Description string         `json:"description"`  // From README or docstring
}

// Parse extracts schema from a table file without loading all data.
func (p *TableParser) Parse(path string, content []byte) ([]domain.Entity, []domain.Relationship, error) {
	ext := strings.ToLower(filepath.Ext(path))

	var schema *TableSchema
	var err error

	switch ext {
	case ".csv":
		schema, err = p.parseCSV(path, ',')
	case ".tsv":
		schema, err = p.parseCSV(path, '\t')
	case ".json":
		schema, err = p.parseJSON(path)
	default:
		return nil, nil, nil
	}

	if err != nil || schema == nil {
		return nil, nil, err
	}

	// Create File entity with schema metadata
	schemaJSON, _ := json.Marshal(schema.Columns)

	entities := []domain.Entity{
		{
			ID:        path,
			Type:      domain.EntityFile,
			Name:      filepath.Base(path),
			Path:      path,
			Signature: string(schemaJSON), // Store schema in signature field
		},
	}

	return entities, nil, nil
}

// parseCSV reads only headers and sample rows from CSV/TSV.
func (p *TableParser) parseCSV(path string, delimiter rune) (*TableSchema, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// Get file size
	stat, _ := f.Stat()
	fileSizeKB := stat.Size() / 1024

	reader := csv.NewReader(f)
	reader.Comma = delimiter
	reader.LazyQuotes = true
	reader.TrimLeadingSpace = true

	// Read header
	headers, err := reader.Read()
	if err != nil {
		return nil, err
	}

	// Initialize columns
	columns := make([]ColumnSchema, len(headers))
	for i, h := range headers {
		columns[i] = ColumnSchema{
			Name: strings.TrimSpace(h),
			Type: "null",
		}
	}

	// Read up to 10 sample rows to infer types
	sampleRows := 10
	rowCount := 0
	for i := 0; i < sampleRows; i++ {
		row, err := reader.Read()
		if err != nil {
			break // EOF or error
		}
		rowCount++

		for j, val := range row {
			if j >= len(columns) {
				continue
			}
			val = strings.TrimSpace(val)

			// Set example if not set
			if columns[j].Example == "" && val != "" {
				columns[j].Example = truncateValue(val, 50)
			}

			// Infer type (upgrade from null → specific type)
			inferredType := inferType(val)
			columns[j].Type = mergeTypes(columns[j].Type, inferredType)
		}
	}

	// Estimate total rows for large files (avg bytes per row)
	estimatedRows := rowCount
	if rowCount == sampleRows && fileSizeKB > 10 {
		// Seek to estimate based on bytes read
		pos, _ := f.Seek(0, 1) // Current position
		if pos > 0 {
			estimatedRows = int(stat.Size() / (pos / int64(rowCount+1)))
		}
	}

	// Look for README in same directory
	desc := findDatasetDescription(path)

	return &TableSchema{
		Path:        path,
		Columns:     columns,
		RowCount:    estimatedRows,
		FileSizeKB:  fileSizeKB,
		Description: desc,
	}, nil
}

// parseJSON extracts schema from JSON (array of objects or single object).
func (p *TableParser) parseJSON(path string) (*TableSchema, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// Read first 64KB to detect structure
	buf := make([]byte, 64*1024)
	n, _ := f.Read(buf)
	content := string(buf[:n])

	// Try to parse as JSON array
	var arr []map[string]any
	if err := json.Unmarshal([]byte(content), &arr); err == nil && len(arr) > 0 {
		return p.schemaFromObjects(path, arr[:min(10, len(arr))])
	}

	// Try single object
	var obj map[string]any
	if err := json.Unmarshal([]byte(content), &obj); err == nil {
		return p.schemaFromObjects(path, []map[string]any{obj})
	}

	// Try JSONL (first few lines)
	f.Seek(0, 0)
	scanner := bufio.NewScanner(f)
	var objects []map[string]any
	for i := 0; i < 10 && scanner.Scan(); i++ {
		var obj map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &obj); err == nil {
			objects = append(objects, obj)
		}
	}
	if len(objects) > 0 {
		return p.schemaFromObjects(path, objects)
	}

	return nil, nil
}

func (p *TableParser) schemaFromObjects(path string, objects []map[string]any) (*TableSchema, error) {
	// Collect all keys
	keys := make(map[string]ColumnSchema)
	keyOrder := []string{}

	for _, obj := range objects {
		for k, v := range obj {
			if _, exists := keys[k]; !exists {
				keyOrder = append(keyOrder, k)
				keys[k] = ColumnSchema{Name: k, Type: "null"}
			}

			col := keys[k]
			valStr := fmt.Sprintf("%v", v)

			if col.Example == "" && v != nil {
				col.Example = truncateValue(valStr, 50)
			}

			col.Type = mergeTypes(col.Type, inferTypeFromValue(v))
			keys[k] = col
		}
	}

	// Build ordered columns
	columns := make([]ColumnSchema, 0, len(keyOrder))
	for _, k := range keyOrder {
		columns = append(columns, keys[k])
	}

	return &TableSchema{
		Path:     path,
		Columns:  columns,
		RowCount: -1,
	}, nil
}

// inferType guesses the type from a string value.
func inferType(val string) string {
	if val == "" {
		return "null"
	}

	// Try int
	if _, err := strconv.ParseInt(val, 10, 64); err == nil {
		return "int"
	}

	// Try float
	if _, err := strconv.ParseFloat(val, 64); err == nil {
		return "float"
	}

	// Try bool
	lower := strings.ToLower(val)
	if lower == "true" || lower == "false" {
		return "bool"
	}

	// Check for datetime patterns
	if looksLikeDatetime(val) {
		return "datetime"
	}

	return "string"
}

// inferTypeFromValue guesses type from a Go value.
func inferTypeFromValue(v any) string {
	switch v.(type) {
	case nil:
		return "null"
	case bool:
		return "bool"
	case float64:
		// JSON numbers are float64
		return "float"
	case string:
		return "string"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	default:
		return "any"
	}
}

// mergeTypes combines two types (type widening).
func mergeTypes(existing, new string) string {
	if existing == "null" {
		return new
	}
	if new == "null" {
		return existing
	}
	if existing == new {
		return existing
	}

	// int + float = float
	if (existing == "int" && new == "float") || (existing == "float" && new == "int") {
		return "float"
	}

	// Anything else = string (most general)
	return "string"
}

func looksLikeDatetime(val string) bool {
	// Common datetime patterns
	patterns := []string{
		"2006-01-02",
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"01/02/2006",
		"02-01-2006",
	}
	for _, p := range patterns {
		if len(val) >= len(p) && (val[4] == '-' || val[2] == '/' || val[2] == '-') {
			return true
		}
	}
	return false
}

func truncateValue(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// findDatasetDescription looks for README or documentation in the dataset directory.
func findDatasetDescription(dataPath string) string {
	dir := filepath.Dir(dataPath)

	// Look for common documentation files
	docFiles := []string{
		"README.md", "README.txt", "README",
		"DESCRIPTION.md", "DESCRIPTION.txt",
		"DATA.md", "SCHEMA.md",
		filepath.Base(dataPath) + ".md", // e.g., users.csv.md
	}

	for _, doc := range docFiles {
		docPath := filepath.Join(dir, doc)
		content, err := os.ReadFile(docPath)
		if err == nil {
			// Return first 500 chars of description
			desc := strings.TrimSpace(string(content))
			if len(desc) > 500 {
				// Find end of first paragraph
				if idx := strings.Index(desc[100:], "\n\n"); idx > 0 {
					desc = desc[:100+idx]
				} else {
					desc = desc[:497] + "..."
				}
			}
			return desc
		}
	}

	return ""
}

// Verify interface
var _ Parser = (*TableParser)(nil)
