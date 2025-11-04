package xlog

const (
	// InvalidParameter indicates that an invalid value or type was passed as a parameter to a function.
	InvalidParameter = 1

	// HandleRecordError indicates there was a general error when an [slog.Handler.Handle] function was called that
	// resulted in the record not being logged.
	//
	// References:
	//   https://pkg.go.dev/log/slog#Handler.Handle
	HandleRecordError = 2

	// BuildHandlerError indicates that an error occurred when an [slog.Handler] was in the process of being built by
	// a [HandlerBuilder].
	//
	// References:
	//   https://pkg.go.dev/log/slog#Handler
	//   https://pkg.go.dev/go.innotegrity.dev/xlog#HandlerBuilder
	BuildHandlerError = 3

	// HandlerOptionDoesNotExist indicates that the given option for a handler does not exist.
	HandlerOptionDoesNotExist = 4

	// HandlerOptionIsNotSettable indicates that the given option for a handler exists but is not exported and
	// therefore cannot have its value modified.
	HandlerOptionIsNotSettable = 5

	// HandlerOptionDoesNotSupportNil indicates that the given option for a handler exists but a nil value was passed
	// and the option does not support being assigned a nil value.
	HandlerOptionDoesNotSupportNil = 6

	// HandlerOptionValueIsIncompatible indicates that the given option for a handler exists but a value was passed
	// which is not compatible with the underlying option's variable type.
	HandlerOptionValueIncompatible = 7

	// HandlerOptionIsNotGettable indicates that the given option for a handler exists but is not exported and
	// therefore cannot have its value retrieved as an 'any' (interface) type.
	HandlerOptionIsNotGettable = 8

	// MarshalError indicates there was an error marshalling data to JSON or unmarshalling data from JSON.
	MarshalError = 9

	// UnsupportedHandlerType indicates that an unsupported type of handler was requested to be created.
	UnsupportedHandlerType = 10

	// OptionsValidationError indicates that one or more options or values for a handler are invalid.
	OptionsValidationError = 11

	// HandlerTypeExists indicates that the handler type already exists but is trying to be registered again.
	HandlerTypeExists = 12

	// DataCompressionError indicates that there was an error compressing data.
	DataCompressionError = 13

	// HTTPClientError indicates that there was a general error with an HTTP client transmission.
	HTTPClientError = 14

	// HTTPRequestError indicates that there was an error specifically with an HTTP request.
	HTTPRequestError = 15

	// HTTPResponseError indicates that there was an error specifically with an HTTP response.
	HTTPResponseError = 16
)
