package stats

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Record is one docs/onboarding/records/<id>.yaml (format: docs/onboarding/
// PROTOCOL.md). Every field is optional; unknown fields are ignored and a
// field of the wrong type is dropped with a warning, so a half-written record
// never prevents the report from being produced.
type Record struct {
	Product    string `yaml:"product" json:"product"`
	Order      int    `yaml:"order" json:"order"`         // 0: unknown
	Wave       *int   `yaml:"wave" json:"wave,omitempty"` // nil: unknown
	Repository string `yaml:"repository" json:"repository,omitempty"`

	Onboarding struct {
		StartedAt  string   `yaml:"startedAt" json:"startedAt,omitempty"`
		FinishedAt string   `yaml:"finishedAt" json:"finishedAt,omitempty"`
		Minutes    *float64 `yaml:"minutes" json:"minutes,omitempty"` // nil: unknown
	} `yaml:"onboarding" json:"onboarding"`

	Discovery struct {
		Proposal string `yaml:"proposal" json:"proposal,omitempty"`
		Report   string `yaml:"report" json:"report,omitempty"`
	} `yaml:"discovery" json:"discovery"`

	// Sources and Artifacts list where each element of the final definition
	// came from.
	Sources           []OriginEntry `yaml:"sources" json:"sources,omitempty"`
	Artifacts         []OriginEntry `yaml:"artifacts" json:"artifacts,omitempty"`
	DiscoveryRejected []IDReason    `yaml:"discoveryRejected" json:"discoveryRejected,omitempty"`

	Relationships struct {
		HistoricalReleasesChecked []string `yaml:"historicalReleasesChecked" json:"historicalReleasesChecked,omitempty"`
		InitialFailures           *int     `yaml:"initialFailures" json:"initialFailures,omitempty"`
		FinalFailures             *int     `yaml:"finalFailures" json:"finalFailures,omitempty"`
		ManualInterventions       []struct {
			Subject string `yaml:"subject" json:"subject"`
			Change  string `yaml:"change" json:"change"`
			Reason  string `yaml:"reason" json:"reason"`
		} `yaml:"manualInterventions" json:"manualInterventions,omitempty"`
		Unverifiable []struct {
			Subject string `yaml:"subject" json:"subject"`
			Reason  string `yaml:"reason" json:"reason"`
		} `yaml:"unverifiable" json:"unverifiable,omitempty"`
	} `yaml:"relationships" json:"relationships"`

	Constructs struct {
		// New are the constructs this product introduced: the authoritative
		// input of the convergence curve.
		New        []ConstructRef `yaml:"new" json:"new,omitempty"`
		Considered []ConstructRef `yaml:"considered" json:"considered,omitempty"`
	} `yaml:"constructs" json:"constructs"`

	GoChanges struct {
		ProductSpecific int `yaml:"productSpecific" json:"productSpecific"`
		Generic         []struct {
			File    string `yaml:"file" json:"file"`
			Purpose string `yaml:"purpose" json:"purpose"`
		} `yaml:"generic" json:"generic,omitempty"`
	} `yaml:"goChanges" json:"goChanges"`

	UnreachableSources []struct {
		Host   string `yaml:"host" json:"host"`
		Reason string `yaml:"reason" json:"reason"`
	} `yaml:"unreachableSources" json:"unreachableSources,omitempty"`
	AmbiguousRelationships []Text `yaml:"ambiguousRelationships" json:"ambiguousRelationships,omitempty"`
	Representability       string `yaml:"representability" json:"representability,omitempty"` // full | partial | poor
	Gaps                   []Text `yaml:"gaps" json:"gaps,omitempty"`

	SampleEdge struct {
		From            string `yaml:"from" json:"from,omitempty"`
		To              string `yaml:"to" json:"to,omitempty"`
		Summary         string `yaml:"summary" json:"summary,omitempty"`
		EvidenceRecords int    `yaml:"evidenceRecords" json:"evidenceRecords,omitempty"`
	} `yaml:"sampleEdge" json:"sampleEdge"`
	Notes string `yaml:"notes" json:"notes,omitempty"`

	// File is the path the record was read from.
	File string `yaml:"-" json:"-"`
}

// Origins of a source or artifact in the final definition.
const (
	OriginDiscovered         = "discovered"
	OriginDiscoveredModified = "discovered-modified"
	OriginManual             = "manual"
)

// OriginEntry says where one source or artifact of the definition came from.
type OriginEntry struct {
	ID     string `yaml:"id" json:"id"`
	Origin string `yaml:"origin" json:"origin"`
	Note   string `yaml:"note" json:"note,omitempty"`
}

// IDReason is an id with a free-text reason.
type IDReason struct {
	ID     string `yaml:"id" json:"id"`
	Reason string `yaml:"reason" json:"reason"`
}

// ConstructRef names a construct. In a record it may be written as a plain
// string (the name) or as a mapping {name, kind, justification, otherProducts}.
type ConstructRef struct {
	Name          string   `yaml:"name" json:"name"`
	Kind          string   `yaml:"kind" json:"kind,omitempty"`
	Justification string   `yaml:"justification" json:"justification,omitempty"`
	OtherProducts []string `yaml:"otherProducts" json:"otherProducts,omitempty"`
}

// UnmarshalYAML accepts a scalar (the name) or a mapping.
func (c *ConstructRef) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.ScalarNode {
		c.Name = strings.TrimSpace(n.Value)
		return nil
	}
	type plain ConstructRef
	var p plain
	var aliases struct {
		Construct string `yaml:"construct"`
		ID        string `yaml:"id"`
	}
	var ignored []string
	decodeLoose(n, reflect.ValueOf(&p).Elem(), "", &ignored)
	decodeLoose(n, reflect.ValueOf(&aliases).Elem(), "", &ignored)
	*c = ConstructRef(p)
	if c.Name == "" {
		c.Name = firstNonEmpty(aliases.Construct, aliases.ID)
	}
	c.Name = strings.TrimSpace(c.Name)
	return nil
}

// Text is free text that a record may write as a string or as a mapping; a
// mapping is flattened to "key: value; key: value".
type Text string

// UnmarshalYAML accepts a scalar or a mapping of scalars.
func (t *Text) UnmarshalYAML(n *yaml.Node) error {
	switch n.Kind {
	case yaml.ScalarNode:
		*t = Text(strings.TrimSpace(n.Value))
	case yaml.MappingNode:
		var parts []string
		for i := 0; i+1 < len(n.Content); i += 2 {
			parts = append(parts, strings.TrimSpace(n.Content[i].Value)+": "+strings.TrimSpace(n.Content[i+1].Value))
		}
		*t = Text(strings.Join(parts, "; "))
	default:
		return fmt.Errorf("line %d: expected text", n.Line)
	}
	return nil
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}

// ParseRecord decodes a record tolerantly. The returned warnings describe the
// parts that were ignored (wrong type, not a mapping); the error is non-nil
// only when data is not YAML at all.
func ParseRecord(data []byte) (*Record, []string, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, nil, err
	}
	var r Record
	var warns []string
	if doc.Kind == 0 { // empty document
		return &r, nil, nil
	}
	root := &doc
	if root.Kind == yaml.DocumentNode && len(root.Content) > 0 {
		root = root.Content[0]
	}
	decodeLoose(root, reflect.ValueOf(&r).Elem(), "", &warns)
	return &r, warns, nil
}

var unmarshalerTyp = reflect.TypeOf((*yaml.Unmarshaler)(nil)).Elem()

// decodeLoose decodes n into v, dropping (with a warning) whatever does not
// fit instead of failing the whole document. It reports whether a value was
// stored (a mapping with some ignored members still counts).
func decodeLoose(n *yaml.Node, v reflect.Value, path string, warns *[]string) bool {
	for n.Kind == yaml.AliasNode && n.Alias != nil {
		n = n.Alias
	}
	warn := func(format string, a ...any) bool {
		where := path
		if where == "" {
			where = "record"
		}
		*warns = append(*warns, fmt.Sprintf("%s (line %d): %s", where, n.Line, fmt.Sprintf(format, a...)))
		return false
	}
	if n.Kind == yaml.ScalarNode && n.Tag == "!!null" {
		return false // null: leave the zero value
	}
	// custom unmarshalers decode themselves
	if v.CanAddr() && v.Addr().Type().Implements(unmarshalerTyp) {
		if err := n.Decode(v.Addr().Interface()); err != nil {
			return warn("ignored: %v", err)
		}
		return true
	}
	scalar := func(what string) (string, bool) {
		if n.Kind != yaml.ScalarNode {
			warn("expected %s, ignored", what)
			return "", false
		}
		return strings.TrimSpace(n.Value), true
	}
	switch v.Kind() {
	case reflect.Ptr:
		elem := reflect.New(v.Type().Elem())
		if !decodeLoose(n, elem.Elem(), path, warns) {
			return false
		}
		v.Set(elem)
	case reflect.Struct:
		if n.Kind != yaml.MappingNode {
			return warn("expected a mapping, ignored")
		}
		fields := yamlFields(v.Type())
		for i := 0; i+1 < len(n.Content); i += 2 {
			key := n.Content[i].Value
			idx, ok := fields[key]
			if !ok {
				continue // extra field: tolerated
			}
			decodeLoose(n.Content[i+1], v.Field(idx), joinPath(path, key), warns)
		}
	case reflect.Slice:
		switch n.Kind {
		case yaml.SequenceNode:
			out := reflect.MakeSlice(v.Type(), 0, len(n.Content))
			for i, c := range n.Content {
				el := reflect.New(v.Type().Elem()).Elem()
				if decodeLoose(c, el, fmt.Sprintf("%s[%d]", path, i), warns) {
					out = reflect.Append(out, el)
				}
			}
			v.Set(out)
		case yaml.ScalarNode:
			if strings.TrimSpace(n.Value) == "" {
				return false
			}
			// a single value where a list was expected
			el := reflect.New(v.Type().Elem()).Elem()
			if !decodeLoose(n, el, path+"[0]", warns) {
				return false
			}
			v.Set(reflect.Append(reflect.MakeSlice(v.Type(), 0, 1), el))
		default:
			return warn("expected a list, ignored")
		}
	case reflect.String:
		if n.Kind != yaml.ScalarNode {
			return warn("expected a string, ignored")
		}
		v.SetString(n.Value)
	case reflect.Int, reflect.Int64:
		str, ok := scalar("an integer")
		if !ok {
			return false
		}
		i, err := strconv.ParseInt(str, 10, 64)
		if err != nil {
			f, ferr := strconv.ParseFloat(str, 64)
			if ferr != nil {
				return warn("expected an integer, got %q; ignored", n.Value)
			}
			i = int64(f)
		}
		v.SetInt(i)
	case reflect.Float64:
		str, ok := scalar("a number")
		if !ok {
			return false
		}
		f, err := strconv.ParseFloat(str, 64)
		if err != nil {
			return warn("expected a number, got %q; ignored", n.Value)
		}
		v.SetFloat(f)
	case reflect.Bool:
		str, ok := scalar("a boolean")
		if !ok {
			return false
		}
		b, err := strconv.ParseBool(str)
		if err != nil {
			return warn("expected a boolean, got %q; ignored", n.Value)
		}
		v.SetBool(b)
	default:
		return warn("unsupported field type %s", v.Type())
	}
	return true
}

func joinPath(path, key string) string {
	if path == "" {
		return key
	}
	return path + "." + key
}

// yamlFields maps yaml keys to field indexes of a struct type.
func yamlFields(t reflect.Type) map[string]int {
	m := map[string]int{}
	for i := 0; i < t.NumField(); i++ {
		if name := yamlName(t.Field(i)); name != "" {
			m[name] = i
		}
	}
	return m
}

// LoadRecords reads every *.yaml / *.yml file in dir. A file that cannot be
// parsed is skipped with a warning; warnings from tolerant decoding are
// prefixed with the file name. A missing directory yields no records.
func LoadRecords(dir string) ([]*Record, []string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, nil
		}
		return nil, nil, err
	}
	var recs []*Record
	var warns []string
	for _, e := range entries {
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if e.IsDir() || (ext != ".yaml" && ext != ".yml") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			warns = append(warns, fmt.Sprintf("%s: %v", path, err))
			continue
		}
		r, w, err := ParseRecord(data)
		if err != nil {
			warns = append(warns, fmt.Sprintf("%s: not YAML, skipped: %v", path, err))
			continue
		}
		for _, m := range w {
			warns = append(warns, fmt.Sprintf("%s: %s", path, m))
		}
		r.File = path
		if r.Product == "" {
			r.Product = strings.TrimSuffix(e.Name(), ext)
			warns = append(warns, fmt.Sprintf("%s: no product field, using %q from the file name", path, r.Product))
		}
		recs = append(recs, r)
	}
	sort.SliceStable(recs, func(i, j int) bool { return recs[i].Product < recs[j].Product })
	return recs, warns, nil
}
