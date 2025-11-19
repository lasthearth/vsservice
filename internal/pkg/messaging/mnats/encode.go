package mnats

import (
	"encoding/json"
	"fmt"
)

type QueueEncoder string

const (
	JsonEncoder QueueEncoder = "json"
)

func natsEncode[T any](encoder QueueEncoder, data T) ([]byte, error) {
	switch encoder {
	case JsonEncoder:
		return json.Marshal(data)
	}
	return nil, fmt.Errorf("unknown encoder")
}

func natsDecode[T any](encoder QueueEncoder, data []byte) (T, error) {
	var emptyRes T
	if len(data) == 0 {
		return emptyRes, nil
	}

	switch encoder {
	case JsonEncoder:
		var res T
		if err := json.Unmarshal(data, &res); err != nil {
			return res, err
		}

		return res, nil
	}

	return emptyRes, fmt.Errorf("unknown decoder")
}
