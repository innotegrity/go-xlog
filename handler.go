package xlog

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"reflect"

	"go.innotegrity.dev/xerrors"
)

var (
	// DefaultErrorHandlerWriter is the [io.Writer] that will be used to write any error messages to if the
	// [DefaultErrorHandler] function is used for any of the handlers.
	//
	// References:
	//   https://pkg.go.dev/io#Writer
	//   https://pkg.go.dev/go.innotegrity.dev/xlog#DefaultErrorHandler
	DefaultErrorHandlerWriter io.Writer = os.Stderr
)

// ErrorHandlerFn is a function that's called to process any internal errors that may occur when a message is
// processed by a handler.
//
// It is passed the context and original record being handled along with the error that occurred.  It can return
// the same error back or modify the error, which will subsequently be returned by the [slog.Handler.Handle] function.
//
// Note that in some cases (eg: when a handler is closed and a buffer is flushed), the original record being logged
// may not be available, so it will be nil.
//
// Typically err should never be nil, but your function should also check to make sure that it is not nil.
//
// You should also not modify the passed record in any way.  If you need to make changes to it, use the
// [slog.Record.Clone] function to clone it first.
type ErrorHandlerFn func(ctx context.Context, err error, r *slog.Record) error

// ExtendedHandler defines the interface for a handler with extended functionality that is useful when creating
// handlers from configuration files.
type ExtendedHandler interface {
	slog.Handler

	// ChildHandlers returns any child handler(s) for the handler.
	//
	// This function should return nil if the handler has no child handlers.
	ChildHandlers() []slog.Handler

	// Options should return the configured handler-specific options.
	Options() any

	// Type should return the type of the handler.
	Type() string
}

// LevelHandler defines the interface for a handler that allows you to retrieve underlying [slog.LevelVar] objects
// in the handler which is when building handlers from configuration files.
type LevelVarHandler interface {
	// GetLevelVar should return the [slog.LevelVar] object for manipulating the current minimum logging level.
	//
	// References:
	//   https://pkg.go.dev/log/slog#LevelVar
	GetLevelVar() *slog.LevelVar

	// GetMaxLevelVar should return the [slog.LevelVar] object for manipulating the current maximum logging level.
	//
	// This function should return nil if the handler has no support for a maximum level.
	//
	// References:
	//   https://pkg.go.dev/log/slog#LevelVar
	GetMaxLevelVar() *slog.LevelVar
}

// DefaultErrorHandler can be used as a default error handler for any of the handlers supported by this package.
//
// It will simply wrap the error in an [xerrors.Error] object and add the record's details as attributes to the error
// and print the error to [os.Stderr], returning the new error object.
//
// This function will always return a [HandleRecordError] error.
func DefaultErrorHandler(ctx context.Context, err error, r *slog.Record) error {
	output := map[string]any{}

	// get the record details
	record := RecordToMap(r)
	if len(record) > 0 {
		output["record"] = record
	}

	// get the error details
	errMap := map[string]any{}
	var xerr xerrors.Error
	if err != nil {
		errMap["message"] = fmt.Sprintf("failed to write record: %s", err.Error())
		errMap["code"] = HandleRecordError
		errMap["error"] = err
		xerr = xerrors.Wrapf(HandleRecordError, err, "failed to write record: %s", err.Error())
	} else {
		msg := "an unexpected error occurred while writing the record"
		errMap["message"] = msg
		errMap["code"] = HandleRecordError
		xerr = xerrors.New(HandleRecordError, msg)
	}
	if len(errMap) > 0 {
		output["error"] = errMap
	}

	// print the error to the writer
	if DefaultErrorHandlerWriter == nil {
		DefaultErrorHandlerWriter = io.Discard
	}
	if o, err := json.Marshal(output); err == nil {
		fmt.Fprintf(DefaultErrorHandlerWriter, "%s\n", string(o))
	} else {
		fmt.Fprintf(DefaultErrorHandlerWriter, "%+v\n", output)
	}
	return xerr.WithAttrs(output)
}

// GetHandlerOptionValue inspects the given options (which should be a struct or a pointer to a struct) to find an
// exported field with the given name. If the field exists and is exported, it returns the field's value.
//
// This function may return an error with any of the following codes:
//   - [HandlerOptionDoesNotExist]: given field (name) does not exist in the options
//   - [HandlerOptionIsNotGettable]: given field (name) cannot be retrieve because it is not exported
//   - [InvalidParameter]: options is not a pointer to a struct
func GetHandlerOptionValue(options any, name string) (any, xerrors.Error) {
	// check the options object passed
	objVal := reflect.ValueOf(options)
	if objVal.Kind() == reflect.Pointer {
		objVal = objVal.Elem()
	}
	if objVal.Kind() != reflect.Struct {
		return nil, xerrors.Newf(InvalidParameter,
			"options must be a struct or a pointer to a struct, but got %T", options)
	}

	// get the field from the struct by its name
	field := objVal.FieldByName(name)
	if !field.IsValid() {
		return nil, xerrors.Newf(HandlerOptionDoesNotExist, "%s: no such field exists in the options", name)
	}
	if !field.CanInterface() {
		return nil, xerrors.Newf(HandlerOptionIsNotGettable, "%s: field exists but is inaccessible", name)
	}
	return field.Interface(), nil
}

// New is just a wrapper to create a new [slog.Logger] object.
func New(h slog.Handler) *slog.Logger {
	return slog.New(h)
}

// OverrideHandlerOptionValue inspects the given options (which should be a pointer to a struct) to find a field with
// the given name. If the field exists, is settable, and the type of value is assignable to the field's type, it sets
// the field's value.
//
// This function may return an error with any of the following codes:
//   - [HandlerOptionDoesNotExist]: given field (name) does not exist in the options
//   - [HandlerOptionDoesNotSupportNil]: given field (name) does not support nil values but one was passed
//   - [HandlerOptionIsNotSettable]: given field (name) cannot be set because it is not exported
//   - [HandlerOptionValueIncompatible]: value given is not compatible with the field (name)
//   - [InvalidParameter]: options is not a pointer to a struct
func OverrideHandlerOptionValue(options any, name string, value any) xerrors.Error {
	// make sure options is a pointer to a struct
	objVal := reflect.ValueOf(options)
	if objVal.Kind() != reflect.Pointer {
		return xerrors.Newf(InvalidParameter, "options must be a pointer to a struct, but got %T",
			options)
	}
	structVal := objVal.Elem()
	if structVal.Kind() != reflect.Struct {
		return xerrors.Newf(InvalidParameter,
			"options must be a pointer to a struct, but got pointer to %v", structVal.Kind())
	}

	// check if the field is valid (exists) and can be set (is exported).
	field := structVal.FieldByName(name)
	if !field.IsValid() {
		return xerrors.Newf(HandlerOptionDoesNotExist, "%s: no such field exists in the options", name)
	}
	if !field.CanSet() {
		return xerrors.Newf(HandlerOptionIsNotSettable, "%s: field exists but is not settable", name)
	}

	// handle nil values
	fieldType := field.Type()
	valToSetVal := reflect.ValueOf(value)
	if !valToSetVal.IsValid() {
		// valueToSet is a raw 'nil' - we can only set 'nil' to types that support it (pointers, interfaces,
		// slices, etc.)
		fieldKind := fieldType.Kind()
		if fieldKind == reflect.Ptr || fieldKind == reflect.Interface ||
			fieldKind == reflect.Slice || fieldKind == reflect.Map ||
			fieldKind == reflect.Chan || fieldKind == reflect.Func {

			field.Set(reflect.Zero(fieldType))
			return nil
		}

		// field type does not support 'nil'
		return xerrors.Newf(HandlerOptionDoesNotSupportNil, "%s: field cannot be set to nil value", name)
	}

	// handle non-nil values - check if the value's type is assignable to the field's type
	// this is more flexible than checking for exact type equality (e.g., allows interfaces)
	valToSetType := valToSetVal.Type()
	if valToSetType.AssignableTo(fieldType) {
		field.Set(valToSetVal)
		return nil
	}

	// types are not compatible
	return xerrors.Newf(HandlerOptionValueIncompatible, "%s: value type '%s' is not compatible with field",
		name, valToSetType.String())
}
