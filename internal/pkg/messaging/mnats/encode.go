package mnats

import (
	"encoding/json"
	"errors"
)

type QueueEncoder string

const (
	JsonEncoder QueueEncoder = "json"
)

func natsEncode[T any](encoder QueueEncoder, data T) ([]byte, error) {
	if encoder == JsonEncoder {
		return json.Marshal(data)
	}
	return nil, errors.New("unknown encoder")
}

func natsDecode[T any](encoder QueueEncoder, data []byte) (T, error) {
	var emptyRes T
	if len(data) == 0 {
		return emptyRes, nil
	}

	if encoder == JsonEncoder {
		var res T
		if err := json.Unmarshal(data, &res); err != nil {
			return res, err
		}

		return res, nil
	}

	return emptyRes, errors.New("unknown decoder")
}
