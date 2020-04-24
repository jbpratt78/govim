package config

type Command string

type Function string

const (
	InternalFunctionPrefix = "_internal_"
	FunctionHello          = "Hello"
	// FunctionSetUserBusy is an internal function used by govim for indicated
	// whether the user is busy or not (based on cursor movement)
	FunctionSetUserBusy = InternalFunctionPrefix + "SetUserBusy"
	// FunctionBufChanged is an internal function used by govim for handling
	// delta-based changes in buffers.
	FunctionBufChanged = InternalFunctionPrefix + "BufChanged"
	// FunctionEnrichDelta is an internal function used by govim for enriching
	// listener_add based callbacks before calling FunctionBufChanged
	FunctionEnrichDelta = InternalFunctionPrefix + "EnrichDelta"
)

const (
	CommandStartSession Command = "StartSession"
	CommandEndSession   Command = "EndSession"
	CommandJoinSession  Command = "JoinSession"
	CommandLeaveSession Command = "LeaveSession"
)
