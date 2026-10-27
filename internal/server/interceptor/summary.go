package interceptor

import (
	"strings"

	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// summaryRequestFields are the scalar request fields safe to record: paging
// and filters, never content. Identity fields are matched separately.
var summaryRequestFields = map[string]struct{}{
	"page":      {},
	"limit":     {},
	"offset":    {},
	"page_size": {},
	"kind":      {},
	"count":     {},
}

// requestSummary records the identity and paging fields of a request, never a
// body. A record must say which entity the call touched without carrying the
// payload.
func requestSummary(req any) []zap.Field {
	m, ok := reflectMessage(req)
	if !ok {
		return nil
	}

	fields := m.Descriptor().Fields()
	out := make([]zap.Field, 0, 2)
	for i := 0; i < fields.Len() && len(out) < maxSummaryFields; i++ {
		fd := fields.Get(i)
		if fd.IsList() || fd.IsMap() || fd.Message() != nil {
			continue
		}
		if !isIdentityField(string(fd.Name())) {
			continue
		}
		if f, ok := scalarField(m, fd); ok {
			out = append(out, f)
		}
	}
	return out
}

// responseSummary records how much the client received: the first repeated
// field with its length, plus the id of a single-entity response.
func responseSummary(resp any) []zap.Field {
	m, ok := reflectMessage(resp)
	if !ok {
		return nil
	}

	fields := m.Descriptor().Fields()
	out := make([]zap.Field, 0, 3)
	for i := 0; i < fields.Len(); i++ {
		fd := fields.Get(i)
		if !fd.IsList() {
			continue
		}
		out = append(out,
			zap.String("resp_list", string(fd.Name())),
			zap.Int("resp_count", m.Get(fd).List().Len()),
		)
		break
	}

	if fd := fields.ByName("id"); fd != nil && !fd.IsList() && fd.Message() == nil {
		if f, ok := scalarField(m, fd); ok {
			out = append(out, f)
		}
	}
	return out
}

func reflectMessage(v any) (protoreflect.Message, bool) {
	msg, ok := v.(proto.Message)
	if !ok || msg == nil {
		return nil, false
	}
	m := msg.ProtoReflect()
	if m == nil || !m.IsValid() {
		return nil, false
	}
	return m, true
}

func isIdentityField(name string) bool {
	if name == "id" || strings.HasSuffix(name, "_id") {
		return true
	}
	_, ok := summaryRequestFields[name]
	return ok
}

// scalarField renders one scalar proto field. Unset fields are skipped, so a
// record carries no zero-value noise.
func scalarField(m protoreflect.Message, fd protoreflect.FieldDescriptor) (zap.Field, bool) {
	if !m.Has(fd) {
		return zap.Field{}, false
	}

	v := m.Get(fd)
	key := string(fd.Name())

	switch fd.Kind() {
	case protoreflect.StringKind:
		return zap.String(key, v.String()), true
	case protoreflect.BoolKind:
		return zap.Bool(key, v.Bool()), true
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		return zap.Int32(key, int32(v.Int())), true
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		return zap.Int64(key, v.Int()), true
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		return zap.Uint32(key, uint32(v.Uint())), true
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		return zap.Uint64(key, v.Uint()), true
	case protoreflect.EnumKind:
		return zap.String(key, enumName(fd, v)), true
	default:
		return zap.Field{}, false
	}
}

func enumName(fd protoreflect.FieldDescriptor, v protoreflect.Value) string {
	ed := fd.Enum()
	if ed == nil {
		return ""
	}
	ev := ed.Values().ByNumber(v.Enum())
	if ev == nil {
		return ""
	}
	return string(ev.Name())
}
