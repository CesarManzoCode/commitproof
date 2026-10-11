// Package workload implements the workload clients W1–W4 (CONTRACT §7) and
// the closed completion classification of §6.2.
package workload

import (
	"strings"
)

// Completion types.
const (
	OK   = "ok"
	Fail = "fail"
	Info = "info"
)

// Phases of §6.1 err.phase.
const (
	PhaseConnect   = "connect"
	PhaseBegin     = "begin"
	PhaseStatement = "statement"
	PhaseCommit    = "commit"
	PhaseTimeout   = "client-timeout"
	PhaseTransport = "transport"
)

// Attempt is what the client knows about an operation when it ends. It is
// the only input of Classify, so every row of §6.2 can be tested without a
// server (K2.5).
type Attempt struct {
	// Err is nil on success.
	Err error
	// SQLState is the server's SQLSTATE ("" for transport errors and client
	// timeouts). Message is the server or transport error text.
	SQLState string
	Message  string
	// ClientTimeout: the P01 deadline expired. Transport: the connection
	// failed (no SQLSTATE).
	ClientTimeout bool
	Transport     bool
	// Where the error happened: connect, begin, statement or commit.
	At string
	// Autocommit: a single statement outside an explicit transaction.
	Autocommit bool
	// WriteSent: a write statement had been sent when the error happened
	// (including the failing statement). WriteExecuted: a write statement
	// completed successfully before the error. StatementExecuted: a data
	// statement (not BEGIN) completed successfully before the error.
	WriteSent         bool
	WriteExecuted     bool
	StatementExecuted bool
}

// noWritablePrimary matches Multigres' "no writable primary" condition: the
// MTF01 code or the gateway's diagnostic text (SQLSTATE 57P03, message "no
// writable primary is currently available",
// go/services/multigateway/poolergateway/availability_errors.go).
func noWritablePrimary(a Attempt) bool {
	return a.SQLState == "MTF01" || strings.Contains(a.Message, "no writable primary")
}

// Classify applies CONTRACT §6.2. A completion is ok only on explicit
// success; fail only if the error proves the operation did not take effect;
// otherwise info. The rows are checked in table order, with the one specific
// exception the table itself states (40001 returned by COMMIT is fail).
func Classify(a Attempt) string {
	if a.Err == nil {
		return OK // row 1 (notices, including the S61 WARNING, are recorded)
	}
	class := ""
	if len(a.SQLState) == 5 {
		class = a.SQLState[:2]
	}
	if a.SQLState != "" {
		if a.At == PhaseCommit {
			if a.SQLState == "40001" {
				return Fail // row 3
			}
			return Info // row 8: any error during or after sending COMMIT
		}
		switch {
		case class == "40":
			return Fail // row 2
		case class == "23":
			return Fail // row 4
		case a.SQLState == "0A000" || class == "42" || class == "22":
			if !a.WriteExecuted {
				return Fail // row 5
			}
			return Info // row 11
		case a.SQLState == "25006":
			return Fail // row 6
		case noWritablePrimary(a):
			if !a.StatementExecuted {
				return Fail // row 7
			}
			return Info // row 11
		case a.At == PhaseConnect:
			// The server refused the connection during startup (e.g. 57P03,
			// 28P01): no statement was sent.
			return Fail // row 10
		}
		return Info // row 11: any SQLSTATE not listed
	}
	// No SQLSTATE: transport failure or client timeout.
	switch {
	case a.At == PhaseConnect:
		return Fail // row 10: connection error before any statement was sent
	case a.At == PhaseCommit:
		return Info // row 8
	case a.ClientTimeout && a.WriteSent:
		return Info // row 9
	case a.Autocommit:
		return Info // row 12: transport error or timeout after sending
	}
	return Info // otherwise info (§6.2 preamble)
}

// ErrObject is the §6.1 err field.
func ErrObject(a Attempt) map[string]any {
	class, phase := "", a.At
	switch {
	case a.SQLState != "":
		class = a.SQLState
		if len(class) == 5 {
			class = class[:2]
		}
	case a.ClientTimeout:
		class, phase = "timeout", PhaseTimeout
	case a.At == PhaseConnect:
		class = "connect"
	default:
		class, phase = "transport", PhaseTransport
	}
	return map[string]any{"sqlstate": a.SQLState, "class": class, "phase": phase}
}
