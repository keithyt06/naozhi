package knowledge

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
	"go.abhg.dev/goldmark/frontmatter"
)

// VaultConfig holds Obsidian vault configuration.
type VaultConfig struct {
	VaultPath    string   `yaml:"vault_path"`
	IncludePaths []string `yaml:"include_paths"`
	ExcludePaths []string `yaml:"exclude_paths"`
}

const treeCacheTTL = 60 * time.Second

// Vault provides Obsidian vault file reading and Markdown rendering.
type Vault struct {
	cfg          VaultConfig
	md           goldmark.Markdown
	treeMu       sync.RWMutex
	cachedTree   *TreeNode
	treeScannedAt time.Time
}

// NewVault creates a Vault with goldmark rendering pipeline.
func NewVault(cfg VaultConfig) *Vault {
	md := goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,
			&frontmatter.Extender{},
		),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
		),
		goldmark.WithRendererOptions(
			html.WithUnsafe(),
		),
	)
	return &Vault{cfg: cfg, md: md}
}

// ReadFile returns raw markdown content of a vault file.
func (v *Vault) ReadFile(relPath string) ([]byte, error) {
	absPath := filepath.Join(v.cfg.VaultPath, relPath)
	absPath = filepath.Clean(absPath)
	// Security: ensure path is within vault
	if !strings.HasPrefix(absPath, filepath.Clean(v.cfg.VaultPath)) {
		return nil, fmt.Errorf("path escapes vault: %s", relPath)
	}
	return os.ReadFile(absPath)
}

// ResolveImagePath tries to find an image file in common Obsidian asset locations.
// It returns the resolved relative path and true if found, or the original path and false.
func (v *Vault) ResolveImagePath(relPath string) (string, bool) {
	// Try the path as-is first
	candidates := []string{
		relPath,
		"assets/" + relPath,
		"Attachments/" + relPath,
	}
	// Also try with just the filename in case relPath has directory parts
	base := filepath.Base(relPath)
	if base != relPath {
		candidates = append(candidates, base, "assets/"+base, "Attachments/"+base)
	}
	for _, candidate := range candidates {
		absPath := filepath.Join(v.cfg.VaultPath, candidate)
		absPath = filepath.Clean(absPath)
		if !strings.HasPrefix(absPath, filepath.Clean(v.cfg.VaultPath)) {
			continue
		}
		if _, err := os.Stat(absPath); err == nil {
			return candidate, true
		}
	}
	return relPath, false
}

// RenderFile reads and renders a vault Markdown file to HTML.
func (v *Vault) RenderFile(relPath string) (string, map[string]interface{}, error) {
	src, err := v.ReadFile(relPath)
	if err != nil {
		return "", nil, err
	}
	return v.Render(src)
}

// Render converts markdown bytes to HTML, returning rendered HTML and frontmatter.
func (v *Vault) Render(src []byte) (string, map[string]interface{}, error) {
	// Pre-process image embeds: ![[image.png]] → <img> tag
	processed := processImageEmbeds(src)

	// Pre-process wikilinks: [[target]] → <a class="wikilink">target</a>
	// and [[target|alias]] → <a class="wikilink">alias</a>
	processed = processWikilinks(processed)

	// Pre-process callouts: > [!type] → <div class="callout type">
	processed = processCallouts(processed)

	var buf bytes.Buffer
	ctx := parser.NewContext()
	if err := v.md.Convert(processed, &buf, parser.WithContext(ctx)); err != nil {
		return "", nil, err
	}

	// Extract frontmatter
	fm := make(map[string]interface{})
	d := frontmatter.Get(ctx)
	if d != nil {
		_ = d.Decode(&fm)
	}

	return sanitizeHTML(buf.String()), fm, nil
}

// sanitizeHTML strips dangerous HTML constructs (script tags, on* event handlers,
// javascript: URIs) from rendered vault content to prevent XSS. This is a lightweight
// alternative to a full HTML sanitizer library -- it allows the safe Obsidian HTML
// (div, span, class, data-*, a, img, table, code, pre) while blocking injection vectors.
var (
	reScript    = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`)
	reOnHandler = regexp.MustCompile(`(?i)\bon\w+\s*=`)
	reJSURI     = regexp.MustCompile(`(?i)javascript:`)
)

func sanitizeHTML(html string) string {
	html = reScript.ReplaceAllString(html, "")
	html = reOnHandler.ReplaceAllString(html, "data-blocked-")
	html = reJSURI.ReplaceAllString(html, "blocked:")
	return html
}

// Configured returns true if the vault has a configured path.
func (v *Vault) Configured() bool {
	return v.cfg.VaultPath != ""
}

var imageEmbedRe = regexp.MustCompile(`!\[\[([^\]]+\.(?:png|jpg|jpeg|gif|svg|webp))\]\]`)

func processImageEmbeds(src []byte) []byte {
	return imageEmbedRe.ReplaceAllFunc(src, func(match []byte) []byte {
		parts := imageEmbedRe.FindSubmatch(match)
		filename := string(parts[1])
		// Pass the filename to the raw handler; it will resolve asset paths
		return []byte(fmt.Sprintf(`<img class="obs-img" src="/api/vault/raw?path=%s" alt="%s">`, filename, filename))
	})
}

var wikilinkRe = regexp.MustCompile(`\[\[([^\]|]+)(?:\|([^\]]+))?\]\]`)

func processWikilinks(src []byte) []byte {
	return wikilinkRe.ReplaceAllFunc(src, func(match []byte) []byte {
		parts := wikilinkRe.FindSubmatch(match)
		target := string(parts[1])
		display := target
		if len(parts) > 2 && len(parts[2]) > 0 {
			display = string(parts[2])
		}
		return []byte(fmt.Sprintf(`<a class="wikilink" data-target="%s">%s</a>`, target, display))
	})
}

var calloutRe = regexp.MustCompile(`(?m)^> \[!(\w+)\]\s*(.*)$`)

func processCallouts(src []byte) []byte {
	lines := bytes.Split(src, []byte("\n"))
	var result [][]byte
	inCallout := false
	calloutType := ""

	for _, line := range lines {
		if m := calloutRe.FindSubmatch(line); m != nil {
			if inCallout {
				result = append(result, []byte("</div>"))
			}
			calloutType = strings.ToLower(string(m[1]))
			title := string(m[2])
			if title == "" {
				title = capitalize(calloutType)
			}
			result = append(result, []byte(fmt.Sprintf(`<div class="callout %s"><div class="callout-title">%s</div>`, calloutType, title)))
			inCallout = true
			continue
		}
		if inCallout {
			trimmed := bytes.TrimPrefix(line, []byte("> "))
			trimmed = bytes.TrimPrefix(trimmed, []byte(">"))
			if len(line) > 0 && line[0] != '>' {
				result = append(result, []byte("</div>"))
				inCallout = false
				result = append(result, line)
			} else {
				result = append(result, trimmed)
			}
		} else {
			result = append(result, line)
		}
	}
	if inCallout {
		result = append(result, []byte("</div>"))
	}
	return bytes.Join(result, []byte("\n"))
}

// IsExcluded checks if a path should be excluded from vault browsing.
func (v *Vault) IsExcluded(relPath string) bool {
	for _, exc := range v.cfg.ExcludePaths {
		if strings.HasPrefix(relPath, exc) {
			return true
		}
	}
	return false
}

// IsIncluded checks if a path is within configured include paths.
// If no include paths configured, everything is included.
func (v *Vault) IsIncluded(relPath string) bool {
	if len(v.cfg.IncludePaths) == 0 {
		return true
	}
	for _, inc := range v.cfg.IncludePaths {
		if strings.HasPrefix(relPath, inc) {
			return true
		}
	}
	return false
}

// TreeNode represents a file or directory in the vault tree.
type TreeNode struct {
	Name      string      `json:"name"`
	Path      string      `json:"path"`
	IsDir     bool        `json:"is_dir"`
	Children  []*TreeNode `json:"children,omitempty"`
	FileCount int         `json:"file_count,omitempty"`
}

// BuildTree returns a cached directory tree, rescanning if older than treeCacheTTL.
func (v *Vault) BuildTree() (*TreeNode, error) {
	v.treeMu.RLock()
	if v.cachedTree != nil && time.Since(v.treeScannedAt) < treeCacheTTL {
		tree := v.cachedTree
		v.treeMu.RUnlock()
		return tree, nil
	}
	v.treeMu.RUnlock()

	v.treeMu.Lock()
	defer v.treeMu.Unlock()
	// Double-check after acquiring write lock
	if v.cachedTree != nil && time.Since(v.treeScannedAt) < treeCacheTTL {
		return v.cachedTree, nil
	}

	root := &TreeNode{
		Name:  filepath.Base(v.cfg.VaultPath),
		Path:  "",
		IsDir: true,
	}
	if err := v.buildTreeRecursive(v.cfg.VaultPath, "", root); err != nil {
		return nil, err
	}
	v.cachedTree = root
	v.treeScannedAt = time.Now()
	return root, nil
}

func (v *Vault) buildTreeRecursive(absPath, relPath string, node *TreeNode) error {
	entries, err := os.ReadDir(absPath)
	if err != nil {
		return err
	}
	for _, e := range entries {
		childRel := filepath.Join(relPath, e.Name())
		if v.IsExcluded(childRel) {
			continue
		}
		if !v.IsIncluded(childRel) && !e.IsDir() {
			continue
		}
		child := &TreeNode{
			Name:  e.Name(),
			Path:  childRel,
			IsDir: e.IsDir(),
		}
		if e.IsDir() {
			if err := v.buildTreeRecursive(filepath.Join(absPath, e.Name()), childRel, child); err != nil {
				continue // skip unreadable dirs
			}
			child.FileCount = countMdFiles(child)
			if child.FileCount == 0 && len(child.Children) == 0 {
				continue // skip empty dirs
			}
		} else if !strings.HasSuffix(e.Name(), ".md") {
			continue // only show .md files
		}
		node.Children = append(node.Children, child)
	}
	return nil
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func countMdFiles(node *TreeNode) int {
	count := 0
	for _, c := range node.Children {
		if c.IsDir {
			count += countMdFiles(c)
		} else {
			count++
		}
	}
	return count
}

// WalkMdFiles walks all .md files in the vault, calling fn for each.
func (v *Vault) WalkMdFiles(fn func(relPath string, info fs.FileInfo) error) error {
	return filepath.Walk(v.cfg.VaultPath, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return nil // skip errors
		}
		relPath, _ := filepath.Rel(v.cfg.VaultPath, path)
		if v.IsExcluded(relPath) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if info.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		if !v.IsIncluded(relPath) {
			return nil
		}
		return fn(relPath, info)
	})
}
