package cognitive

import "testing"

func TestTokenize(t *testing.T) {
	tests := []struct {
		input string
		want  int // number of tokens
	}{
		{"hello world", 2},
		{"ModuleNotFoundError: No module named 'foo'", 4}, // module, not, named, foo (>2 chars)
		{"", 0},
		{"a b c", 0},        // all too short
		{"abc def ghi", 3},  // all 3+ chars
		{"HTTP 404 Error", 3}, // HTTP, 404, Error (all kept)
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			tokens := tokenize(tt.input)
			if len(tokens) != tt.want {
				t.Errorf("tokenize(%q) = %d tokens, want %d (got: %v)",
					tt.input, len(tokens), tt.want, tokens)
			}
		})
	}
}

func TestJaccardSimilarity(t *testing.T) {
	tests := []struct {
		name string
		a    map[string]bool
		b    map[string]bool
		want float64
	}{
		{
			name: "identical",
			a:    map[string]bool{"hello": true, "world": true},
			b:    map[string]bool{"hello": true, "world": true},
			want: 1.0,
		},
		{
			name: "no overlap",
			a:    map[string]bool{"hello": true, "world": true},
			b:    map[string]bool{"foo": true, "bar": true},
			want: 0.0,
		},
		{
			name: "partial overlap",
			a:    map[string]bool{"hello": true, "world": true},
			b:    map[string]bool{"hello": true, "foo": true},
			want: 0.333, // 1 intersection / 3 union
		},
		{
			name: "empty sets",
			a:    map[string]bool{},
			b:    map[string]bool{},
			want: 0.0,
		},
		{
			name: "one empty",
			a:    map[string]bool{"hello": true},
			b:    map[string]bool{},
			want: 0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := jaccardSimilarity(tt.a, tt.b)
			diff := result - tt.want
			if diff < -0.01 || diff > 0.01 {
				t.Errorf("jaccardSimilarity() = %f, want %f", result, tt.want)
			}
		})
	}
}

func TestToLowerCase(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Hello", "hello"},
		{"WORLD", "world"},
		{"MixedCase", "mixedcase"},
		{"already", "already"},
		{"123ABC", "123abc"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := toLowerCase(tt.input)
			if result != tt.want {
				t.Errorf("toLowerCase(%q) = %q, want %q", tt.input, result, tt.want)
			}
		})
	}
}
