package handlers

import "go.innotegrity.dev/xlog"

var (
	_builders map[string]xlog.NewBuilderFromConfigFn
)

func init() {
	// register built-in handler builders
	_builders = map[string]xlog.NewBuilderFromConfigFn{
		ConsoleHandlerType:        NewConsoleHandlerBuilderFromConfig,
		DiscardHandlerType:        NewDiscardHandlerBuilderFromConfig,
		FanoutHandlerType:         NewFanoutHandlerBuilderFromConfig,
		FileHandlerType:           NewFileHandlerBuilderFromConfig,
		SentinelOneHECHandlerType: NewSentinelOneHECHandlerBuilderFromConfig,
	}
}
