package catalog

import (
	"bytes"
	"fmt"
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
	products map[string]*ProductDefinition
}

// LoadDir loads every *.yaml / *.yml file in dir. It fails on duplicate ids.
func LoadDir(dir string) (*Catalog, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	c := &Catalog{products: map[string]*ProductDefinition{}}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if ext != ".yaml" && ext != ".yml" {
			continue
		}
		d, err := LoadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		if _, dup := c.products[d.ID]; dup {
			return nil, fmt.Errorf("duplicate product id %q in %s", d.ID, e.Name())
		}
		c.products[d.ID] = d
	}
	return c, nil
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
