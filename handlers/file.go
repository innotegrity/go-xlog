package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"go.innotegrity.dev/types"
	"go.innotegrity.dev/xerrors"
	"go.innotegrity.dev/xlog"
	"gopkg.in/natefinch/lumberjack.v2"
)

const (
	// FileHandlerType is the type for a [FileHandler].
	//
	// References:
	//   https://pkg.go.dev/go.innotegrity.dev/xlog/handlers#FileHandler
	FileHandlerType = "file"
)

var (
	// DefaultFileHandlerAutoChmodLogFile is the flag to indicate whether or not [os.Chmod] should be called on the
	// log file after it is created or on its parent directory if parent directory creation is enabled.
	//
	// This value is used when no setting is present in the [FileHandlerOptions] parsed from a configuration file or
	// raw JSON.
	//
	// Setting this value changes the default globally for the package.
	//
	// References:
	//   https://pkg.go.dev/go.innotegrity.dev/xlog/handlers#FileHandlerOptions
	DefaultFileHandlerAutoChmodLogFile = true

	// DefaultFileHandlerAutoChownLogFile is the flag to indicate whether or not [os.Chown] should be called on the
	// log file after it is created or on its parent directory if parent directory creation is enabled.
	//
	// This value is used when no setting is present in the [FileHandlerOptions] parsed from a configuration file or
	// raw JSON.
	//
	// Setting this value changes the default globally for the package.
	//
	// References:
	//   https://pkg.go.dev/go.innotegrity.dev/xlog/handlers#FileHandlerOptions
	DefaultFileHandlerAutoChownLogFile = false

	// DefaultFileHandlerAutoCreateLogFileParent is the flag to indicate whether or not [os.MkdirAll] should be called
	// on the parent folder of the log file before it is created.
	//
	// This value is used when no setting is present in the [FileHandlerOptions] parsed from a configuration file or
	// raw JSON.
	//
	// Setting this value changes the default globally for the package.
	//
	// References:
	//   https://pkg.go.dev/go.innotegrity.dev/xlog/handlers#FileHandlerOptions
	DefaultFileHandlerAutoCreateLogFileParent = true

	// DefaultFileHandlerDirMode is the mode that will be used to create any parent directories of the log file if
	// parent directory creation is enabled or if the auto chmod feature is enabled.
	//
	// This value will be used when the log file's [types.Path.DirMode] in [FileHandlerOptions] is set to 0.
	//
	// Setting this value changes the default globally for the package.
	//
	// References:
	//   https://pkg.go.dev/go.innotegrity.dev/xlog/handlers#FileHandlerOptions
	//   https://pkg.go.dev/go.innotegrity.dev/types#Path.DirMode
	DefaultFileHandlerDirMode = types.FileMode(0755)

	// DefaultFileHandlerFileMode is the mode that will be used to for the log file itself when it is created or when
	// the auto chmod feature is enabled.
	//
	// This value will be used when the log file's [types.Path.FileMode] in [FileHandlerOptions] is set to 0.
	//
	// Setting this value changes the default globally for the package.
	//
	// References:
	//   https://pkg.go.dev/go.innotegrity.dev/xlog/handlers#FileHandlerOptions
	//   https://pkg.go.dev/go.innotegrity.dev/types#Path.DirMode
	DefaultFileHandlerFileMode = types.FileMode(0640)

	// DefaultFileHandlerFileName is the name of the log file that will be used if no log file path is specified and
	// the name of the executable cannot be retrieved.
	//
	// The log file will be created using this name in the first log folder in which can be written to.
	//
	// This value is used when the log file's [types.Path.FSPath] in [FileHandlerOptions] is empty.
	//
	// Setting this value changes the default globally for the package.
	//
	// References:
	//   https://pkg.go.dev/go.innotegrity.dev/xlog/handlers#FileHandlerOptions
	//   https://pkg.go.dev/go.innotegrity.dev/types#Path.FSpath
	DefaultFileHandlerFileName = "app.log"

	// DefaultFileHandlerLogFolders is a list of possible folders where the log file can be written to. The first
	// folder in the list which allows for the successful creation of the log file will be used.
	//
	// This value is used when the log file's [types.Path.FSPath] in [FileHandlerOptions] is empty.
	//
	// Setting this value changes the default globally for the package.
	//
	// References:
	//   https://pkg.go.dev/go.innotegrity.dev/xlog/handlers#FileHandlerOptions
	//   https://pkg.go.dev/go.innotegrity.dev/types#Path.FSpath
	DefaultFileHandlerLogFolders = []string{"."}

	// DefaultFileHandlerLogLevel is the log level to use for the handler.
	//
	// This value is used when the level in [FileHandlerOptions] is unset.
	//
	// Setting this value changes the default globally for the package.
	// References:
	//   https://pkg.go.dev/go.innotegrity.dev/xlog/handlers#FileHandlerOptions
	DefaultFileHandlerLogLevel = slog.LevelInfo
)

// FileHandlerOptions holds the options for a [FileHandler].
type FileHandlerOptions struct {
	// BufferSize indicates the size (in bytes) of the buffer to use before flushing records to the file.
	//
	// The default behavior is to disable buffering.
	//
	// When reading configuration settings from a file or raw JSON, if this value is not present, it will be set
	// to 0.
	BufferSize types.Size `json:"buffer_size"`

	// Compress indicates whether or not to compress rotated log files using gzip.
	//
	// The default behavior is to disable compression.
	//
	// When reading configuration settings from a file or raw JSON, if this value is not present, it will be set
	// to false.
	Compress bool `json:"compress"`

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

	// File is the output path for the file.
	//
	// The default behavior is defined by the default file settings defined in the package. If the group or owner
	// members are left set to -1, the current user's ID and group ID are used for ownership.
	//
	// When reading configuration settings from a file or raw JSON, if this value is not present, the following
	// values are set:
	// 	 - AutoChmod is set to the global package default value.
	//	 - AutoChown is set to the global package default value.
	//	 - AutoCreateParent is set to the global package default value.
	//	 - DirMode will be 0.
	//	 - FileMode will be 0.
	//	 - FSPath will be an empty string.
	//	 - Group will be -1.
	//	 - Owner will be -1.
	File types.Path `json:"file"`

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

	// MaxAge is the maximum number of days to retain old log files based on the
	// timestamp encoded in their filename.
	//
	// Note that a day is defined as 24 hours and may not exactly correspond to calendar days due to daylight
	// savings, leap seconds, etc.
	//
	// The default behavior is not to remove old log files based on age.
	//
	// When reading configuration settings from a file or raw JSON, if this value is not present, it will be set
	// to 0.
	MaxAge int `json:"max_age,omitempty"`

	// MaxCount is the maximum number of old log files to retain.
	//
	// The default behavior is to retain all old log files (though MaxAge may still cause them to get deleted).
	//
	// When reading configuration settings from a file or raw JSON, if this value is not present, it will be set
	// to 0.
	MaxCount int `json:"max_count,omitempty"`

	// MaxLevel is the maximum level at which to log messages.
	//
	// The default behavior is to disable any maximum log message level.
	//
	// When reading configuration settings from a file or raw JSON, if this value is not present, it will be set
	// to nil.
	MaxLevel *slog.LevelVar `json:"max_level,omitempty"`

	// MaxSize is the maximum size in megabytes of the log file before it gets rotated.
	//
	// The default behavior is to rotate files when they reach 100MB in size.
	//
	// When reading configuration settings from a file or raw JSON, if this value is not present, it will be set
	// to 0.
	MaxSize int `json:"max_size,omitempty"`

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
}

// jsonFileHandlerOptions is an alternate form of [FileHandlerOptions] that is used during unmarshalling to prevent
// infinite recursion.
type jsonFileHandlerOptions struct {
	BufferSize types.Size `json:"buffer_size"`
	Compress   bool       `json:"compress"`
	File       struct {
		AutoChmod        *bool           `json:"auto_chmod"`
		AutoChown        *bool           `json:"auto_chown"`
		AutoCreateParent *bool           `json:"auto_create_parent"`
		DirMode          *types.FileMode `json:"dir_mode"`
		FileMode         *types.FileMode `json:"file_mode"`
		FSPath           *string         `json:"path"`
		Group            *types.GroupID  `json:"group"`
		Owner            *types.UserID   `json:"owner"`
	} `json:"file"`
	IncludeCaller bool   `json:"include_caller"`
	Level         string `json:"level"`
	MaxAge        int    `json:"max_age"`
	MaxCount      int    `json:"max_count"`
	MaxLevel      string `json:"max_level"`
	MaxSize       int    `json:"max_size"`
}

// UnmarshalJSON decodes the JSON-encoded data into the current object.
func (o *FileHandlerOptions) UnmarshalJSON(data []byte) error {
	var opts jsonFileHandlerOptions
	if err := json.Unmarshal(data, &opts); err != nil {
		return err
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

	// configure file defaults
	//
	// note that we purposely leave some values unchanged here if they're not set so that they can be set when the
	// handler is created or overridden by the calling application
	if opts.File.AutoChmod != nil {
		o.File.AutoChmod = *opts.File.AutoChmod
	} else {
		o.File.AutoChmod = DefaultFileHandlerAutoChmodLogFile
	}
	if opts.File.AutoChown != nil {
		o.File.AutoChown = *opts.File.AutoChown
	} else {
		o.File.AutoChown = DefaultFileHandlerAutoChownLogFile
	}
	if opts.File.AutoCreateParent != nil {
		o.File.AutoCreateParent = *opts.File.AutoCreateParent
	} else {
		o.File.AutoCreateParent = DefaultFileHandlerAutoCreateLogFileParent
	}
	if opts.File.DirMode != nil {
		o.File.DirMode = *opts.File.DirMode
	} else {
		o.File.DirMode = 0
	}
	if opts.File.FileMode != nil {
		o.File.FileMode = *opts.File.FileMode
	} else {
		o.File.FileMode = 0
	}
	if opts.File.FSPath != nil {
		o.File.FSPath = *opts.File.FSPath
	} else {
		o.File.FSPath = ""
	}
	if opts.File.Group != nil {
		o.File.Group = *opts.File.Group
	} else {
		o.File.Group = -1
	}
	if opts.File.Owner != nil {
		o.File.Owner = *opts.File.Owner
	} else {
		o.File.Owner = -1
	}

	// copy remaining options
	o.BufferSize = opts.BufferSize
	o.Compress = opts.Compress
	o.IncludeCaller = opts.IncludeCaller
	o.MaxAge = opts.MaxAge
	o.MaxCount = opts.MaxCount
	o.MaxSize = opts.MaxSize

	return nil
}

// ensure [FileHandler] implements [xlog.ExtendedHandler] interface.
var _ xlog.ExtendedHandler = &FileHandler{}

// ensure [FileHandler] implements [xlog.LevelVarHandler] interface.
var _ xlog.LevelVarHandler = &FileHandler{}

// FileHandler is a handler that writes messages to a file with optional buffering and file rotation.
type FileHandler struct {
	// unexported variables
	bufferedWriter *atomicWriter      // buffer writer
	fileWriter     *lumberjack.Logger // lumberjack logger
	handler        slog.Handler       // underlying handler used for output
	options        FileHandlerOptions // handler options
}

// NewFileHandler creates a new [FileHandler] object with the given options.
//
// This function may return an error with any of the following codes:
//   - [xlog.OptionsValidationError]: one or more options are invalid
func NewFileHandler(options FileHandlerOptions) (*FileHandler, xerrors.Error) {
	var writer io.Writer
	h := &FileHandler{
		options: options,
	}

	// ensure a minimum level is set
	if h.options.Level == nil {
		var level slog.LevelVar
		level.Set(DefaultFileHandlerLogLevel)
		h.options.Level = &level
	}

	// set file defaults
	if h.options.File.DirMode == 0 {
		h.options.File.DirMode = DefaultFileHandlerDirMode
	}
	if h.options.File.FileMode == 0 {
		h.options.File.FileMode = DefaultFileHandlerFileMode
	}
	if h.options.File.Owner == -1 {
		h.options.File.Owner = types.UserID(os.Getuid())
	}
	if h.options.File.Group == -1 {
		h.options.File.Group = types.GroupID(os.Getgid())
	}

	// construct the lumberjack logger for file rotation
	filename, xerr := createLogFile(h.options.File)
	if xerr != nil {
		return nil, xerr
	}
	filename, err := filepath.Abs(filename)
	if err != nil {
		return nil, xerrors.Wrapf(xlog.OptionsValidationError, err,
			"failed to convert log file path '%s' to an absolute path: %s", filename, err.Error()).
			WithAttr("log_file", filename)
	}
	h.options.File.FSPath = filename
	h.fileWriter = &lumberjack.Logger{
		Compress:   h.options.Compress,
		Filename:   filename,
		MaxAge:     h.options.MaxAge,
		MaxBackups: h.options.MaxCount,
		MaxSize:    h.options.MaxSize,
	}
	writer = h.fileWriter

	// construct the buffered writer, if enabled
	if h.options.BufferSize > 0 {
		h.bufferedWriter = newAtomicWriter(h.fileWriter, int(h.options.BufferSize))
		writer = h.bufferedWriter
	}

	// create the JSON handler for the output
	h.handler = slog.NewJSONHandler(writer, &slog.HandlerOptions{
		AddSource:   h.options.IncludeCaller,
		Level:       h.options.Level,
		ReplaceAttr: h.options.ReplaceAttr,
	})
	return h, nil
}

// ChildHandlers returns the underlying [slog.Handler] which actually performs the logging.
func (h *FileHandler) ChildHandlers() []slog.Handler {
	return []slog.Handler{h.handler}
}

// Close flushes any data in the buffer to the file and then closes the file handle.
func (h *FileHandler) Close() error {
	if h.bufferedWriter != nil {
		if err := h.bufferedWriter.Flush(); err != nil {
			return err
		}
	}
	if h.fileWriter != nil {
		if err := h.fileWriter.Close(); err != nil {
			return err
		}
	}
	return nil
}

// Enabled returns true if the handler should handle the message or false if it should not.
func (h *FileHandler) Enabled(ctx context.Context, level slog.Level) bool {
	handlerLevel := h.options.Level.Level()
	if h.options.MaxLevel == nil {
		return level >= handlerLevel
	}
	return level >= handlerLevel && level <= handlerLevel
}

// GetLevelVar returns the handler's [slog.LevelVar] for manipulating the minimum logging level.
func (h *FileHandler) GetLevelVar() *slog.LevelVar {
	return h.options.Level
}

// GetMaxLevelVar returns the handler's [slog.LevelVar] for manipulating the maximum logging level.
func (h *FileHandler) GetMaxLevelVar() *slog.LevelVar {
	return h.options.MaxLevel
}

// Handle processes the record and handles logging it.
func (h *FileHandler) Handle(ctx context.Context, r slog.Record) error {
	err := h.handler.Handle(ctx, r)
	if err != nil && h.options.ErrorHandler != nil {
		err = h.options.ErrorHandler(ctx, err, &r)
	}
	return err
}

// Options returns the handler's options.
func (h *FileHandler) Options() any {
	return h.options
}

// Type returns the type of the handler.
func (h *FileHandler) Type() string {
	return FileHandlerType
}

// WithAttrs returns a new handler whose attributes consist of both the current object's attributes and the
// given attributes.
func (h *FileHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	clone := h.clone()
	clone.handler = h.handler.WithAttrs(attrs)
	return clone
}

// WithGroup returns a new handler with the existing object's attributes part of the given group.
func (h *FileHandler) WithGroup(name string) slog.Handler {
	if len(name) == 0 {
		return h
	}

	clone := h.clone()
	clone.handler = h.handler.WithGroup(name)
	return clone
}

// clone creates a copy of current handler.
func (h *FileHandler) clone() *FileHandler {
	return &FileHandler{
		bufferedWriter: h.bufferedWriter,
		fileWriter:     h.fileWriter,
		handler:        h.handler,
		options:        h.options,
	}
}

// createDefaultLogFile attempts to open the default log file for writing.
//
// This function may return an error with any of the following codes:
//   - [xlog.OptionsValidationError]: log file could not be opened for writing
func createDefaultLogFile(path types.Path) (string, xerrors.Error) {
	// get the base name of the executable to use as the default log file name
	var filename string
	exe, err := os.Executable()
	if err != nil {
		filename = DefaultFileHandlerFileName
	} else {
		actualPath, err := filepath.EvalSymlinks(exe)
		if err != nil {
			filename = DefaultFileHandlerFileName
		}
		filename = strings.TrimSuffix(filepath.Base(actualPath), filepath.Ext(actualPath)) + ".log"
	}

	// attempt to create the default file
	var xerr xerrors.Error
	var file *os.File
	for _, p := range DefaultFileHandlerLogFolders {
		path.FSPath = os.ExpandEnv(filepath.Join(p, filename))
		file, xerr = path.OpenFile(os.O_CREATE | os.O_APPEND | os.O_RDWR)
		if xerr == nil {
			file.Close()
			return path.FSPath, nil
		}
		xerr = xerrors.Wrapf(xlog.OptionsValidationError, xerr, "failed to open log file '%s' for writing: %s",
			path.FSPath, xerr.Error()).WithAttr("log_file", path.FSPath)
	}
	return "", xerr
}

// createLogFile attempts to open the given log file for writing.
//
// This function may return an error with any of the following codes:
//   - [xlog.OptionsValidationError]: the log file could not be opened for writing
func createLogFile(path types.Path) (string, xerrors.Error) {
	path.FSPath = os.ExpandEnv(path.FSPath)
	if path.FSPath == "" {
		return createDefaultLogFile(path)
	}

	file, xerr := path.OpenFile(os.O_CREATE | os.O_APPEND | os.O_RDWR)
	if xerr != nil {
		xerr = xerrors.Wrapf(xlog.OptionsValidationError, xerr, "failed to open log file '%s' for writing: %s",
			path.FSPath, xerr.Error()).WithAttr("log_file", path.FSPath)
		return "", xerr
	}
	file.Close()
	return path.FSPath, nil
}

// fileHandlerBuilder is used to build the handler from configuration options.
type fileHandlerBuilder struct {
	// unexported variables
	options FileHandlerOptions // handler options
}

// NewFileHandlerBuilderFromConfig creates a new [HandlerBuilder] and validates the given options, setting
// and default values as necessary.
//
// This function may return an error with any of the following codes:
//   - [xlog.MarshalError]: error while unmarshaling options to JSON
func NewFileHandlerBuilderFromConfig(options json.RawMessage) (xlog.HandlerBuilder, xerrors.Error) {
	var opts FileHandlerOptions
	if err := json.Unmarshal(options, &opts); err != nil {
		return nil, xerrors.Wrapf(xlog.MarshalError, err, "failed to unmarshal handler options: %s",
			err.Error()).WithAttr("options", string(options))
	}

	return &fileHandlerBuilder{
		options: opts,
	}, nil
}

// Build actually creates and returns the handler.
//
// This function may return an error with any of the following codes:
//   - [xlog.BuildHandlerError]: failed to construct the new handler
//
// This function may return other errors if the callback function fails and defines its own error values.
func (b *fileHandlerBuilder) Build(cb xlog.BuildHandlerCallbackFn) (slog.Handler, xerrors.Error) {
	if cb != nil {
		if err := cb(b.Type(), &b.options); err != nil {
			return nil, err
		}
	}
	h, err := NewFileHandler(b.options)
	if err != nil {
		return nil, xerrors.Wrapf(xlog.BuildHandlerError, err, "failed to build '%s' handler: %s", b.Type(),
			err.Error())
	}
	return h, nil
}

// MarshalJSON overrides how the object is marshalled to JSON to alter how field values are presented or to
// add additional fields.
func (b *fileHandlerBuilder) MarshalJSON() ([]byte, error) {
	return json.Marshal(b.options)
}

// Options returns the options as a string map.
func (b *fileHandlerBuilder) Options() map[string]any {
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
func (b *fileHandlerBuilder) Type() string {
	return FileHandlerType
}
