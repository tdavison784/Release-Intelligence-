package normalize

import (
	"regexp"
	"testing"
)

func TestExtractTableRowSkipsTablesWithoutValueColumns(t *testing.T) {
	md := []byte(`## LTS
| Release | Vendor | End of Life |
|---|---|---|
| 1.17 LTS | Acme | 2027 |

## Old releases
| Release | EOL | Compatible Kubernetes versions |
|---|---|---|
| [1.17][] | 2025 | 1.29 → 1.33 |
`)
	sel := TableSelector{KeyColumns: []string{"Release"}, KeyRe: regexp.MustCompile(`^1\.17(\s|$)`)}
	row, err := ExtractTableRow(md, sel)
	if err != nil || row.Line != 4 {
		t.Fatalf("without ValueColumns the first table matches: %+v %v", row, err)
	}
	sel.ValueColumns = []string{"Compatible Kubernetes versions"}
	row, err = ExtractTableRow(md, sel)
	if err != nil || row.Line != 9 || row.Cells["Compatible Kubernetes versions"] != "1.29 → 1.33" {
		t.Fatalf("with ValueColumns the compatibility table must match: %+v %v", row, err)
	}
}
