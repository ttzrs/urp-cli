package specs

import (
	"bytes"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Spec represents a parsed specification with frontmatter
type Spec struct {
	// Frontmatter metadata
	ID      string   `yaml:"id"`
	Title   string   `yaml:"title"`
	Status  string   `yaml:"status"`  // draft, approved, implemented
	Owner   string   `yaml:"owner"`
	Type    string   `yaml:"type"`    // feature, bugfix, refactor
	Context []string `yaml:"context"` // files to include as context

	// Parsed content
	RawBody      string
	Requirements []Requirement
	Plan         []string // Numbered plan steps
}

// Requirement represents a checkbox item
type Requirement struct {
	Text     string
	Complete bool
}

// ParseSpec reads a markdown file and extracts frontmatter + content
func ParseSpec(filePath string) (*Spec, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	return ParseSpecContent(content)
}

// ParseSpecContent parses spec content from bytes
func ParseSpecContent(content []byte) (*Spec, error) {
	// Split frontmatter from body
	// Format: ---\nYAML\n---\nMarkdown
	parts := bytes.SplitN(content, []byte("---"), 3)

	spec := &Spec{}

	if len(parts) >= 3 {
		// Has frontmatter
		if err := yaml.Unmarshal(bytes.TrimSpace(parts[1]), spec); err != nil {
			// Ignore YAML errors, treat as no frontmatter
			spec.RawBody = string(content)
		} else {
			spec.RawBody = string(bytes.TrimSpace(parts[2]))
		}
	} else {
		// No frontmatter
		spec.RawBody = string(content)
	}

	// Extract requirements (checkboxes)
	spec.Requirements = extractRequirements(spec.RawBody)

	// Extract plan steps
	spec.Plan = extractPlanSteps(spec.RawBody)

	return spec, nil
}

// extractRequirements finds all checkbox items in the markdown
func extractRequirements(body string) []Requirement {
	var reqs []Requirement
	lines := strings.Split(body, "\n")

	for _, line := range lines {
		trim := strings.TrimSpace(line)

		// Match "- [ ]" or "- [x]" or "- [X]"
		if strings.HasPrefix(trim, "- [ ]") {
			text := strings.TrimSpace(strings.TrimPrefix(trim, "- [ ]"))
			reqs = append(reqs, Requirement{Text: text, Complete: false})
		} else if strings.HasPrefix(trim, "- [x]") || strings.HasPrefix(trim, "- [X]") {
			text := strings.TrimSpace(strings.TrimPrefix(trim, "- [x]"))
			text = strings.TrimSpace(strings.TrimPrefix(text, "- [X]"))
			reqs = append(reqs, Requirement{Text: text, Complete: true})
		}
	}

	return reqs
}

// extractPlanSteps finds numbered plan items
func extractPlanSteps(body string) []string {
	var steps []string
	lines := strings.Split(body, "\n")
	inPlan := false

	for _, line := range lines {
		trim := strings.TrimSpace(line)

		// Detect plan section
		if strings.HasPrefix(strings.ToLower(trim), "# plan") ||
			strings.HasPrefix(strings.ToLower(trim), "## plan") {
			inPlan = true
			continue
		}

		// End plan section on next header
		if inPlan && strings.HasPrefix(trim, "#") {
			inPlan = false
			continue
		}

		// Extract numbered items in plan
		if inPlan && len(trim) > 2 {
			if trim[0] >= '1' && trim[0] <= '9' && trim[1] == '.' {
				step := strings.TrimSpace(trim[2:])
				steps = append(steps, step)
			}
		}
	}

	return steps
}

// MarkComplete updates a requirement as complete and rewrites the file
func (s *Spec) MarkComplete(index int, filePath string) error {
	if index < 0 || index >= len(s.Requirements) {
		return nil
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	text := s.Requirements[index].Text
	oldCheckbox := "- [ ] " + text
	newCheckbox := "- [x] " + text

	newContent := strings.Replace(string(content), oldCheckbox, newCheckbox, 1)

	return os.WriteFile(filePath, []byte(newContent), 0644)
}

// FormatRequirements returns requirements as formatted string
func (s *Spec) FormatRequirements() string {
	if len(s.Requirements) == 0 {
		return "No requirements defined"
	}

	var b strings.Builder
	for _, r := range s.Requirements {
		check := "[ ]"
		if r.Complete {
			check = "[x]"
		}
		b.WriteString("- ")
		b.WriteString(check)
		b.WriteString(" ")
		b.WriteString(r.Text)
		b.WriteString("\n")
	}
	return b.String()
}

// FormatPlan returns plan steps as formatted string
func (s *Spec) FormatPlan() string {
	if len(s.Plan) == 0 {
		return ""
	}

	var b strings.Builder
	for i, step := range s.Plan {
		b.WriteString(strings.Repeat(" ", 0))
		b.WriteString(string(rune('1' + i)))
		b.WriteString(". ")
		b.WriteString(step)
		b.WriteString("\n")
	}
	return b.String()
}

// Progress returns completion percentage
func (s *Spec) Progress() float64 {
	if len(s.Requirements) == 0 {
		return 0
	}

	complete := 0
	for _, r := range s.Requirements {
		if r.Complete {
			complete++
		}
	}

	return float64(complete) / float64(len(s.Requirements)) * 100
}
