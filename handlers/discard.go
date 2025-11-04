package handlers

import (
	"context"
	"encoding/json"
	"log/slog"

	"go.innotegrity.dev/xlog"

	"go.innotegrity.dev/xerrors"
)

const (
	// DiscardHandlerType is the type for a [DiscardHandler].
	//
	// References:
	//   https://pkg.go.dev/go.innotegrity.dev/xlog/handlers#DiscardHandler
	DiscardHandlerType = "discard"
)

// DiscardHandlerOptions holds the options for a [DiscardHandler].
type DiscardHandlerOptions struct{}

// ensure [DiscardHandler] implements [ExtendedHandler] interface.
var _ xlog.ExtendedHandler = &DiscardHandler{}

// DiscardHandler is a handler that simply writes messages to stdout or stderr.
type DiscardHandler struct {
	// unexported variables
	handler slog.Handler          // underlying handler used for output
	options DiscardHandlerOptions // handler options
}

// NewDiscardHandler creates a new [DiscardHandler] object with the given options.
//
// This function will never return an error. The returned error parameter is present to maintain consistency across
// handler "constructors".
func NewDiscardHandler(options DiscardHandlerOptions) (*DiscardHandler, xerrors.Error) {
	return &DiscardHandler{
		handler: slog.DiscardHandler,
		options: options,
	}, nil
}

// ChildHandlers returns the underlying [slog.Handler] which actually performs the logging.
func (h *DiscardHandler) ChildHandlers() []slog.Handler {
	return []slog.Handler{h.handler}
}

// Close does nothing for this handler.
func (h *DiscardHandler) Close() error {
	return nil
}

// Enabled always returns false as this handler just discards messages.
func (h *DiscardHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return false
}

// Handle does nothing as it just discards the record.
func (h *DiscardHandler) Handle(ctx context.Context, r slog.Record) error {
	return nil
}

// Options returns the handler's options.
func (h *DiscardHandler) Options() any {
	return h.options
}

// Type returns the type of the handler.
func (h *DiscardHandler) Type() string {
	return DiscardHandlerType
}

// WithAttrs returns a new handler whose attributes consist of both the current object's attributes and the
// given attributes.
func (h *DiscardHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return h.handler.WithAttrs(attrs)
}

// WithGroup returns a new handler with the existing object's attributes part of the given group.
func (h *DiscardHandler) WithGroup(name string) slog.Handler {
	return h.handler.WithGroup(name)
}

// discardHandlerBuilder is used to build the handler from configuration options.
type discardHandlerBuilder struct {
	// unexported variables
	options DiscardHandlerOptions // handler options
}

// NewDiscardHandlerBuilderFromConfig creates a new [xlog.HandlerBuilder] and validates the given options, setting
// and default values as necessary.
//
// This function may return an error with any of the following codes:
//   - [xlog.MarshalError]: error while unmarshaling options to JSON
func NewDiscardHandlerBuilderFromConfig(options json.RawMessage) (xlog.HandlerBuilder, xerrors.Error) {
	var opts DiscardHandlerOptions
	if err := json.Unmarshal(options, &opts); err != nil {
		return nil, xerrors.Wrapf(xlog.MarshalError, err, "failed to unmarshal handler options: %s",
			err.Error()).WithAttr("options", string(options))
	}

	return &discardHandlerBuilder{
		options: opts,
	}, nil
}

// Build actually creates and returns the handler.
//
// This function may return an error if the callback function fails and defines its own error values.
func (b *discardHandlerBuilder) Build(cb xlog.BuildHandlerCallbackFn) (slog.Handler, xerrors.Error) {
	if cb != nil {
		if err := cb(b.Type(), &b.options); err != nil {
			return nil, err
		}
	}
	h, err := NewDiscardHandler(b.options)
	if err != nil {
		return nil, xerrors.Wrapf(xlog.BuildHandlerError, err, "failed to build '%s' handler: %s", b.Type(),
			err.Error())
	}
	return h, nil
}

// MarshalJSON overrides how the object is marshalled to JSON to alter how field values are presented or to
// add additional fields.
func (b *discardHandlerBuilder) MarshalJSON() ([]byte, error) {
	return json.Marshal(b.options)
}

// Options returns the options as a string map.
func (b *discardHandlerBuilder) Options() map[string]any {
	jsonOptions, err := json.Marshal(b)
	if err != nil {
		return map[string]any{
			"error": err.Error(),
		}
	}

	var options map[string]any
	if err := json.Unmarshal(jsonOptions, &options); err != nil {
		return map[string]any{
			"error": err.Error(),
		}
	}
	return options
}

// Type returns the type of the handler being built.
func (b *discardHandlerBuilder) Type() string {
	return DiscardHandlerType
}
