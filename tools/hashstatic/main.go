// tools/hashstatic: reads internal/server/static/{css,js}/** and writes
// content-hashed copies into internal/server/static/dist/ along with a
// manifest.json mapping logical paths (e.g. "js/views/chat.js") to their
// hashed counterparts (e.g. "js/views/chat.abc12345.js").
//
// dashboard.html and sw.js are copied verbatim (fixed URLs; SW registration
// would break under a hashed name, and dashboard.html is rendered by Go
// templates at serve time).
//
// Usage: go run ./tools/hashstatic
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const (
	srcRoot  = "internal/server/static"
	distRoot = "internal/server/static/dist"
)

// verbatimNames are top-level filenames copied into dist/ without hashing.
// dashboard.html stays at a fixed path because Go templates render it at
// serve time. sw.js stays at a fixed path because service workers are fetched
// from a stable URL (/sw.js) — hashing would break SW registration.
//
// The PWA manifest.json is intentionally NOT copied: the asset manifest we
// write at dist/manifest.json must not be shadowed.
var verbatimNames = map[string]bool{
	"dashboard.html": true,
	"sw.js":          true,
}

func main() {
	if err := os.RemoveAll(distRoot); err != nil {
		fatal(err)
	}
	if err := os.MkdirAll(distRoot, 0o755); err != nil {
		fatal(err)
	}

	manifest := map[string]string{}
	hashed := 0
	copied := 0

	err := filepath.WalkDir(srcRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path == distRoot {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(srcRoot, path)
		rel = filepath.ToSlash(rel)
		if strings.HasPrefix(rel, "dist/") {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		// Verbatim copy for fixed-URL artifacts.
		if verbatimNames[rel] {
			target := filepath.Join(distRoot, rel)
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(target, data, 0o644); err != nil {
				return err
			}
			copied++
			return nil
		}

		ext := strings.ToLower(filepath.Ext(rel))
		if ext != ".css" && ext != ".js" {
			return nil
		}

		h := sha256.Sum256(data)
		hash := hex.EncodeToString(h[:4]) // first 8 hex chars
		base := strings.TrimSuffix(rel, ext)
		hashedRel := fmt.Sprintf("%s.%s%s", base, hash, ext)
		target := filepath.Join(distRoot, filepath.FromSlash(hashedRel))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(target, data, 0o644); err != nil {
			return err
		}
		manifest[rel] = hashedRel
		hashed++
		return nil
	})
	if err != nil {
		fatal(err)
	}

	manPath := filepath.Join(distRoot, "manifest.json")
	// Write a stable (sorted) manifest for reproducible builds. Go's json
	// package already marshals maps with sorted keys, so MarshalIndent is fine.
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		fatal(err)
	}
	if err := os.WriteFile(manPath, data, 0o644); err != nil {
		fatal(err)
	}
	fmt.Printf("hashstatic: hashed %d css/js files, copied %d verbatim, wrote %s\n",
		hashed, copied, manPath)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
