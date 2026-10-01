package catalog

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// Parse decodes a definition from YAML bytes. Unknown fields are rejected so
// typos in checked-in definitions surface immediately.
func Parse(data []byte) (*ProductDefinition, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	var d ProductDefinition
	if err := dec.Decode(&d); err != nil {
		return nil, fmt.Errorf("decode product definition: %w", err)
	}
	d.digest = domain.Digest(data)
	return &d, nil
}

// LoadFile loads a definition from a YAML file.
func LoadFile(path string) (*ProductDefinition, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	d, err := Parse(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	d.path = path
	return d, nil
}

// Catalog is a set of product definitions keyed by id.
type Catalog struct {
	products   map[string]*ProductDefinition
	loadErrors map[string]error // file path → why it was not loaded
}

// LoadDir loads every *.yaml / *.yml file in dir. A file that cannot be read
// or decoded, or whose product id is already defined by an earlier file (files
// are visited in name order), is not loaded and recorded in LoadErrors; the
// remaining files are loaded regardless. It returns an error only when dir
// itself cannot be read.
func LoadDir(dir string) (*Catalog, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	c := &Catalog{products: map[string]*ProductDefinition{}, loadErrors: map[string]error{}}
	paths := map[string]string{} // product id → file that defined it
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if ext != ".yaml" && ext != ".yml" {
			continue
		}
		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			var pe *fs.PathError
			if errors.As(err, &pe) {
				err = pe.Err // the path is the key
			}
			c.loadErrors[path] = err
			continue
		}
		d, err := Parse(data)
		if err != nil {
			c.loadErrors[path] = err
			continue
		}
		d.path = path
		if first, dup := paths[d.ID]; dup {
			c.loadErrors[path] = fmt.Errorf("duplicate product id %q (already defined in %s)", d.ID, first)
			continue
		}
		paths[d.ID] = path
		c.products[d.ID] = d
	}
	return c, nil
}

// LoadErrors returns the files LoadDir could not load, keyed by file path.
// The map is empty when every file loaded.
func (c *Catalog) LoadErrors() map[string]error {
	out := make(map[string]error, len(c.loadErrors))
	for p, err := range c.loadErrors {
		out[p] = err
	}
	return out
}

// LoadErrorPaths returns the keys of LoadErrors in sorted order.
func (c *Catalog) LoadErrorPaths() []string {
	out := make([]string, 0, len(c.loadErrors))
	for p := range c.loadErrors {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// New builds a catalog from in-memory definitions.
func New(defs ...*ProductDefinition) *Catalog {
	c := &Catalog{products: map[string]*ProductDefinition{}}
	for _, d := range defs {
		c.products[d.ID] = d
	}
	return c
}

// Get returns the definition for id.
func (c *Catalog) Get(id string) (*ProductDefinition, bool) {
	d, ok := c.products[id]
	return d, ok
}

// List returns all definitions sorted by id.
func (c *Catalog) List() []*ProductDefinition {
	out := make([]*ProductDefinition, 0, len(c.products))
	for _, d := range c.products {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Marshal encodes a definition as YAML.
func Marshal(d *ProductDefinition) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(d); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
