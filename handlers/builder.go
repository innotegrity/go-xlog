package handlers

import (
	"encoding/json"
	"strings"

	"go.innotegrity.dev/xerrors"
	"go.innotegrity.dev/xlog"
)

// NewBuilderFromConfig parses and validates the given handler type and its options and returns a new
// [HandlerBuilder] for creating the handler when ready.
//
// This function may return an error with any of the following codes:
//   - [xlog.MarshalError]: error while unmarshaling options to JSON
//   - [xlog.UnsupportedHandlerType]: unknown or unsupported handler type was encountered
//
// In addition, the function may return any error returned by the "New..." function for any type of builder supported
// by this package.
//
// To register additional builders outside of the built-in builders, use the [RegisterBuilder] function.
func NewBuilderFromConfig(handlerType string, options map[string]any) (xlog.HandlerBuilder, xerrors.Error) {
	handlerType = strings.TrimSpace(strings.ToLower(handlerType))

	// marshal the options to JSON
	jsonOptions, err := json.Marshal(options)
	if err != nil {
		return nil, xerrors.Wrapf(xlog.MarshalError, err, "failed to marshal handler options to JSON: %s",
			err.Error()).WithAttrs(map[string]any{
			"type":    handlerType,
			"options": options,
		})
	}

	// create the builder
	if factoryFn, ok := _builders[handlerType]; ok {
		return factoryFn(jsonOptions)
	}
	return nil, xerrors.Newf(xlog.UnsupportedHandlerType, "unsupported handler type: %s", handlerType).
		WithAttrs(map[string]any{
			"type":    handlerType,
			"options": options,
		})
}

// RegisterBuilder attempts to register an [xlog.NewBuilderFromConfigFn] for creating a handler builder with the given
// handler type.
//
// To overwrite the function attached to a particular handler type, set overwrite to true.
//
// This function may return an error with any of the following codes:
//   - [xlog.InvalidParameter]: an invalid parameter was passed to the function (eg: handler was empty or factory
//     function was nil)
//   - [xlog.HandlerTypeExists]: a builder for the given handler type already exists
func RegisterBuilder(handlerType string, factoryFn xlog.NewBuilderFromConfigFn, overwrite bool) xerrors.Error {
	handlerType = strings.TrimSpace(strings.ToLower(handlerType))
	if handlerType == "" {
		return xerrors.New(xlog.InvalidParameter, "handler type cannot be empty")
	}
	if factoryFn == nil {
		return xerrors.New(xlog.InvalidParameter, "factory function cannot be nil")
	}
	if _, ok := _builders[handlerType]; ok && !overwrite {
		return xerrors.Newf(xlog.HandlerTypeExists, "%s: handler type is already registered").
			WithAttr("type", handlerType)
	}
	_builders[handlerType] = factoryFn
	return nil
}

// handlerBuilder is used to build a handler that contains child handlers.
type handlerBuilder struct {
	// HandlerType holds the type of the handler to build.
	HandlerType string `json:"type"`

	// HandlerOptions holds the options for the handler to build.
	HandlerOptions map[string]any `json:"options"`

	// unexported variables
	builder xlog.HandlerBuilder // the underlying builder to use to build the new handler
}

// jsonHandlerBuilder is just an alias for [handlerBuilder] that is used during marshalling and unmarshalling to
// prevent infinite recursion.
type jsonHandlerBuilder handlerBuilder

// UnmarshalJSON decodes the JSON-encoded data into the current object.
func (h *handlerBuilder) UnmarshalJSON(data []byte) error {
	var b jsonHandlerBuilder
	if err := json.Unmarshal(data, &b); err != nil {
		return err
	}

	builder, err := NewBuilderFromConfig(b.HandlerType, b.HandlerOptions)
	if err != nil {
		return err
	}
	h.HandlerType = b.HandlerType
	h.HandlerOptions = b.HandlerOptions
	h.builder = builder

	return nil
}
