package xlog

import "log/slog"

var (
	// AttrsKey is the key under which a record's attributes are mapped when a record is converted to a string map.
	AttrsKey = "attrs"

	// FileKey is the key under which the file associated with a record's caller is mapped when a record is converted
	// to a string map.
	FileKey = "file"

	// FunctionKey is the key under which the function associated with a record's caller is mapped when a record is
	// converted to a string map.
	FunctionKey = "function"

	// LevelKey is the key under which a record's level is mapped when a record is converted to a string map.
	LevelKey = slog.LevelKey

	// LineKey is the key under which the line associated with a record's caller is mapped when a record is converted
	// to a string map.
	LineKey = "line"

	// MessageKey is the key under which a record's message is mapped when a record is converted to a string map.
	MessageKey = slog.MessageKey

	// SourceKey is the key under which a record's caller information is mapped when a record is converted to a
	// string map.
	SourceKey = slog.SourceKey

	// TimeKey is the key under which a record's timestamp is mapped when a record is converted to a string map.
	TimeKey = slog.TimeKey
)

// RecordToMap converts an entire [slog.Record] into a map[string]any.
//
// The map includes the record's time, level, message and any and all user-provided attributes, with support for
// nested groups.
//
// Record fields are mapped as follows:
//   - timestamp is mapped to the value of the package's [TimeKey] (default: [slog.TimeKey] value)
//   - level is mapped to the value of the package's [LevelKey] (default: [slog.LevelKey] value)
//   - message is mapped to the value of the package's [MessageKey] (default: [slog.MessageKey] value)
//   - attributes are mapped to the value of the package's [AttrsKey] (default: "attrs")
//
// If the record contains source (caller) information, the caller information is mapped to the field defined by
// the package's [SourceKey] (default: [slog.SourceKey]) with the fields [FileKey], [LineKey] and [FunctionKey]
// mapped to the caller's file, line and function, respectively.
func RecordToMap(r *slog.Record) map[string]any {
	if r == nil {
		return nil
	}

	// create the map, starting with the built-in fields - we can pre-allocate a reasonable size
	m := make(map[string]any, 4)

	// add the built-in fields
	m[TimeKey] = r.Time
	m[LevelKey] = r.Level.String()
	m[MessageKey] = r.Message
	src := r.Source()
	if src != nil {
		m[SourceKey] = map[string]any{
			FileKey:     src.File,
			LineKey:     src.Line,
			FunctionKey: src.Function,
		}
	}
	// iterate over all attributes in the record
	attrs := make(map[string]any, r.NumAttrs())
	r.Attrs(func(a slog.Attr) bool {
		attrs[a.Key] = resolveValue(a.Value)
		return true
	})
	if len(attrs) > 0 {
		m[AttrsKey] = attrs
	}
	return m
}

// resolveValue recursively processes an slog.Value.
//
// If the value is a group, it creates a nested map. Otherwise, it returns the value's underlying 'any'
// representation.
func resolveValue(v slog.Value) any {
	if v.Kind() == slog.KindGroup {
		attrs := v.Group()
		groupMap := make(map[string]any, len(attrs))
		for _, a := range attrs {
			groupMap[a.Key] = resolveValue(a.Value)
		}
		return groupMap
	}
	return v.Any()
}
