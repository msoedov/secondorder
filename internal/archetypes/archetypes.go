package archetypes

import (
	"bufio"
	"embed"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

//go:embed *.md agency
var archetypesFS embed.FS

var overridesDir = "archetypes"

// CatalogEntry is the metadata used by the CEO agent and the /api/archetypes endpoint.
type CatalogEntry struct {
	Slug        string `json:"slug"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Division    string `json:"division"`
	Source      string `json:"source"` // "builtin" or "agency"
}

func SetOverridesDir(dir string) {
	overridesDir = dir
}

func GetOverridesDir() string {
	return overridesDir
}

// slugToPath maps a slug like "agency/design/ui-designer" or "ceo" to a file path.
func slugToPath(slug string) string {
	return slug + ".md"
}

func Read(slug string) ([]byte, error) {
	filename := slugToPath(slug)
	if data, err := os.ReadFile(filepath.Join(overridesDir, filename)); err == nil {
		return data, nil
	}
	return archetypesFS.ReadFile(filename)
}

// List returns every known slug — builtins, embedded agency personas, and overrides.
func List() ([]string, error) {
	slugs := make(map[string]bool)

	_ = fs.WalkDir(archetypesFS, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".md") {
			return nil
		}
		slugs[strings.TrimSuffix(p, ".md")] = true
		return nil
	})

	if _, err := os.Stat(overridesDir); err == nil {
		_ = filepath.WalkDir(overridesDir, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(p, ".md") {
				return nil
			}
			rel, err := filepath.Rel(overridesDir, p)
			if err != nil {
				return nil
			}
			slugs[filepath.ToSlash(strings.TrimSuffix(rel, ".md"))] = true
			return nil
		})
	}

	out := make([]string, 0, len(slugs))
	for s := range slugs {
		out = append(out, s)
	}
	sort.Strings(out)
	return out, nil
}

func Exists(slug string) bool {
	filename := slugToPath(slug)
	if _, err := os.Stat(filepath.Join(overridesDir, filename)); err == nil {
		return true
	}
	_, err := archetypesFS.Open(filename)
	return err == nil
}

// Get returns the archetype content for the given slug, or an error if not found.
func Get(slug string) (string, error) {
	data, err := Read(slug)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// WriteToTemp writes the archetype content to a temporary file and returns its path.
func WriteToTemp(slug string) (string, func(), error) {
	data, err := Read(slug)
	if err != nil {
		return "", nil, err
	}
	// Sanitize slug for tmp filename (avoid path separators).
	safe := strings.ReplaceAll(slug, "/", "-")
	tmpFile, err := os.CreateTemp("", safe+"-*.md")
	if err != nil {
		return "", nil, err
	}
	if _, err := tmpFile.Write(data); err != nil {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
		return "", nil, err
	}
	tmpFile.Close()
	cleanup := func() { os.Remove(tmpFile.Name()) }
	return tmpFile.Name(), cleanup, nil
}

// Catalog returns metadata for every archetype known to the system.
// Used by the CEO agent to discover which specialists it can hire.
func Catalog() ([]CatalogEntry, error) {
	slugs, err := List()
	if err != nil {
		return nil, err
	}
	out := make([]CatalogEntry, 0, len(slugs))
	for _, slug := range slugs {
		data, err := Read(slug)
		if err != nil {
			continue
		}
		entry := CatalogEntry{Slug: slug}
		if strings.Contains(slug, "/") {
			parts := strings.SplitN(slug, "/", 3)
			// agency/design/foo -> division "design"
			if len(parts) >= 2 {
				entry.Division = parts[1]
			}
			entry.Source = "agency"
		} else {
			entry.Division = "core"
			entry.Source = "builtin"
		}
		entry.Title, entry.Description = extractTitleAndDescription(data)
		out = append(out, entry)
	}
	return out, nil
}

// extractTitleAndDescription pulls the first H1 and the first non-empty paragraph
// after it. Works for both builtin terse files and trimmed agency personas.
func extractTitleAndDescription(data []byte) (title, description string) {
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var desc strings.Builder
	seenTitle := false
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), " \t")
		if !seenTitle {
			if strings.HasPrefix(line, "# ") {
				title = strings.TrimSpace(strings.TrimPrefix(line, "# "))
				seenTitle = true
			}
			continue
		}
		if line == "" {
			if desc.Len() > 0 {
				break
			}
			continue
		}
		if strings.HasPrefix(line, "#") {
			if desc.Len() > 0 {
				break
			}
			continue
		}
		if desc.Len() > 0 {
			desc.WriteByte(' ')
		}
		// Strip simple markdown emphasis wrapping.
		line = strings.Trim(line, "*_")
		desc.WriteString(line)
		if desc.Len() > 400 {
			break
		}
	}
	description = desc.String()
	if len(description) > 400 {
		description = description[:397] + "..."
	}
	return title, description
}
