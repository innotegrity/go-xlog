package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"reflect"

	"go.innotegrity.dev/xlog"

	"go.innotegrity.dev/xerrors"
)

const (
	// FanoutHandlerType is the type for a [FanoutHandler].
	//
	// References:
	//   https://pkg.go.dev/go.innotegrity.dev/xlog/handlers#FanoutHandler
	FanoutHandlerType = "fanout"
)

// FanoutHandlerOptions holds the options for a [FanoutHandler].
type FanoutHandlerOptions struct {
	// Handlers holds the list of handlers to use for logging messages.
	Handlers []slog.Handler `json:"-"`
}

// ensure [FanoutHandler] implements [xlog.ExtendedHandler] interface.
var _ xlog.ExtendedHandler = &FanoutHandler{}

// FanoutHandler is a handler that simply writes messages to multiple child handlers.
type FanoutHandler struct {
	// unexported variables
	options FanoutHandlerOptions // handler options
}

// NewFanoutHandler creates a new [FanoutHandler] object.
//
// This function will never return an error. The returned error parameter is present to maintain consistency across
// handler "constructors".
func NewFanoutHandler(options FanoutHandlerOptions) (*FanoutHandler, xerrors.Error) {
	return &FanoutHandler{
		options: options,
	}, nil
}

// ChildHandlers returns the underlying [slog.Handler] children which each perform the actual logging.
func (h *FanoutHandler) ChildHandlers() []slog.Handler {
	return h.options.Handlers
}

// Close will close any child handlers.
func (h *FanoutHandler) Close() error {
	var errs []error
	for _, handler := range h.options.Handlers {
		if closer, ok := handler.(io.Closer); ok {
			if err := closer.Close(); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

// Enabled returns true if any child handler is enabled.
func (h *FanoutHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, handler := range h.options.Handlers {
		if handler.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

// Handle distributes a log record to all enabled handlers.
//
// The method:
//  1. iterates through all registered handlers
//  2. checks if each handler is enabled for the record's level
//  3. for enabled handlers, calls their Handle method with a cloned record
//  4. collects any errors that occur during handling
//  5. returns a combined error if any handlers failed
//
// Each handler receives a cloned record to prevent interference between handlers. This ensures that one handler
// cannot modify the record for other handlers.
func (h *FanoutHandler) Handle(ctx context.Context, r slog.Record) error {
	var errs []error
	for _, handler := range h.options.Handlers {
		if handler.Enabled(ctx, r.Level) {
			err := try(func() error {
				return handler.Handle(ctx, r.Clone())
			})
			if err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

// Options returns all of the child handler options in an array inside a string map under the "handlers" key.
func (h *FanoutHandler) Options() any {
	handlerOptions := []any{}
	for _, handler := range h.options.Handlers {
		if extHandler, ok := handler.(xlog.ExtendedHandler); ok {
			handlerOptions = append(handlerOptions, map[string]any{
				"type":    extHandler.Type(),
				"options": extHandler.Options(),
			})
		} else {
			t := reflect.TypeOf(handler)
			if t.Kind() == reflect.Ptr {
				t = t.Elem()
			}
			handlerOptions = append(handlerOptions, map[string]any{
				"type": t.String(),
				"options": map[string]any{
					"unknown": "handler does not support ExtendedHandler interface",
				},
			})
		}
	}
	return map[string]any{
		"handlers": handlerOptions,
	}
}

// Type returns the type of the handler.
func (h *FanoutHandler) Type() string {
	return FanoutHandlerType
}

// WithAttrs creates a new [FanoutHandler] with additional attributes added to all child handlers.
//
// The method creates new handler instances for each child handler with the additional attributes, ensuring that the
// attributes are properly propagated to all handlers in the fanout chain.
func (h *FanoutHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	handlers := make([]slog.Handler, len(h.options.Handlers))
	for i, handler := range h.options.Handlers {
		handlers[i] = handler.WithAttrs(attrs)
	}
	clone, _ := NewFanoutHandler(FanoutHandlerOptions{
		Handlers: handlers,
	})
	return clone
}

// WithGroup creates a new [FanoutHandler] with a group name applied to all child handlers.
//
// The method follows the same pattern as the standard slog implementation:
// - If the group name is empty, returns the original handler unchanged
// - Otherwise, creates new handler instances for each child handler with the group name
func (h *FanoutHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}

	handlers := make([]slog.Handler, len(h.options.Handlers))
	for i, handler := range h.options.Handlers {
		handlers[i] = handler.WithGroup(name)
	}
	clone, _ := NewFanoutHandler(FanoutHandlerOptions{
		Handlers: handlers,
	})
	return clone
}

// fanoutHandlerBuilderOptions simply holds the builders needed to build the child handlers for the [FanoutHandler].
type fanoutHandlerBuilderOptions struct {
	HandlerBuilders []handlerBuilder `json:"handlers"`
}

// fanoutHandlerBuilder is used to build the handler from configuration options.
type fanoutHandlerBuilder struct {
	// unexported variables
	options fanoutHandlerBuilderOptions // builder options
}

// NewFanoutHandlerBuilderFromConfig creates a new [xlog.HandlerBuilder] and validates the given options, setting
// and default values as necessary.
//
// This function may return an error with any of the following codes:
//   - [xlog.MarshalError]: error while unmarshaling options to JSON
func NewFanoutHandlerBuilderFromConfig(options json.RawMessage) (xlog.HandlerBuilder, xerrors.Error) {
	var opts fanoutHandlerBuilderOptions
	if err := json.Unmarshal(options, &opts); err != nil {
		return nil, xerrors.Wrapf(xlog.MarshalError, err, "failed to unmarshal handler options: %s",
			err.Error()).WithAttr("options", string(options))
	}

	return &fanoutHandlerBuilder{
		options: opts,
	}, nil
}

// Build will build each child handler and then the fanout handler and return it.
//
// The callback function is called for each child handler being built.
//
// This function may return an error with any of the following codes:
//   - [xlog.BuildHandlerError]: failed to construct one or more handlers
//
// This function may return other errors if the callback function fails and defines its own error values.
func (b *fanoutHandlerBuilder) Build(cb xlog.BuildHandlerCallbackFn) (slog.Handler, xerrors.Error) {
	var errs []error
	handlers := make([]slog.Handler, len(b.options.HandlerBuilders))
	for i, hb := range b.options.HandlerBuilders {
		handler, err := hb.builder.Build(cb)
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to build '%s' handler: %s", hb.builder.Type(), err.Error()))
		} else {
			handlers[i] = handler
		}
	}
	if len(errs) > 0 {
		return nil, xerrors.Wrap(xlog.BuildHandlerError, errors.Join(errs...),
			"failed to build one or more handlers")
	}
	return NewFanoutHandler(FanoutHandlerOptions{
		Handlers: handlers,
	})
}

// MarshalJSON overrides how the object is marshalled to JSON to alter how field values are presented or to
// add additional fields.
func (b *fanoutHandlerBuilder) MarshalJSON() ([]byte, error) {
	return json.Marshal(b.options)
}

// Options returns the options as a string map.
func (b *fanoutHandlerBuilder) Options() map[string]any {
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
func (b *fanoutHandlerBuilder) Type() string {
	return FanoutHandlerType
}
