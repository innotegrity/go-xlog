package handlers

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"go.innotegrity.dev/xlog"

	"go.innotegrity.dev/secretmgr/secrets"
	"go.innotegrity.dev/types"
	"go.innotegrity.dev/xerrors"
)

const (
	// SentinelOneHECHandlerType is the type for a [SentinelOneHECHandler].
	//
	// References:
	//   https://pkg.go.dev/go.innotegrity.dev/xlog/handlers#SentinelOneHECHandler
	SentinelOneHECHandlerType = "sentinelone:hec"
)

const (
	// sentinelOneHECIngestURL is the tokenized form of the ingestion URL for HEC.
	sentinelOneHECIngestURL = "https://%s/services/collector/event"
)

var (
	// DefaultSentinelOneHECHandlerCallerKey is the default name of the attribute for the source/caller information
	// to be stored within the "event" group when sending the event to the SentinelOne HTTP Event Collector.
	//
	// This value is used when the caller key in [SentinelOneHECHandlerOptions] is unset.
	//
	// Setting this value changes the default globally for the package.
	//
	// References:
	//   https://pkg.go.dev/go.innotegrity.dev/xlog/handlers#SentinelOneHECHandlerOptions
	DefaultSentinelOneHECHandlerCallerKey = "caller"

	// DefaultSentinelOneHECHandlerDSCategory is the value to use for dataSource.Category when sending the event
	// to the SentinelOne HTTP Event Collector.
	//
	// This value is used when the data source category in [SentinelOneHECHandlerOptions] is unset.
	//
	// Setting this value changes the default globally for the package.
	//
	// References:
	//   https://pkg.go.dev/go.innotegrity.dev/xlog/handlers#SentinelOneHECHandlerOptions
	DefaultSentinelOneHECHandlerDSCategory = "applog"

	// DefaultSentinelOneHECHandlerDSName is the value to use for dataSource.Name when sending the event
	// to the SentinelOne HTTP Event Collector.
	//
	// This value is used when the data source name in [SentinelOneHECHandlerOptions] is unset and the name of the
	// executable could not be retrieved.
	//
	// Setting this value changes the default globally for the package.
	//
	// References:
	//   https://pkg.go.dev/go.innotegrity.dev/xlog/handlers#SentinelOneHECHandlerOptions
	DefaultSentinelOneHECHandlerDSName = "unknown"

	// DefaultSentinelOneHECHandlerDSVendor is the value to use for dataSource.Vendor when sending the event
	// to the SentinelOne HTTP Event Collector.
	//
	// This value is used when the data source vendor in [SentinelOneHECHandlerOptions] is unset.
	//
	// Setting this value changes the default globally for the package.
	//
	// References:
	//   https://pkg.go.dev/go.innotegrity.dev/xlog/handlers#SentinelOneHECHandlerOptions
	DefaultSentinelOneHECHandlerDSCVendor = "Unknown"

	// DefaultSentinelOneHECHandlerHostname is the value to use for host when sending the event
	// to the SentinelOne HTTP Event Collector.
	//
	// This value is used when the hostname was not specified in [SentinelOneHECHandlerOptions] and could not be
	// retrieved.
	//
	// Setting this value changes the default globally for the package.
	//
	// References:
	//   https://pkg.go.dev/go.innotegrity.dev/xlog/handlers#SentinelOneHECHandlerOptions
	DefaultSentinelOneHECHandlerHostname = "unknown"

	// DefaultSentinelOneHECHandlerLogLevel is the default log level to use when one is not provided.
	//
	// This value is used when the level in [SentinelOneHECHandlerOptions] is unset.
	//
	// Setting this value changes the default globally for the package.
	//
	// References:
	//   https://pkg.go.dev/go.innotegrity.dev/xlog/handlers#SentinelOneHECHandlerOptions
	DefaultSentinelOneHECHandlerLogLevel = slog.LevelInfo

	// DefaultSentinelOneHECHandlerSendTimeout is the default duration to wait for an HTTP request to be sent
	// before the request times out.
	//
	// This value is used when the timeout in [SentinelOneHECHandlerOptions] is unset or is set to -1.
	//
	// Setting this value changes the default globally for the package.
	//
	// References:
	//   https://pkg.go.dev/go.innotegrity.dev/xlog/handlers#SentinelOneHECHandlerOptions
	DefaultSentinelOneHECHandlerSendTimeout = types.Duration(10 * time.Second)

	// DefaultSentinelOneHECHandlerSource is the value to use for source when sending the event
	// to the HTTP Event Collector.
	//
	// This value is used when the source was not specified in [SentinelOneHECHandlerOptions] and the name of the
	// executable could not be retrieved.
	//
	// Setting this value changes the default globally for the package.
	//
	// References:
	//   https://pkg.go.dev/go.innotegrity.dev/xlog/handlers#SentinelOneHECHandlerOptions
	DefaultSentinelOneHECHandlerSource = "unknown"
)

// DefaultSentinelOneHECLevelTranslator acts as a default translator which takes an [slog.Level] and translates it to
// an appropriate "severity" level when a message is logged to the SentinelOne HTTP Event Collector.
//
// This function translates the level as follows:
//   - message level > [slog.LevelError] = "critical"
//   - [slog.LevelError] >= message level > [slog.LevelWarn] = "error"
//   - [slog.LevelWarn] >= message level > [slog.LevelInfo] = "warning"
//   - [slog.LevelInfo] >= message level > [slog.LevelDebug] = "info"
//   - [slog.LevelDebug] >= message level > [slog.LevelDebug]-4 = "debug"
//   - [slog.LevelDebug]-4 >= message level > [slog.LevelDebug]-8 = "trace"
//   - [slog.LevelDebug]-8 >= message level = "finest"
func DefaultSentinelOneHECLevelTranslator(l slog.Level) string {
	if l > slog.LevelError {
		return "critical"
	} else if l > slog.LevelWarn {
		return "error"
	} else if l > slog.LevelInfo {
		return "warning"
	} else if l > slog.LevelDebug {
		return "info"
	} else if l > slog.LevelDebug-4 {
		return "debug"
	} else if l > slog.LevelDebug-8 {
		return "trace"
	}
	return "finest"
}

// SentinelOneHECHandlerOptions holds the options for a [SentinelOneHECHandler].
type SentinelOneHECHandlerOptions struct {
	// APIToken holds the URL to use to retrieve the API token for the SentinelOne HTTP Event Collector ingest API.
	//
	// This field is required.
	//
	// It supports the drivers supported by the [secretmgr.secrets.GenericSecret] type where the data in the generic
	// secret is the actual API token.
	//
	// If the secret is stored in a file using a relative path, the path is relative to the current working directory
	// for the application, not the configuration file.
	//
	// When reading configuration settings from a file or raw JSON, if this value is not present, it will be set
	// to an empty string.
	//
	// References:
	//   https://pkg.go.dev/go.innotegrity.dev/secretmgr/secrets#GenericSecret
	APIToken secrets.GenericSecret `json:"api_token"`

	// BufferSize indicates the size (in bytes) of the buffer to use before flushing records to the HTTP pipe.
	//
	// The default behavior is to disable buffering.
	//
	// When reading configuration settings from a file or raw JSON, if this value is not present, it will be set
	// to 0.
	BufferSize types.Size `json:"buffer_size"`

	// CallerKey is the name of the attribute for the source/caller information to be stored within the "event"
	// group when sending the event to the HTTP Event Collector.
	//
	// The default behavior is to use the default caller key defined in the package.
	//
	// When reading configuration settings from a file or raw JSON, if this value is not present, it will be set
	// to an empty string.
	CallerKey string `json:"caller_key"`

	// DisableAsync disables sending events asynchronously and forces everything to be sent synchronously over HTTP.
	//
	// Note that when the handler is being closed, it will always synchronously send any data remaining in the buffer.
	//
	// The default behavior is to always send data asynchronously over HTTP.
	//
	// When reading configuration settings from a file or raw JSON, if this value is not present, it will be set
	// to false.
	DisableAsync bool `json:"disable_async"`

	// DSCategory corresponds to the dataSource.Category value that will be sent to the HTTP event collector.
	//
	// The default behavior is to use the default category defined in the package.
	//
	// When reading configuration settings from a file or raw JSON, if this value is not present, it will be set
	// to an empty string.
	DSCategory string `json:"datasource_category"`

	// DSName corresponds to the dataSource.Name value that will be sent to the HTTP event collector.
	//
	// The default behavior is to use the name of the executable if it can be retrieved or the default name defined in
	// the package otherwise.
	//
	// When reading configuration settings from a file or raw JSON, if this value is not present, it will be set
	// to an empty string.
	DSName string `json:"datasource_name"`

	// DSVendor corresponds to the dataSource.Vendor value that will be sent to the HTTP event collector.
	//
	// The default behavior is to use the default vendor defined in the package.
	//
	// When reading configuration settings from a file or raw JSON, if this value is not present, it will be set
	// to an empty string.
	DSVendor string `json:"datasource_vendor"`

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

	// Fields holds the value of any additional fields to send in the 'fields' field to the HTTP event collector.
	//
	// 'fields' will not be populated if this value is nil or an empty map.
	//
	// The default behavior is to not populate any fields.
	//
	// When reading configuration settings from a file or raw JSON, if this value is not present, it will be set
	// to nil.
	Fields map[string]any `json:"fields"`

	// Host is the value to send for the 'host' field to the HTTP event collector.
	//
	// 'host' will not be populated if this value is an empty string.
	//
	// The default behavior is to use the name of the host if it can be retrieved or default hostname defined in the
	// package otherwise.
	//
	// When reading configuration settings from a file or raw JSON, if this value is not present, it will be set
	// to an empty string.
	Host string `json:"host"`

	// IncludeCaller indicates whether or not to include the caller in log messages.
	//
	// The default behavior is to not include caller information.
	//
	// When reading configuration settings from a file or raw JSON, if this value is not present, it will be set
	// to false.
	IncludeCaller bool `json:"include_caller"`

	// IngestHostname is the hostname to use in the SentinelOne HTTP event collector ingestion URL.
	//
	// This field is required.
	//
	// When reading configuration settings from a file or raw JSON, if this value is not present, it will be set
	// to an empty string.
	IngestHostname string `json:"ingest_hostname"`

	// Level is the minimum level at which to log messages.
	//
	// The default behavior is defined by the default level setting defined in the package.
	//
	// When reading configuration settings from a file or raw JSON, if this value is not present, it will be set
	// to nil.
	Level *slog.LevelVar `json:"level"`

	// LevelTranslator is a function that's called to translate a standard [slog.Level] into an appropriate "severity"
	// level for the SentinelOne HTTP Event Collector.
	//
	// It is passed the level of the record/message being logged and should return the corresponding "severity".
	//
	// The default behavior is to use the [DefaultSentinelOneHECLevelTranslator] function.
	//
	// When reading configuration settings from a file or raw JSON, create an [xlog.HandlerBuilder] and pass the
	// [xlog.HandlerBuilder.Build] function an [xlog.HandlerBuildCallbackFn] callback to modify the options and
	// set this value from your application, if desired.
	//
	// References:
	//	 https://pkg.go.dev/log/slog#Level
	//   https://pkg.go.dev/go.innotegrity.dev/xlog/handlers#DefaultSentinelOneHECLevelTranslator
	//   https://pkg.go.dev/go.innotegrity.dev/xlog#HandlerBuilder
	//   https://pkg.go.dev/go.innotegrity.dev/xlog#HandlerBuilder.Build
	//   https://pkg.go.dev/go.innotegrity.dev/xlog#HandlerBuilderBuildCallbackFn
	LevelTranslator func(slog.Level) string `json:"-"`

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

	// Scope is the SentinelOne scope that will be passed in the S1-Scope header.
	//
	// S1-Scope can contain the following:
	//   - AccountID only: S1-Scope: <AccountID>
	//	 - AccountID and SiteID: S1-Scope: <AccountID>:<SiteID>
	//   - AccountID, SiteID and GroupID: S1-Scope: <AccountID>:<SiteID>:<GroupID>
	//
	// This field is required.
	//
	// When reading configuration settings from a file or raw JSON, if this value is not present, it will be set
	// to an empty string.
	Scope string `json:"scope"`

	// SendTimeout is the duration to wait for an HTTP request to complete before timing out.
	//
	// Set this to 0 if you wish to disable timeouts.
	//
	// The default behavior is to wait the duration specified by the package default before timing out.
	//
	// When reading configuration settings from a file or raw JSON, if this value is not present, it will be set
	// to -1.
	SendTimeout types.Duration `json:"send_timeout"`

	// Source is the value to send for the 'source' field to the HTTP event collector.
	//
	// 'source' will not be populated if this value is an empty string.
	//
	// The default behavior is to use the name of the executable if it can be retrieved or the default source defined
	// in the package otherwise.
	//
	// When reading configuration settings from a file or raw JSON, if this value is not present, it will be set
	// to an empty string.
	Source string `json:"source"`
}

// jsonSentinelOneHECHandlerOptions is an alternate form of [SentinelOneHECHandlerOptions] that is used during
// unmarshalling to prevent infinite recursion.
type jsonSentinelOneHECHandlerOptions struct {
	APIToken       secrets.GenericSecret `json:"api_token"`
	BufferSize     types.Size            `json:"buffer_size"`
	CallerKey      string                `json:"caller_key"`
	DisableAsync   bool                  `json:"disable_async"`
	DSCategory     string                `json:"datasource_category"`
	DSName         string                `json:"datasource_name"`
	DSVendor       string                `json:"datasource_vendor"`
	Fields         map[string]any        `json:"fields"`
	Host           string                `json:"host"`
	IncludeCaller  bool                  `json:"include_caller"`
	IngestHostname string                `json:"ingest_hostname"`
	Level          string                `json:"level"`
	MaxLevel       string                `json:"max_level"`
	Scope          string                `json:"scope"`
	SendTimeout    *types.Duration       `json:"send_timeout"`
	Source         string                `json:"source"`
}

// UnmarshalJSON decodes the JSON-encoded data into the current object.
func (o *SentinelOneHECHandlerOptions) UnmarshalJSON(data []byte) error {
	var opts jsonSentinelOneHECHandlerOptions
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

	// validate the send timeout setting
	//
	// note that we purposely set it to -1 here if it's not set so that it can be set when the handler is created or
	// overridden by the calling application
	if opts.SendTimeout == nil {
		o.SendTimeout = -1
	} else {
		o.SendTimeout = *opts.SendTimeout
	}

	// copy remaining options
	o.APIToken = opts.APIToken
	o.BufferSize = opts.BufferSize
	o.CallerKey = opts.CallerKey
	o.DisableAsync = opts.DisableAsync
	o.DSCategory = opts.DSCategory
	o.DSName = opts.DSName
	o.DSVendor = opts.DSVendor
	o.Fields = opts.Fields
	o.Host = opts.Host
	o.IncludeCaller = opts.IncludeCaller
	o.IngestHostname = opts.IngestHostname
	o.Scope = opts.Scope
	o.Source = opts.Source

	return nil
}

// ensure [SentinelOneHECHandler] implements [xlog.ExtendedHandler] interface.
var _ xlog.ExtendedHandler = &SentinelOneHECHandler{}

// ensure [SentinelOneHECHandler] implements [xlog.LevelVarHandler] interface.
var _ xlog.LevelVarHandler = &SentinelOneHECHandler{}

// SentinelOneHECHandler is a handler that sends events to SentinelOne AI SIEM using its HTTP event collector.
type SentinelOneHECHandler struct {
	// unexported variables
	attrs        []slog.Attr                  // immuatable attributes for the handler
	authToken    string                       // authorization token
	client       *http.Client                 // HTTP client object
	groups       []string                     // immutable groups for the handler
	ingestionURL string                       // HEC ingestion URL
	options      SentinelOneHECHandlerOptions // handler options
	state        *sentinelOneHECHandlerState  // shared buffer and mutex
}

// sentinelOneHECHandlerState holds the shared, mutable state for a handler and its descendants. This includes the
// buffer and the mutex protecting it.
type sentinelOneHECHandlerState struct {
	mu  sync.Mutex
	buf *bytes.Buffer
}

// NewSentinelOneHECHandler creates a new [SentinelOneHECHandler] object with the given options.
//
// This function may return an error with any of the following codes:
//   - [xlog.OptionsValidationError]: one or more options are invalid
func NewSentinelOneHECHandler(options SentinelOneHECHandlerOptions) (*SentinelOneHECHandler, xerrors.Error) {
	h := &SentinelOneHECHandler{
		client:  &http.Client{},
		options: options,
		state: &sentinelOneHECHandlerState{
			buf: &bytes.Buffer{},
		},
	}

	// API token, ingest hostname and scope are required fields
	if len(h.options.APIToken.Data) == 0 {
		return nil, xerrors.New(xlog.OptionsValidationError, "api_token is a required setting")
	}
	if h.options.IngestHostname == "" {
		return nil, xerrors.New(xlog.OptionsValidationError, "ingest_hostname is a required setting")
	}
	if h.options.Scope == "" {
		return nil, xerrors.New(xlog.OptionsValidationError, "scope is a required setting")
	}
	h.ingestionURL = fmt.Sprintf(sentinelOneHECIngestURL, h.options.IngestHostname)
	h.authToken = fmt.Sprintf("Bearer %s", h.options.APIToken.Data)

	// ensure a minimum level is set
	if h.options.Level == nil {
		var level slog.LevelVar
		level.Set(DefaultConsoleHandlerLogLevel)
		h.options.Level = &level
	}

	// get the EXE name
	exe, err := os.Executable()
	if err == nil {
		realPath, err := filepath.EvalSymlinks(exe)
		if err == nil {
			exe = strings.TrimSuffix(filepath.Base(realPath), filepath.Ext(realPath))
		}
	}

	// validate data source fields
	if h.options.DSCategory == "" {
		h.options.DSCategory = DefaultSentinelOneHECHandlerDSCategory
	}
	if h.options.DSName == "" {
		if exe != "" {
			h.options.DSName = exe
		} else {
			h.options.DSName = DefaultSentinelOneHECHandlerDSName
		}
	}
	if h.options.DSVendor == "" {
		h.options.DSVendor = DefaultSentinelOneHECHandlerDSCVendor
	}

	// validate other defaults
	if h.options.CallerKey == "" {
		h.options.CallerKey = DefaultSentinelOneHECHandlerCallerKey
	}

	if h.options.Host == "" {
		hostname, err := os.Hostname()
		if err != nil {
			h.options.Host = DefaultSentinelOneHECHandlerHostname
		} else {
			h.options.Host = hostname
		}
	}

	if h.options.SendTimeout == -1 {
		h.options.SendTimeout = DefaultSentinelOneHECHandlerSendTimeout
	}
	if h.options.SendTimeout > 0 {
		h.client.Timeout = time.Duration(h.options.SendTimeout)
	}

	if h.options.Source == "" {
		if exe != "" {
			h.options.Source = exe
		} else {
			h.options.Source = DefaultSentinelOneHECHandlerSource
		}
	}

	return h, nil
}

// ChildHandlers will always return nil as this handler has no child handlers.
func (h *SentinelOneHECHandler) ChildHandlers() []slog.Handler {
	return nil
}

// Close synchronously flushes any data in the buffer to the HTTP event collector.
func (h *SentinelOneHECHandler) Close() error {
	h.state.mu.Lock()

	// nothing in the buffer to flush
	if h.state.buf.Len() == 0 {
		h.state.mu.Unlock()
		return nil
	}

	// send the remaining buffer content synchronously to ensure everything has been sent
	payload := make([]byte, h.state.buf.Len())
	copy(payload, h.state.buf.Bytes())
	h.state.buf.Reset()
	h.state.mu.Unlock()
	h.send(context.Background(), nil, payload)
	return nil
}

// Enabled returns true if the handler should handle the message or false if it should not.
func (h *SentinelOneHECHandler) Enabled(ctx context.Context, level slog.Level) bool {
	handlerLevel := h.options.Level.Level()
	if h.options.MaxLevel == nil {
		return level >= handlerLevel
	}
	return level >= handlerLevel && level <= handlerLevel
}

// GetLevelVar returns the handler's [slog.LevelVar] for manipulating the minimum logging level.
func (h *SentinelOneHECHandler) GetLevelVar() *slog.LevelVar {
	return h.options.Level
}

// GetMaxLevelVar returns the handler's [slog.LevelVar] for manipulating the maximum logging level.
func (h *SentinelOneHECHandler) GetMaxLevelVar() *slog.LevelVar {
	return h.options.MaxLevel
}

// Handle processes the record and handles logging it.
func (h *SentinelOneHECHandler) Handle(ctx context.Context, r slog.Record) error {
	// create a *local* buffer to avoid holding the global lock during JSON formatting
	recordBuf := &bytes.Buffer{}

	// create a temporary JSONHandler that writes to our *local* buffer.
	tempHandler := slog.Handler(slog.NewJSONHandler(recordBuf, &slog.HandlerOptions{
		AddSource: false, // don't need the caller here
		Level:     h.options.Level,
		ReplaceAttr: func(groups []string, attr slog.Attr) slog.Attr {
			numGroups := len(groups)

			// call the user-defined ReplaceAttr() function if it's set
			if h.options.ReplaceAttr != nil {
				attr = h.options.ReplaceAttr(groups, attr)
			}

			// make sure the "time" key is set to seconds since the epoch
			if numGroups == 0 && attr.Key == slog.TimeKey && attr.Value.Kind() == slog.KindTime {
				attr.Key = "time"
				attr.Value = slog.Int64Value(attr.Value.Time().UnixMilli())
			}

			// remove the top-level "time", "level" and "msg" keys
			if numGroups == 0 && (attr.Key == slog.LevelKey || attr.Key == slog.MessageKey) {
				return slog.Attr{}
			}
			return attr
		},
	}))
	if h.attrs != nil {
		tempHandler = tempHandler.WithAttrs(h.attrs)
	}
	if h.groups != nil {
		for _, group := range h.groups {
			tempHandler = tempHandler.WithGroup(group)
		}
	}

	// copy all of the record's attributes so they can be added to a new record under an "event" group
	extraAttrs := 2
	if h.options.IncludeCaller {
		extraAttrs++
	}
	eventAttrs := make([]slog.Attr, 0, r.NumAttrs()+extraAttrs)
	r.Attrs(func(attr slog.Attr) bool {
		eventAttrs = append(eventAttrs, attr)
		return true
	})

	// add the message to the "event" group
	eventAttrs = append(eventAttrs, slog.String("message", r.Message))

	// add the time to the "event" group
	//eventAttrs = append(eventAttrs, slog.Time("time", r.Time))

	// rename event.level to event.severity and modify value
	var severity string
	if h.options.LevelTranslator != nil {
		severity = h.options.LevelTranslator(r.Level)
	} else {
		severity = DefaultSentinelOneHECLevelTranslator(r.Level)
	}
	eventAttrs = append(eventAttrs, slog.String("severity", severity))

	// add the caller info if desired
	if h.options.IncludeCaller && r.PC != 0 {
		fs := runtime.CallersFrames([]uintptr{r.PC})
		f, _ := fs.Next()
		eventAttrs = append(eventAttrs, slog.Any(h.options.CallerKey, &slog.Source{
			Function: f.Function,
			File:     f.File,
			Line:     f.Line,
		}))
	}

	// add dataSource fields
	eventAttrs = append(eventAttrs, slog.Group("dataSource",
		slog.String("category", h.options.DSCategory),
		slog.String("name", h.options.DSName),
		slog.String("vendor", h.options.DSVendor),
	))

	// create the new record with the "event" group
	record := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
	if len(eventAttrs) > 0 {
		record.AddAttrs(slog.GroupAttrs("event", eventAttrs...))
	}

	// add host, source, sourcetype, and site.id fields
	record.AddAttrs(
		slog.String("host", h.options.Host),
		slog.String("source", h.options.Source),
		slog.String("sourcetype", "gron"),
	)

	// let the temporary handler format the record into our *local* buffer
	if err := tempHandler.Handle(ctx, record); err != nil {
		return h.handleError(ctx, fmt.Errorf(
			"failed to format log record to send to SentinelOne HTTP event collector: %w", err), &record)
	}

	// add a newline to separate log entries (NDJSON format)
	recordBuf.WriteByte('\n')

	// lock the shared buffer
	h.state.mu.Lock()
	defer h.state.mu.Unlock()

	// check if the buffer is full *after* adding this new record
	//
	// We check if the buffer *already has data* before checking size. This ensures a single log larger than the max
	// size is still processed.
	var payload []byte
	if h.state.buf.Len() > 0 && (h.options.BufferSize == 0 ||
		(types.Size(h.state.buf.Len()+recordBuf.Len()) > h.options.BufferSize)) {

		// buffer is full (or disabled) -- prepare to send the *current* buffer contents
		payload = make([]byte, h.state.buf.Len())
		copy(payload, h.state.buf.Bytes())
		h.state.buf.Reset()
	}

	// write the new record to the (possibly empty) buffer
	if _, err := h.state.buf.Write(recordBuf.Bytes()); err != nil {
		return h.handleError(ctx, fmt.Errorf(
			"failed to write to buffer for SentinelOne HTTP event collector: %w\n", err), &record)
	}

	// send the payload if one was created
	if payload != nil {
		if h.options.DisableAsync {
			return h.send(ctx, &record, payload)
		}
		go h.send(ctx, &record, payload)
	}
	return nil
}

// Options returns the handler's options.
func (h *SentinelOneHECHandler) Options() any {
	return h.options
}

// Type returns the type of the handler.
func (h *SentinelOneHECHandler) Type() string {
	return SentinelOneHECHandlerType
}

// WithAttrs returns a new handler whose attributes consist of both the current object's attributes and the
// given attributes.
func (h *SentinelOneHECHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	clone := h.clone()
	newAttrs := make([]slog.Attr, len(h.attrs)+len(attrs))
	copy(newAttrs, h.attrs)
	copy(newAttrs[len(h.attrs):], attrs)
	clone.attrs = newAttrs
	return clone
}

// WithGroup returns a new handler with the existing object's attributes part of the given group.
func (h *SentinelOneHECHandler) WithGroup(name string) slog.Handler {
	if len(name) == 0 {
		return h
	}

	clone := h.clone()
	newGroups := make([]string, len(h.groups)+1)
	copy(newGroups, h.groups)
	newGroups[len(h.groups)] = name
	clone.groups = newGroups
	return clone
}

// clone creates a copy of current handler.
func (h *SentinelOneHECHandler) clone() *SentinelOneHECHandler {
	return &SentinelOneHECHandler{
		attrs:        slices.Clone(h.attrs),
		authToken:    h.authToken,
		client:       h.client,
		groups:       slices.Clone(h.groups),
		ingestionURL: h.ingestionURL,
		options:      h.options,
		state:        h.state,
	}
}

// handleError is a simple wrapper function to call the error handler function if it is defined.
func (h *SentinelOneHECHandler) handleError(ctx context.Context, err error, r *slog.Record) error {
	if h.options.ErrorHandler != nil {
		err = h.options.ErrorHandler(ctx, err, r)
	}
	return err
}

// send actually sends the HTTP POST request to the SentinelOne Event Collector.
//
// This function may return an error with any of the following codes:
//   - [xlog.DataCompressionError]: failed to gzip the payload
//   - [xlog.HTTPClientError]: failed to send the HTTP request
//   - [xlog.HTTPRequestError]: failed to construct the HTTP request
//   - [xlog.HTTPResponseError]: failed to process the HTTP response
//
// It is possible that the function may return other errors if the handler's [ErrorHandler] modifies the
// error passed to it in any way.
func (h *SentinelOneHECHandler) send(ctx context.Context, r *slog.Record, payload []byte) error {
	// gzip the payload
	var gzipBuf bytes.Buffer
	gw := gzip.NewWriter(&gzipBuf)
	if _, err := gw.Write(payload); err != nil {
		return h.handleError(ctx, xerrors.Wrapf(xlog.DataCompressionError, err, "failed to compress payload: %s",
			err.Error()), r)
	}
	if err := gw.Close(); err != nil {
		return h.handleError(ctx, xerrors.Wrapf(xlog.DataCompressionError, err, "failed to close gzip writer: %s",
			err.Error()), r)
	}

	// construct the request
	req, err := http.NewRequest("POST", h.ingestionURL, &gzipBuf)
	if err != nil {
		return h.handleError(ctx, xerrors.Wrapf(xlog.HTTPRequestError, err, "failed to create HTTP request: %s",
			err.Error()), r)
	}
	req.Header.Set("Authorization", h.authToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Encoding", "gzip")
	req.Header.Set("S1-Scope", h.options.Scope)

	// execute the request
	resp, err := h.client.Do(req)
	if err != nil {
		return h.handleError(ctx, xerrors.Wrapf(xlog.HTTPClientError, err, "failed to execute HTTP request: %s",
			err.Error()), r)
	}
	defer resp.Body.Close()

	// ensure an error did not occur
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return h.handleError(ctx, xerrors.Newf(xlog.HTTPResponseError,
			"log endpoint returned non-OK status: %s, body: %s\n", resp.Status, string(body)).WithAttrs(
			map[string]any{
				"status_code": resp.StatusCode,
				"status":      resp.Status,
				"body":        string(body),
			}), r)
	}
	return nil
}

// sentinelOneHECHandlerBuilder is used to build the handler from configuration options.
type sentinelOneHECHandlerBuilder struct {
	// unexported variables
	options SentinelOneHECHandlerOptions // handler options
}

// NewSentinelOneHECHandlerBuilderFromConfig creates a new [xlog.HandlerBuilder] and validates the given options,
// setting and default values as necessary.
//
// This function may return an error with any of the following codes:
//   - [xlog.MarshalError]: error while unmarshaling options to JSON
func NewSentinelOneHECHandlerBuilderFromConfig(options json.RawMessage) (xlog.HandlerBuilder, xerrors.Error) {
	var opts SentinelOneHECHandlerOptions
	if err := json.Unmarshal(options, &opts); err != nil {
		return nil, xerrors.Wrapf(xlog.MarshalError, err, "failed to unmarshal handler options: %s",
			err.Error()).WithAttr("options", string(options))
	}

	return &sentinelOneHECHandlerBuilder{
		options: opts,
	}, nil
}

// Build actually creates and returns the handler.
//
// This function may return an error with any of the following codes:
//   - [xlog.BuildHandlerError]: failed to construct the new handler
//
// This function may return other errors if the callback function fails and defines its own error values.
func (b *sentinelOneHECHandlerBuilder) Build(cb xlog.BuildHandlerCallbackFn) (slog.Handler, xerrors.Error) {
	if cb != nil {
		if err := cb(b.Type(), &b.options); err != nil {
			return nil, err
		}
	}
	h, err := NewSentinelOneHECHandler(b.options)
	if err != nil {
		return nil, xerrors.Wrapf(xlog.BuildHandlerError, err, "failed to build '%s' handler: %s", b.Type(),
			err.Error())
	}
	return h, nil
}

// MarshalJSON overrides how the object is marshalled to JSON to alter how field values are presented or to
// add additional fields.
func (b *sentinelOneHECHandlerBuilder) MarshalJSON() ([]byte, error) {
	return json.Marshal(b.options)
}

// Options returns the options as a string map.
func (b *sentinelOneHECHandlerBuilder) Options() map[string]any {
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
func (b *sentinelOneHECHandlerBuilder) Type() string {
	return SentinelOneHECHandlerType
}
