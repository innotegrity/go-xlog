package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"go.innotegrity.dev/xlog"

	"github.com/lmittmann/tint"
	"github.com/mattn/go-colorable"
	"github.com/mattn/go-isatty"
	"go.innotegrity.dev/xerrors"
)

const (
	// ConsoleHandlerJSONFormat outputs messages in JSON format using [slog.JSONHandler].
	//
	// References:
	//   https://pkg.go.dev/log/slog#JSONHandler
	ConsoleHandlerJSONFormat ConsoleHandlerFormat = "json"

	// ConsoleHandlerPlaintextFormat outputs messages in plaintext format using [slog.TextHandler].
	//
	// References:
	//   https://pkg.go.dev/log/slog#TextHandler
	ConsoleHandlerPlaintextFormat ConsoleHandlerFormat = "plaintext"

	// ConsoleHandlerPrettyFormat outputs messages in a colorized format using [tint.NewHandler].
	//
	// References:
	//   https://pkg.go.dev/github.com/lmittmann/tint#NewHandler
	ConsoleHandlerPrettyFormat ConsoleHandlerFormat = "pretty"
)

const (
	// ConsoleHandlerType is the type for a [ConsoleHandler].
	//
	// References:
	//   https://pkg.go.dev/go.innotegrity.dev/xlog/handlers#ConsoleHandler
	ConsoleHandlerType = "console"
)

var (
	// DefaultConsoleHandlerLogLevel is the default log level to use when one is not provided.
	//
	// This value is used when the level in [ConsoleHandlerOptions] is unset.
	//
	// Setting this value changes the default globally for the package.
	//
	// References:
	//   https://pkg.go.dev/go.innotegrity.dev/xlog/handlers#ConsoleHandlerOptions
	DefaultConsoleHandlerLogLevel = slog.LevelInfo

	// DefaultConsoleHandlerFormat is the default output format to use for the handler.
	//
	// This value is used when the format in [ConsoleHandlerOptions] is empty.
	//
	// Setting this value changes the default globally for the package.
	//
	// References:
	//   https://pkg.go.dev/go.innotegrity.dev/xlog/handlers#ConsoleHandlerOptions
	DefaultConsoleHandlerFormat = ConsoleHandlerPrettyFormat
)

// ConsoleHandlerFormat is a pre-defined output format for the console.
type ConsoleHandlerFormat string

// ConsoleHandlerOptions holds the options for a [ConsoleHandler].
type ConsoleHandlerOptions struct {
	// ErrorHandler is a function that's called to process any internal errors that may occur when a message is
	// processed by the underlying handler.
	//
	// The default behavior is to ignore these errors.
	//
	// When reading configuration settings from a file or raw JSON, create an [xlog.HandlerBuilder] and pass the
	// [xlog.HandlerBuilder.Build] function an [xlog.HandlerBuildCallbackFn] callback to modify the options and
	// set this value from your application, if desired.
	//
	// References:
	//   https://pkg.go.dev/go.innotegrity.dev/xlog#HandlerBuilder
	//   https://pkg.go.dev/go.innotegrity.dev/xlog#HandlerBuilder.Build
	//   https://pkg.go.dev/go.innotegrity.dev/xlog#HandlerBuilderBuildCallbackFn
	ErrorHandler xlog.ErrorHandlerFn `json:"-"`

	// Format stores the output format for the handler.
	//
	// The default behavior is defined by the default format setting defined in the package.
	//
	// When reading configuration settings from a file or raw JSON, if this value is not present, it will be set
	// to an empty string.
	Format ConsoleHandlerFormat `json:"format"`

	// IncludeCaller indicates whether or not to include the caller in log messages.
	//
	// The default behavior is to not include caller information.
	//
	// When reading configuration settings from a file or raw JSON, if this value is not present, it will be set
	// to false.
	IncludeCaller bool `json:"include_caller"`

	// Level is the minimum level at which to log messages.
	//
	// The default behavior is defined by the default level setting defined in the package.
	//
	// When reading configuration settings from a file or raw JSON, if this value is not present, it will be set
	// to nil.
	Level *slog.LevelVar `json:"level"`

	// MaxLevel is the maximum level at which to log messages.
	//
	// The default behavior is to disable any maximum log message level.
	//
	// When reading configuration settings from a file or raw JSON, if this value is not present, it will be set
	// to nil.
	MaxLevel *slog.LevelVar `json:"max_level,omitempty"`

	// ReplaceAttr is called to rewrite each non-group attribute before it is logged.
	//
	// The attribute's value has been resolved (see [slog.Value.Resolve]). If ReplaceAttr returns a zero Attr, the
	// attribute is discarded.
	//
	// The built-in attributes with keys [slog.TimeKey], [slog.LevelKey], [slog.SourceKey], and [slog.MessageKey]
	// are passed to this function, except that time is omitted if zero, and source is omitted if IncludeCaller is
	// false.
	//
	// The default behavior is to not replace any attributes.
	//
	// When reading configuration settings from a file or raw JSON, create an [xlog.HandlerBuilder] and pass the
	// [xlog.HandlerBuilder.Build] function an [xlog.HandlerBuildCallbackFn] callback to modify the options and
	// set this value from your application, if desired.
	//
	// References:
	//   https://pkg.go.dev/log/slog#TimeKey
	//   https://pkg.go.dev/log/slog#LevelKey
	//   https://pkg.go.dev/log/slog#SourceKey
	//   https://pkg.go.dev/log/slog#MessageKey
	//   https://pkg.go.dev/log/slog#HandlerOptions
	//   https://pkg.go.dev/go.innotegrity.dev/xlog#HandlerBuilder
	//   https://pkg.go.dev/go.innotegrity.dev/xlog#HandlerBuilder.Build
	//   https://pkg.go.dev/go.innotegrity.dev/xlog#HandlerBuilderBuildCallbackFn
	ReplaceAttr func(groups []string, attr slog.Attr) slog.Attr `json:"-"`

	// Stderr is a flag to send messages for this handler to stderr instead of stdout.
	//
	// When reading configuration settings from a file or raw JSON, if this value is not present, it will be set
	// to false.
	Stderr bool `json:"stderr"`
}

// jsonConsoleHandlerOptions is an alternate form of [ConsoleHandlerOptions] that is used during unmarshalling to
// prevent infinite recursion.
type jsonConsoleHandlerOptions struct {
	Format        string `json:"format"`
	IncludeCaller bool   `json:"include_caller"`
	Level         string `json:"level"`
	MaxLevel      string `json:"max_level"`
	Stderr        bool   `json:"stderr"`
}

// UnmarshalJSON decodes the JSON-encoded data into the current object.
func (o *ConsoleHandlerOptions) UnmarshalJSON(data []byte) error {
	var opts jsonConsoleHandlerOptions
	if err := json.Unmarshal(data, &opts); err != nil {
		return err
	}

	// validate the format
	//
	// note that we purposely leave the format empty here if it's not set so that it can be set when the handler
	// is created or overridden by the calling application
	format := ConsoleHandlerFormat(strings.TrimSpace(strings.ToLower(opts.Format)))
	switch format {
	case ConsoleHandlerJSONFormat, ConsoleHandlerPlaintextFormat, ConsoleHandlerPrettyFormat, "":
		o.Format = format
	default:
		return fmt.Errorf("%s: invalid format for console handler", opts.Format)
	}

	// validate the log level(s)
	//
	// note that we purposely leave the level nil here if it's not set so that it can be set when the handler
	// is created or overridden by the calling application
	if opts.Level != "" {
		var level slog.LevelVar
		if err := level.UnmarshalText([]byte(opts.Level)); err != nil {
			return fmt.Errorf("failed to parse level '%s' for console handler: %s", opts.Level, err.Error())
		}
		o.Level = &level
	}
	if opts.MaxLevel != "" {
		var level slog.LevelVar
		if err := level.UnmarshalText([]byte(opts.MaxLevel)); err != nil {
			return fmt.Errorf("failed to parse max level '%s' for console handler: %s", opts.MaxLevel, err.Error())
		}
		o.MaxLevel = &level
	}

	// copy remaining options
	o.IncludeCaller = opts.IncludeCaller
	o.Stderr = opts.Stderr

	return nil
}

// ensure [ConsoleHandler] implements [xlog.ExtendedHandler] interface.
var _ xlog.ExtendedHandler = &ConsoleHandler{}

// ensure [ConsoleHandler] implements [xlog.LevelVarHandler] interface.
var _ xlog.LevelVarHandler = &ConsoleHandler{}

// ConsoleHandler is a handler that simply writes messages to stdout or stderr.
type ConsoleHandler struct {
	// unexported variables
	handler slog.Handler          // underlying handler used for output
	options ConsoleHandlerOptions // handler options
}

// NewConsoleHandler creates a new [ConsoleHandler] object with the given options.
//
// This function may return an error with any of the following codes:
//   - [xlog.OptionsValidationError]: one or more options are invalid
func NewConsoleHandler(options ConsoleHandlerOptions) (*ConsoleHandler, xerrors.Error) {
	h := &ConsoleHandler{
		options: options,
	}

	// setup the output writer to stdout or stderr
	writer := os.Stdout
	if h.options.Stderr {
		writer = os.Stderr
	}

	// ensure a minimum level is set
	if h.options.Level == nil {
		var level slog.LevelVar
		level.Set(DefaultConsoleHandlerLogLevel)
		h.options.Level = &level
	}

	// create the handler based on the format
	if h.options.Format == "" {
		h.options.Format = DefaultConsoleHandlerFormat
	}
	switch h.options.Format {
	case ConsoleHandlerJSONFormat:
		h.handler = slog.NewJSONHandler(writer, &slog.HandlerOptions{
			AddSource:   h.options.IncludeCaller,
			Level:       h.options.Level,
			ReplaceAttr: h.options.ReplaceAttr,
		})
	case ConsoleHandlerPlaintextFormat:
		h.handler = slog.NewTextHandler(writer, &slog.HandlerOptions{
			AddSource:   h.options.IncludeCaller,
			Level:       h.options.Level,
			ReplaceAttr: h.options.ReplaceAttr,
		})
	case ConsoleHandlerPrettyFormat:
		h.handler = tint.NewHandler(colorable.NewColorable(writer), &tint.Options{
			AddSource:   h.options.IncludeCaller,
			Level:       h.options.Level,
			NoColor:     !isatty.IsTerminal(writer.Fd()),
			ReplaceAttr: h.options.ReplaceAttr,
			TimeFormat:  "2006-01-02 15:04:05",
		})
	default:
		return nil, xerrors.Newf(xlog.OptionsValidationError, "%s: invalid console handler format",
			h.options.Format).WithAttr("format", h.options.Format)
	}

	return h, nil
}

// ChildHandlers returns the underlying [slog.Handler] which actually performs the logging.
func (h *ConsoleHandler) ChildHandlers() []slog.Handler {
	return []slog.Handler{h.handler}
}

// Close does nothing for this handler.
func (h *ConsoleHandler) Close() error {
	return nil
}

// Enabled returns true if the handler should handle the message or false if it should not.
func (h *ConsoleHandler) Enabled(ctx context.Context, level slog.Level) bool {
	handlerLevel := h.options.Level.Level()
	if h.options.MaxLevel == nil {
		return level >= handlerLevel
	}
	return level >= handlerLevel && level <= handlerLevel
}

// GetLevelVar returns the handler's [slog.LevelVar] for manipulating the minimum logging level.
func (h *ConsoleHandler) GetLevelVar() *slog.LevelVar {
	return h.options.Level
}

// GetMaxLevelVar returns the handler's [slog.LevelVar] for manipulating the maximum logging level.
func (h *ConsoleHandler) GetMaxLevelVar() *slog.LevelVar {
	return h.options.MaxLevel
}

// Handle processes the record and handles logging it.
func (h *ConsoleHandler) Handle(ctx context.Context, r slog.Record) error {
	err := h.handler.Handle(ctx, r)
	if err != nil && h.options.ErrorHandler != nil {
		err = h.options.ErrorHandler(ctx, err, &r)
	}
	return err
}

// Options returns the handler's options.
func (h *ConsoleHandler) Options() any {
	return h.options
}

// Type returns the type of the handler.
func (h *ConsoleHandler) Type() string {
	return ConsoleHandlerType
}

// WithAttrs returns a new handler whose attributes consist of both the current object's attributes and the
// given attributes.
func (h *ConsoleHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	clone := h.clone()
	clone.handler = h.handler.WithAttrs(attrs)
	return clone
}

// WithGroup returns a new handler with the existing object's attributes part of the given group.
func (h *ConsoleHandler) WithGroup(name string) slog.Handler {
	if len(name) == 0 {
		return h
	}

	clone := h.clone()
	clone.handler = h.handler.WithGroup(name)
	return clone
}

// clone creates a copy of current handler.
func (h *ConsoleHandler) clone() *ConsoleHandler {
	return &ConsoleHandler{
		handler: h.handler,
		options: h.options,
	}
}

// consoleHandlerBuilder is used to build the handler from configuration options.
type consoleHandlerBuilder struct {
	// unexported variables
	options ConsoleHandlerOptions // handler options
}

// NewConsoleHandlerBuilderFromConfig creates a new [xlog.HandlerBuilder] and validates the given options, setting
// and default values as necessary.
//
// This function may return an error with any of the following codes:
//   - [xlog.MarshalError]: error while unmarshaling options to JSON
func NewConsoleHandlerBuilderFromConfig(options json.RawMessage) (xlog.HandlerBuilder, xerrors.Error) {
	var opts ConsoleHandlerOptions
	if err := json.Unmarshal(options, &opts); err != nil {
		return nil, xerrors.Wrapf(xlog.MarshalError, err, "failed to unmarshal handler options: %s",
			err.Error()).WithAttr("options", string(options))
	}

	return &consoleHandlerBuilder{
		options: opts,
	}, nil
}

// Build actually creates and returns the handler.
//
// This function may return an error with any of the following codes:
//   - [xlog.BuildHandlerError]: failed to construct the new handler
//
// This function may return other errors if the callback function fails and defines its own error values.
func (b *consoleHandlerBuilder) Build(cb xlog.BuildHandlerCallbackFn) (slog.Handler, xerrors.Error) {
	if cb != nil {
		if err := cb(b.Type(), &b.options); err != nil {
			return nil, err
		}
	}
	h, err := NewConsoleHandler(b.options)
	if err != nil {
		return nil, xerrors.Wrapf(xlog.BuildHandlerError, err, "failed to build '%s' handler: %s", b.Type(),
			err.Error())
	}
	return h, nil
}

// MarshalJSON overrides how the object is marshalled to JSON to alter how field values are presented or to
// add additional fields.
func (b *consoleHandlerBuilder) MarshalJSON() ([]byte, error) {
	return json.Marshal(b.options)
}

// Options returns the options as a string map.
func (b *consoleHandlerBuilder) Options() map[string]any {
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
func (b *consoleHandlerBuilder) Type() string {
	return ConsoleHandlerType
}
