// SPDX-License-Identifier: GPL-3.0-only

package report

import "github.com/sayjavajava/rigfit/internal/score"

// ANSI codes for the four Status values (§7) plus a neutral reset. Modern
// Windows terminals (Windows Terminal, PowerShell, and conhost since
// roughly Windows 10 1511) interpret these natively; no VT100-enabling
// syscall dance is needed the way it was on older Windows console hosts.
const (
	ansiReset  = "\x1b[0m"
	ansiGreen  = "\x1b[32m"
	ansiYellow = "\x1b[33m"
	ansiRed    = "\x1b[31m"
	ansiGray   = "\x1b[90m"
)

// colorize wraps s in the ANSI code for status, or returns s unchanged
// when useColor is false (§11's --no-color flag, or the NO_COLOR env var
// convention — both handled by the caller before this is ever invoked).
func colorize(useColor bool, status score.Status, s string) string {
	if !useColor {
		return s
	}
	code := ansiGray
	switch status {
	case score.StatusPass:
		code = ansiGreen
	case score.StatusMarginal:
		code = ansiYellow
	case score.StatusFail:
		code = ansiRed
	}
	return code + s + ansiReset
}
