package orchestrate_test

import (
	"go.temporal.io/api/history/v1"
	"go.temporal.io/sdk/client"
	"google.golang.org/protobuf/encoding/protojson"
)

func historyJSON(iter client.HistoryEventIterator) ([]byte, error) {
	h := &history.History{}
	for iter.HasNext() {
		e, err := iter.Next()
		if err != nil {
			return nil, err
		}
		h.Events = append(h.Events, e)
	}
	return protojson.MarshalOptions{Multiline: true}.Marshal(h)
}
