package orchestrate

import (
	"encoding/json"
	"strconv"
)

func itoa(n int) string { return strconv.Itoa(n) }

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}
