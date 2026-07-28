package agent

// Control protocol (Nezha/Komari-style): panel → agent commands, agent → results.
// All control requires an authenticated WebSocket session.

const (
	MsgTypeReport        = "report"
	MsgTypePing          = "ping"
	MsgTypePong          = "pong"
	MsgTypeAck           = "ack"
	MsgTypeConfig        = "config"
	MsgTypeError         = "error"
	MsgTypeCommand       = "command"
	MsgTypeCommandResult = "command_result"

	// Interactive PTY terminal (Nezha-style).
	MsgTypeTerminalOpen   = "terminal_open"
	MsgTypeTerminalOpened = "terminal_opened"
	MsgTypeTerminalInput  = "terminal_input"
	MsgTypeTerminalOutput = "terminal_output"
	MsgTypeTerminalResize = "terminal_resize"
	MsgTypeTerminalClose  = "terminal_close"
	MsgTypeTerminalClosed = "terminal_closed"
)

// Command types the panel may send to an online agent.
const (
	CmdReportNow       = "report_now"
	CmdSetInterval     = "set_interval"
	CmdRestartAgent    = "restart_agent"
	CmdRestartXray     = "restart_xray"
	CmdRestartSingBox  = "restart_singbox"
	CmdExec            = "exec"
	CmdPing            = "ping"
)

// Command is a panel → agent control request.
type Command struct {
	ID   string                 `json:"id"`
	Type string                 `json:"type"`
	Args map[string]interface{} `json:"args,omitempty"`
}

// CommandResult is an agent → panel response.
type CommandResult struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	OK      bool   `json:"ok"`
	Output  string `json:"output,omitempty"`
	Error   string `json:"error,omitempty"`
	Code    int    `json:"code,omitempty"`
	Elapsed int64  `json:"elapsed_ms,omitempty"`
}

// Wire envelope used on the agent WebSocket.
type WireEnvelope struct {
	Type    string          `json:"type"`
	Payload jsonRawMessage  `json:"payload,omitempty"`
	Time    int64           `json:"time,omitempty"`
	// Flattened fields for simple messages (config/ack/command).
	ID              string                 `json:"id,omitempty"`
	Command         string                 `json:"command,omitempty"`
	Args            map[string]interface{} `json:"args,omitempty"`
	ServerTime      int64                  `json:"server_time,omitempty"`
	IntervalSeconds int                    `json:"interval_seconds,omitempty"`
	Error           string                 `json:"error,omitempty"`
	OK              bool                   `json:"ok,omitempty"`
	Output          string                 `json:"output,omitempty"`
	Code            int                    `json:"code,omitempty"`
	Elapsed         int64                  `json:"elapsed_ms,omitempty"`
}

// jsonRawMessage avoids an import cycle with encoding/json aliases in docs.
type jsonRawMessage = []byte
