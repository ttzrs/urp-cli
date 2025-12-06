package ingest

import (
	"testing"

	"github.com/joss/urp/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- GoParser Tests ---

func TestGoParserExtensions(t *testing.T) {
	p := NewGoParser()
	exts := p.Extensions()
	assert.Equal(t, []string{".go"}, exts)
}

func TestGoParserParseFunction(t *testing.T) {
	p := NewGoParser()
	content := []byte(`package main

func hello(name string) string {
	return "Hello, " + name
}
`)
	entities, rels, err := p.Parse("/test/main.go", content)
	require.NoError(t, err)

	// Should have file + function
	assert.Len(t, entities, 2)

	// Find file entity
	var fileEntity, funcEntity *domain.Entity
	for i := range entities {
		if entities[i].Type == domain.EntityFile {
			fileEntity = &entities[i]
		}
		if entities[i].Type == domain.EntityFunction {
			funcEntity = &entities[i]
		}
	}

	require.NotNil(t, fileEntity)
	assert.Equal(t, "/test/main.go", fileEntity.Path)

	require.NotNil(t, funcEntity)
	assert.Equal(t, "hello", funcEntity.Name)
	assert.Contains(t, funcEntity.Signature, "string")

	// Should have CONTAINS relationship
	assert.Len(t, rels, 1)
	assert.Equal(t, "CONTAINS", rels[0].Type)
}

func TestGoParserParseMethod(t *testing.T) {
	p := NewGoParser()
	content := []byte(`package main

type Server struct{}

func (s *Server) Start() error {
	return nil
}
`)
	entities, _, err := p.Parse("/test/server.go", content)
	require.NoError(t, err)

	// Should have file + struct + method
	assert.Len(t, entities, 3)

	var methodEntity *domain.Entity
	for i := range entities {
		if entities[i].Type == domain.EntityMethod {
			methodEntity = &entities[i]
		}
	}

	require.NotNil(t, methodEntity)
	assert.Equal(t, "Server.Start", methodEntity.Name)
}

func TestGoParserParseStruct(t *testing.T) {
	p := NewGoParser()
	content := []byte(`package main

type Config struct {
	Host string
	Port int
}
`)
	entities, _, err := p.Parse("/test/config.go", content)
	require.NoError(t, err)

	var structEntity *domain.Entity
	for i := range entities {
		if entities[i].Type == domain.EntityStruct {
			structEntity = &entities[i]
		}
	}

	require.NotNil(t, structEntity)
	assert.Equal(t, "Config", structEntity.Name)
}

func TestGoParserParseInterface(t *testing.T) {
	p := NewGoParser()
	content := []byte(`package main

type Reader interface {
	Read(p []byte) (n int, err error)
}
`)
	entities, _, err := p.Parse("/test/reader.go", content)
	require.NoError(t, err)

	var ifaceEntity *domain.Entity
	for i := range entities {
		if entities[i].Type == domain.EntityInterface {
			ifaceEntity = &entities[i]
		}
	}

	require.NotNil(t, ifaceEntity)
	assert.Equal(t, "Reader", ifaceEntity.Name)
}

func TestGoParserParseCalls(t *testing.T) {
	p := NewGoParser()
	content := []byte(`package main

import "fmt"

func greet(name string) {
	fmt.Println("Hello,", name)
	helper()
}

func helper() {}
`)
	_, rels, err := p.Parse("/test/main.go", content)
	require.NoError(t, err)

	// Should have CONTAINS + CALLS relationships
	var calls []domain.Relationship
	for _, r := range rels {
		if r.Type == "CALLS" {
			calls = append(calls, r)
		}
	}

	assert.GreaterOrEqual(t, len(calls), 2) // fmt.Println and helper
}

func TestGoParserInvalidSyntax(t *testing.T) {
	p := NewGoParser()
	content := []byte(`package main

func broken( {
`)
	_, _, err := p.Parse("/test/broken.go", content)
	assert.Error(t, err)
}

// --- Helper Function Tests ---

func TestBuildFuncSignature(t *testing.T) {
	p := NewGoParser()

	tests := []struct {
		code string
		want string
	}{
		{
			code: `package main
func simple() {}`,
			want: "simple()",
		},
		{
			code: `package main
func withParams(a int, b string) {}`,
			want: "withParams(int, string)",
		},
		{
			code: `package main
func withReturn() error { return nil }`,
			want: "withReturn() error",
		},
		{
			code: `package main
func multiReturn(s string) (int, error) { return 0, nil }`,
			want: "multiReturn(string) (int, error)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			entities, _, err := p.Parse("/test/test.go", []byte(tt.code))
			require.NoError(t, err)

			var funcEntity *domain.Entity
			for i := range entities {
				if entities[i].Type == domain.EntityFunction {
					funcEntity = &entities[i]
				}
			}

			require.NotNil(t, funcEntity)
			assert.Equal(t, tt.want, funcEntity.Signature)
		})
	}
}

func TestExprToString(t *testing.T) {
	p := NewGoParser()

	// Test via parsing actual code with various types
	code := `package main

func test(
	a int,
	b *string,
	c []byte,
	d map[string]int,
	e interface{},
	f func(),
	g ...string,
) {}
`
	entities, _, err := p.Parse("/test/types.go", []byte(code))
	require.NoError(t, err)

	var funcEntity *domain.Entity
	for i := range entities {
		if entities[i].Type == domain.EntityFunction {
			funcEntity = &entities[i]
		}
	}

	require.NotNil(t, funcEntity)
	sig := funcEntity.Signature

	assert.Contains(t, sig, "int")
	assert.Contains(t, sig, "*string")
	assert.Contains(t, sig, "[]byte")
	assert.Contains(t, sig, "map[string]int")
	assert.Contains(t, sig, "interface{}")
	assert.Contains(t, sig, "func")
	assert.Contains(t, sig, "...string")
}

func TestGoParserLineNumbers(t *testing.T) {
	p := NewGoParser()
	content := []byte(`package main

// Line 3
func multiline(
	a int,
	b string,
) error {
	// Line 8
	return nil
}
// Line 11
`)
	entities, _, err := p.Parse("/test/lines.go", content)
	require.NoError(t, err)

	var funcEntity *domain.Entity
	for i := range entities {
		if entities[i].Type == domain.EntityFunction {
			funcEntity = &entities[i]
		}
	}

	require.NotNil(t, funcEntity)
	assert.Equal(t, 4, funcEntity.StartLine) // func starts at line 4
	assert.Equal(t, 10, funcEntity.EndLine)  // closing brace at line 10
}
