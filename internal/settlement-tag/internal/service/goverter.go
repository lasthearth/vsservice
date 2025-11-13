package service

import "google.golang.org/protobuf/types/known/wrapperspb"

func FloatValueToFloat32(value *wrapperspb.FloatValue) float32 {
	return value.Value
}

func Float32ToFloatValue(value float32) *wrapperspb.FloatValue {
	return wrapperspb.Float(value)
}
