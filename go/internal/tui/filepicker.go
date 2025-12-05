// Package tui provides the Bubble Tea interactive agent interface.
package tui

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sahilm/fuzzy"
)

// inputMode represents the current input mode
type inputMode int

const (
	modeChat inputMode = iota
	modeFilePicker
)

// fileItem implements list.Item for the file picker
type fileItem struct {
	path    string
	name    string
	isDir   bool
	relPath string
}

func (i fileItem) Title() string {
	prefix := "📄"
	if i.isDir {
		prefix = "📁"
	}
	return prefix + " " + i.relPath
}

func (i fileItem) Description() string { return i.path }
func (i fileItem) FilterValue() string { return i.relPath }

// fileItems is a slice of fileItem that implements fuzzy.Source
type fileItems []fileItem

func (f fileItems) String(i int) string { return f[i].relPath }
func (f fileItems) Len() int            { return len(f) }

// FilePicker handles @ file references
type FilePicker struct {
	list    list.Model
	items   fileItems
	workDir string
	filter  string
	width   int
	height  int
}

// NewFilePicker creates a new file picker
func NewFilePicker(workDir string, width, height int) *FilePicker {
	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = false
	delegate.SetHeight(1)

	// Style the list
	delegate.Styles.SelectedTitle = delegate.Styles.SelectedTitle.
		Foreground(lipgloss.Color("205")).
		BorderForeground(lipgloss.Color("205"))

	l := list.New([]list.Item{}, delegate, width, height)
	l.Title = "Select file (@)"
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	l.SetFilteringEnabled(true)
	l.Styles.Title = lipgloss.NewStyle().
		Foreground(lipgloss.Color("205")).
		Bold(true)

	return &FilePicker{
		list:    l,
		workDir: workDir,
		width:   width,
		height:  height,
	}
}

// LoadFiles scans the working directory and loads files
func (fp *FilePicker) LoadFiles() error {
	var items fileItems

	err := filepath.Walk(fp.workDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		// Skip hidden directories and common noise
		name := info.Name()
		if strings.HasPrefix(name, ".") && name != "." {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip common directories we don't want
		if info.IsDir() {
			switch name {
			case "node_modules", "vendor", "__pycache__", ".git", "dist", "build":
				return filepath.SkipDir
			}
		}

		// Get relative path
		relPath, err := filepath.Rel(fp.workDir, path)
		if err != nil {
			return nil
		}

		if relPath == "." {
			return nil
		}

		items = append(items, fileItem{
			path:    path,
			name:    name,
			isDir:   info.IsDir(),
			relPath: relPath,
		})

		return nil
	})

	if err != nil {
		return err
	}

	// Sort: directories first, then alphabetically
	sort.Slice(items, func(i, j int) bool {
		if items[i].isDir != items[j].isDir {
			return items[i].isDir
		}
		return items[i].relPath < items[j].relPath
	})

	fp.items = items
	fp.updateList("")
	return nil
}

// updateList updates the list with filtered items
func (fp *FilePicker) updateList(filter string) {
	fp.filter = filter

	var listItems []list.Item
	if filter == "" {
		for _, item := range fp.items {
			listItems = append(listItems, item)
		}
	} else {
		// Use fuzzy matching
		matches := fuzzy.FindFrom(filter, fp.items)
		for _, match := range matches {
			listItems = append(listItems, fp.items[match.Index])
		}
	}

	fp.list.SetItems(listItems)
}

// Update handles messages for the file picker
func (fp *FilePicker) Update(msg tea.Msg) (*FilePicker, tea.Cmd) {
	var cmd tea.Cmd
	fp.list, cmd = fp.list.Update(msg)
	return fp, cmd
}

// View renders the file picker
func (fp *FilePicker) View() string {
	return fp.list.View()
}

// SelectedItem returns the selected file path
func (fp *FilePicker) SelectedItem() (string, bool) {
	item, ok := fp.list.SelectedItem().(fileItem)
	if !ok {
		return "", false
	}
	return item.relPath, true
}

// SetSize updates the picker dimensions
func (fp *FilePicker) SetSize(width, height int) {
	fp.width = width
	fp.height = height
	fp.list.SetSize(width, height)
}
