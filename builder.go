package xlog

import (
	"encoding/json"
	"log/slog"

	"go.innotegrity.dev/xerrors"
)

// NewBuilderFromConfigFn should create a new [HandlerBuilder] object using the raw JSON options it is passed.
type NewBuilderFromConfigFn func(options json.RawMessage) (HandlerBuilder, xerrors.Error)

// BuildHandlerCallbackFn is passed a handler type and a pointer to its concrete options so that any options can
// be overridden by a calling application before the handler is actually built.
//
// This gives the application an opportunity to foricbly overwrite option values based on its own defaults or settings
// from feature flags.
//
// The function should modify the options as necessary and return nil on success or an error on failure.
type BuildHandlerCallbackFn func(handlerType string, options any) xerrors.Error

// HandlerBuilder defines the interface that must be implemented in order to build an [slog.Handler] from settings
// read from a configuration file.
type HandlerBuilder interface {
	json.Marshaler

	// Build should process stored handler options and create/initialize the handler.
	Build(cb BuildHandlerCallbackFn) (slog.Handler, xerrors.Error)

	// Options should return the options as a string map.
	Options() map[string]any

	// Type should return the type of the handler.
	Type() string
}
