package aseprite

// CommandError retains process diagnostics for local debug use. The MCP boundary
// returns a fixed public message instead of exposing these streams.
type CommandError struct {
	Cause  error
	Stderr string
	Stdout string
}

func (e *CommandError) Error() string     { return e.Cause.Error() }
func (e *CommandError) Unwrap() error     { return e.Cause }
func (e *CommandError) ErrorCode() string { return "aseprite_execution_failed" }
