package bundle

import (
	"bytes"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/tdavison784/release-intelligence/internal/investigate/redaction"
)

// Redaction (DESIGN.md §2.3), applied at collection, before hashing, with the
// policy of package redaction:
//
//   - Kubernetes Secret data/stringData values become "<redacted sha256:…>"
//     (keys kept);
//   - values whose own key names a credential (password, token, apikey,
//     privatekey, …) likewise, and any value holding a private key;
//   - below a credential-named key holding a mapping, only booleans, nulls
//     and numbers stay readable (redaction@v2; v1 withheld the whole
//     mapping, so `privateKey: {size: 256}` lost its size).
//
// A document that needs no redaction is returned byte for byte; only a
// changed document is re-encoded, so ordinary files keep their text.

var sensitiveKey = redaction.SensitiveKey

func redactedValue(raw string) string { return redaction.Marker(raw) }

// redactable reports whether the file extension is one redaction understands.
func redactable(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".yaml", ".yml", ".json":
		return true
	}
	return false
}

// redactYAML redacts a YAML (or JSON) stream document by document. It
// reports whether anything was redacted. When a document does not parse it
// is kept verbatim (such a file is also flagged by the environment loader).
func redactYAML(in []byte) (out []byte, redacted bool) {
	docs := splitDocs(in)
	var buf bytes.Buffer
	for i, d := range docs {
		var root yaml.Node
		if err := yaml.Unmarshal(d, &root); err != nil || len(root.Content) == 0 {
			buf.Write(d)
			continue
		}
		if redactNode(root.Content[0], "", false, isSecretDoc(root.Content[0])) {
			var enc bytes.Buffer
			e := yaml.NewEncoder(&enc)
			e.SetIndent(2)
			if err := e.Encode(&root); err == nil && e.Close() == nil {
				d = enc.Bytes()
				redacted = true
			}
		}
		buf.Write(d)
		if i < len(docs)-1 && !bytes.HasSuffix(d, []byte("\n")) {
			buf.WriteByte('\n')
		}
	}
	if !redacted {
		return in, false
	}
	return buf.Bytes(), true
}

// splitDocs splits a stream on document separator lines, keeping each
// separator with the document that follows it, so unchanged documents
// concatenate back to the exact input.
func splitDocs(in []byte) [][]byte {
	var docs [][]byte
	start := 0
	for pos := 0; pos < len(in); {
		end := bytes.IndexByte(in[pos:], '\n')
		var line []byte
		next := len(in)
		if end >= 0 {
			line, next = in[pos:pos+end], pos+end+1
		} else {
			line = in[pos:]
		}
		if (bytes.HasPrefix(line, []byte("---")) && len(bytes.TrimSpace(line[3:])) == 0) && pos > start {
			docs = append(docs, in[start:pos])
			start = pos
		}
		pos = next
	}
	if start < len(in) {
		docs = append(docs, in[start:])
	}
	return docs
}

func isSecretDoc(n *yaml.Node) bool {
	if n.Kind != yaml.MappingNode {
		return false
	}
	var kind, api string
	for i := 0; i+1 < len(n.Content); i += 2 {
		switch n.Content[i].Value {
		case "kind":
			kind = n.Content[i+1].Value
		case "apiVersion":
			api = n.Content[i+1].Value
		}
	}
	return kind == "Secret" && (api == "v1" || api == "")
}

// redactNode walks n; key is the nearest mapping key on the way down and
// under reports that a key further up names a credential and holds a
// mapping. In a Secret document everything below data/stringData is
// redacted. It reports whether it changed anything.
func redactNode(n *yaml.Node, key string, under, secretDoc bool) bool {
	changed := false
	switch n.Kind {
	case yaml.MappingNode:
		for i := 0; i+1 < len(n.Content); i += 2 {
			k, v := n.Content[i].Value, n.Content[i+1]
			switch {
			case secretDoc && (k == "data" || k == "stringData") && v.Kind == yaml.MappingNode:
				changed = redactAll(v) || changed
			case sensitiveKey.MatchString(k) && v.Kind == yaml.MappingNode:
				changed = redactNode(v, k, true, secretDoc) || changed
			case sensitiveKey.MatchString(k) && v.Kind != yaml.ScalarNode:
				changed = redactAll(v) || changed // a sequence (or alias): withheld whole, as before
			default:
				changed = redactNode(v, k, under, secretDoc) || changed
			}
		}
	case yaml.SequenceNode:
		for _, c := range n.Content {
			changed = redactNode(c, key, under, secretDoc) || changed
		}
	case yaml.ScalarNode:
		if !redaction.Keep(key, redaction.KindOfTag(n.ShortTag()), n.Value, under) {
			changed = redactScalar(n) || changed
		}
	}
	return changed
}

// redactAll replaces every scalar below n.
func redactAll(n *yaml.Node) bool {
	changed := false
	switch n.Kind {
	case yaml.ScalarNode:
		changed = redactScalar(n)
	case yaml.MappingNode:
		for i := 1; i < len(n.Content); i += 2 {
			changed = redactAll(n.Content[i]) || changed
		}
	case yaml.SequenceNode:
		for _, c := range n.Content {
			changed = redactAll(c) || changed
		}
	}
	return changed
}

func redactScalar(n *yaml.Node) bool {
	if n.Value == "" || redaction.IsMarker(n.Value) {
		return false
	}
	n.Value, n.Tag, n.Style = redactedValue(n.Value), "!!str", yaml.DoubleQuotedStyle
	return true
}
