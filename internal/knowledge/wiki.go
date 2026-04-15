package knowledge

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// WikiPage represents a compiled knowledge page.
type WikiPage struct {
	Name       string   `json:"name" yaml:"-"`
	Title      string   `json:"title" yaml:"title"`
	CompiledAt string   `json:"compiled_at" yaml:"compiled_at"`
	Sources    int      `json:"sources" yaml:"sources"`
	Entities   []string `json:"entities" yaml:"entities"`
	Content    string   `json:"content,omitempty" yaml:"-"`
}

// WikiManager manages compiled wiki pages in ~/.naozhi/wiki/.
type WikiManager struct {
	dir string
}

// NewWikiManager creates a WikiManager for the given directory.
func NewWikiManager(dir string) *WikiManager {
	os.MkdirAll(dir, 0o755)
	return &WikiManager{dir: dir}
}

// ListPages returns all wiki pages (frontmatter only, no content).
func (wm *WikiManager) ListPages() ([]WikiPage, error) {
	entries, err := os.ReadDir(wm.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var pages []WikiPage
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") || e.Name() == "CLAUDE.md" {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".md")
		page, err := wm.readFrontmatter(name)
		if err != nil {
			continue
		}
		pages = append(pages, page)
	}
	sort.Slice(pages, func(i, j int) bool {
		return pages[i].CompiledAt > pages[j].CompiledAt
	})
	return pages, nil
}

// ReadPage returns a wiki page with full content.
func (wm *WikiManager) ReadPage(name string) (WikiPage, error) {
	data, err := os.ReadFile(filepath.Join(wm.dir, name+".md"))
	if err != nil {
		return WikiPage{}, fmt.Errorf("wiki page not found: %s", name)
	}
	page, content := parseFrontmatterAndContent(data)
	page.Name = name
	page.Content = content
	return page, nil
}

// WritePage writes a wiki page with frontmatter.
func (wm *WikiManager) WritePage(name string, page WikiPage, content string) error {
	page.Name = name
	if page.CompiledAt == "" {
		page.CompiledAt = time.Now().Format("2006-01-02T15:04:05Z")
	}

	fm, err := yaml.Marshal(page)
	if err != nil {
		return err
	}
	data := fmt.Sprintf("---\n%s---\n\n%s", string(fm), content)

	tmpPath := filepath.Join(wm.dir, name+".md.tmp")
	finalPath := filepath.Join(wm.dir, name+".md")
	if err := os.WriteFile(tmpPath, []byte(data), 0o644); err != nil {
		return err
	}
	return os.Rename(tmpPath, finalPath)
}

// Dir returns the wiki directory path.
func (wm *WikiManager) Dir() string {
	return wm.dir
}

func (wm *WikiManager) readFrontmatter(name string) (WikiPage, error) {
	data, err := os.ReadFile(filepath.Join(wm.dir, name+".md"))
	if err != nil {
		return WikiPage{}, err
	}
	page, _ := parseFrontmatterAndContent(data)
	page.Name = name
	return page, nil
}

func parseFrontmatterAndContent(data []byte) (WikiPage, string) {
	var page WikiPage
	content := string(data)

	s := string(data)
	if !strings.HasPrefix(s, "---\n") {
		page.Title = extractTitle(s)
		return page, content
	}
	end := strings.Index(s[4:], "\n---")
	if end < 0 {
		page.Title = extractTitle(s)
		return page, content
	}
	fmData := s[4 : 4+end]
	content = strings.TrimSpace(s[4+end+4:])

	_ = yaml.Unmarshal([]byte(fmData), &page)
	if page.Title == "" {
		page.Title = extractTitle(content)
	}
	return page, content
}

func extractTitle(content string) string {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "# ") {
			return strings.TrimPrefix(line, "# ")
		}
	}
	return ""
}
