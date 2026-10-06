// Package policies embeds the shipped tiered upgrade policies.
package policies

import _ "embed"

// Default is policies/default.yaml: the conservative shipped policy.
//
//go:embed default.yaml
var Default []byte
