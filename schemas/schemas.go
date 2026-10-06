// Package schemas embeds the hand-written (non-generated) JSON schemas that
// the binary itself needs at runtime.
package schemas

import _ "embed"

// Policy is schemas/policy.schema.json: the tiered upgrade policy file.
//
//go:embed policy.schema.json
var Policy []byte
