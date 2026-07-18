package logger

import (
	"fmt"
	"strings"
	"time"
)

// ANSI escape codes for terminal formatting.
const (
	Reset = "\x1b[0m"
	Bold  = "\x1b[1m"
	Dim   = "\x1b[2m"

	Red     = "\x1b[31m"
	Green   = "\x1b[32m"
	Yellow  = "\x1b[33m"
	Blue    = "\x1b[34m"
	Magenta = "\x1b[35m"
	Cyan    = "\x1b[36m"
	White   = "\x1b[37m"

	BrightRed     = "\x1b[91m"
	BrightGreen   = "\x1b[92m"
	BrightYellow  = "\x1b[93m"
	BrightBlue    = "\x1b[94m"
	BrightMagenta = "\x1b[95m"
	BrightCyan    = "\x1b[96m"
	BrightWhite   = "\x1b[97m"
)

// ts returns the current timestamp as HH:MM:SS.
func ts() string { return time.Now().Format("15:04:05") }

// logLine prints a formatted log line with timestamp, icon, level, and message.
// color: ANSI color code. icon: status icon. level: log level label. msg: message text.
func logLine(color, icon, level, msg string) {
	t := ts()
	lbl := fmt.Sprintf(" %s %-4s", icon, level)
	fmt.Printf("%s %s%s %s\n", Dim+t+Reset, color+Bold+lbl+Reset, Dim+"│"+Reset, color+msg+Reset)
}

// Info prints an informational log line with a blue INFO label.
// format: printf-style format string. args: format arguments.
func Info(format string, args ...interface{}) {
	logLine(BrightBlue, "ℹ", "INFO", fmt.Sprintf(format, args...))
}

// Ok prints a success log line with a green OK label.
// format: printf-style format string. args: format arguments.
func Ok(format string, args ...interface{}) {
	logLine(BrightGreen, "✓", "OK", fmt.Sprintf(format, args...))
}

// Warn prints a warning log line with a yellow WARN label.
// format: printf-style format string. args: format arguments.
func Warn(format string, args ...interface{}) {
	logLine(BrightYellow, "◆", "WARN", fmt.Sprintf(format, args...))
}

// Fail prints an error log line with a red FAIL label.
// format: printf-style format string. args: format arguments.
func Fail(format string, args ...interface{}) {
	logLine(BrightRed, "✗", "FAIL", fmt.Sprintf(format, args...))
}

// Section prints a bold magenta section header with decorative bars.
// title: section title text.
func Section(title string) {
	fmt.Printf("\n%s%s%s %s %s\n", Bold+BrightMagenta, "━━━", title, "━━━", Reset)
}

// Divider prints a dim horizontal line of 50 characters.
func Divider() {
	fmt.Println(Dim + strings.Repeat("─", 50) + Reset)
}

// Banner prints a titled block with optional content rows.
// title: banner title. rows: lines of content to display below the title.
func Banner(title string, rows ...string) {
	fmt.Printf("\n%s%s%s\n", Bold+BrightCyan, "  ■  "+title, Reset)
	for _, r := range rows {
		fmt.Printf("  %s\n", r)
	}
	fmt.Println()
}

// ShowBanner prints the ASCII art banner with the current version.
// version: version string to display (typically from build flags).
func ShowBanner(version string) {
	b := BrightBlue + Bold
	r := Reset
	fmt.Println()
	fmt.Printf("%s▄▄▄▄▄▄▄ ▄▄▄▄▄▄▄ ▄▄▄▄▄▄▄ ▄▄▄ ▄▄▄ ▄▄▄ ▄▄▄%s\n", b, r)
	fmt.Printf("%s█ ▄▄▄ █ █ ▄▄▄ █ █ ▄▄▄ █ █▄▀█▀▄█ █▄▀█▀▄█%s\n", b, r)
	fmt.Printf("%s█ ▄▄▄▄█ █ ▄ ▄▄█ █ █▄█ █ ▄█▀▄▀█▄  ▀█ █▀%s\n", b, r)
	fmt.Printf("%s█▄█     █▄█▄▄▄█ █▄▄▄▄▄█ █▄█▀█▄█   █▄█  %s\n", b, r)
	fmt.Printf("\n  %sv%s%s  |  %shttps://github.com/vanes430/raced_proxy%s\n\n", Gray, version, Reset, BrightBlue, Reset)
}

// Gray is the ANSI escape code for dim/gray text.
var Gray = "\x1b[90m"
