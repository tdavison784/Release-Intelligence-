package bundle

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Redaction (DESIGN.md §2.3), applied at collection, before hashing:
//
//   - Kubernetes Secret data/stringData values become "<redacted sha256:…>"
//     (keys kept);
//   - values whose key's last path segment names a credential (password,
//     token, apikey, privatekey, …) likewise.
//
// A document that needs no redaction is returned byte for byte; only a
// changed document is re-encoded, so ordinary files keep their text.

var sensitiveKey = regexp.MustCompile(`(?i)password|passwd|secret|token|apikey|api_key|privatekey|private_key|credentials?`)

func redactedValue(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return "<redacted sha256:" + hex.EncodeToString(sum[:])[:12] + ">"
}

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
		if redactNode(root.Content[0], "", isSecretDoc(root.Content[0])) {
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

// redactNode walks n; key is the last mapping key on the way down. In a
// Secret document everything below data/stringData is redacted. It reports
// whether it changed anything.
func redactNode(n *yaml.Node, key string, secretDoc bool) bool {
	changed := false
	switch n.Kind {
	case yaml.MappingNode:
		for i := 0; i+1 < len(n.Content); i += 2 {
			k, v := n.Content[i].Value, n.Content[i+1]
			switch {
			case secretDoc && (k == "data" || k == "stringData") && v.Kind == yaml.MappingNode:
				changed = redactAll(v) || changed
			case sensitiveKey.MatchString(k) && !isBoolLike(v):
				changed = redactAll(v) || changed
			default:
				changed = redactNode(v, k, secretDoc) || changed
			}
		}
	case yaml.SequenceNode:
		for _, c := range n.Content {
			changed = redactNode(c, key, secretDoc) || changed
		}
	}
	return changed
}

// isBoolLike keeps flags such as `createSecret: true` readable: a secret is
// never a boolean.
func isBoolLike(v *yaml.Node) bool {
	return v.Kind == yaml.ScalarNode && (v.Tag == "!!bool" || v.Tag == "!!null")
}

// redactAll replaces every scalar below n.
func redactAll(n *yaml.Node) bool {
	changed := false
	switch n.Kind {
	case yaml.ScalarNode:
		if n.Value != "" && !strings.HasPrefix(n.Value, "<redacted sha256:") {
			n.Value, n.Tag, n.Style = redactedValue(n.Value), "!!str", yaml.DoubleQuotedStyle
			changed = true
		}
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
