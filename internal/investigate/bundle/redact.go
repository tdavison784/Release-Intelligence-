package bundle

import (
	"bytes"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/tdavison784/release-intelligence/internal/redaction"
)

// Redaction (DESIGN.md §2.3), applied at collection, before hashing, with the
// rule of package redaction (shared with the render model):
//
//   - Kubernetes Secret data/stringData values become "<redacted sha256:…>"
//     (keys kept);
//   - values whose own key names a credential (password, token, apikey,
//     privatekey, accessKey, …) likewise, unless the key is an object
//     reference's (secretName, …Ref.name, …Ref.key); any value holding a
//     private key; the value of a credential-named environment variable;
//   - below a credential-named key holding a mapping or list, only booleans,
//     nulls, numbers and reference keys stay readable (v1 withheld the whole
//     mapping, so `privateKey: {size: 256}` lost its size);
//   - a CustomResourceDefinition is a schema: only private-key content is
//     withheld in it.
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
		if secret, crd := docKind(root.Content[0]); redactNode(root.Content[0], "", false, secret, crd) {
			var enc bytes.Buffer
			if nl := bytes.IndexByte(d, '\n'); nl >= 0 && isSeparator(d[:nl]) {
				enc.Write(d[:nl+1]) // v1 dropped it, merging the document into the one before
			}
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
		if isSeparator(line) && pos > start {
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

// isSeparator reports whether line is a document separator line.
func isSeparator(line []byte) bool {
	return bytes.HasPrefix(line, []byte("---")) && len(bytes.TrimSpace(line[3:])) == 0
}

// docKind reports whether a document is a core Secret or a
// CustomResourceDefinition.
func docKind(n *yaml.Node) (secret, crd bool) {
	if n.Kind != yaml.MappingNode {
		return false, false
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
	return kind == "Secret" && (api == "v1" || api == ""), redaction.CRDDocument(api, kind)
}

// redactNode walks n; key is the nearest mapping key on the way down and
// under reports that a key further up names a credential and holds a
// mapping or list. In a Secret document everything below data/stringData is
// redacted; in a CRD only private keys are. It reports whether it changed
// anything.
func redactNode(n *yaml.Node, key string, under, secretDoc, crd bool) bool {
	changed := false
	switch n.Kind {
	case yaml.MappingNode:
		envName, hasName := "", false
		for i := 0; i+1 < len(n.Content); i += 2 {
			if n.Content[i].Value == "name" {
				hasName = true
				if n.Content[i+1].Kind == yaml.ScalarNode {
					envName = n.Content[i+1].Value
				}
			}
		}
		for i := 0; i+1 < len(n.Content); i += 2 {
			k, v := n.Content[i].Value, n.Content[i+1]
			switch {
			case secretDoc && (k == "data" || k == "stringData") && v.Kind == yaml.MappingNode:
				changed = redactAll(v) || changed
			case !crd && k == "value" && v.Kind == yaml.ScalarNode && envName != "" && redaction.CredentialEnvVar(envName):
				changed = redactScalar(v) || changed
			case !crd && (v.Kind == yaml.MappingNode || v.Kind == yaml.SequenceNode) && redaction.CredentialContainer(k):
				changed = redactNode(v, k, true, secretDoc, crd) || changed
			case v.Kind == yaml.ScalarNode:
				changed = redactLeaf(v, k, under, crd, hasName) || changed
			default:
				changed = redactNode(v, k, under, secretDoc, crd) || changed
			}
		}
	case yaml.SequenceNode:
		for _, c := range n.Content {
			changed = redactNode(c, key, under, secretDoc, crd) || changed
		}
	case yaml.ScalarNode:
		changed = redactLeaf(n, key, under, crd, false) // a list item
	}
	return changed
}

// redactLeaf applies the rule to one scalar; nameSibling: its mapping has a
// "name" key.
func redactLeaf(n *yaml.Node, key string, under, crd, nameSibling bool) bool {
	kind := redaction.KindOfTag(n.ShortTag())
	switch {
	case crd:
		if kind == redaction.String && redaction.HoldsPrivateKey(n.Value) {
			return redactScalar(n)
		}
	case !redaction.Keep(key, kind, n.Value, under, nameSibling):
		return redactScalar(n)
	}
	return false
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
