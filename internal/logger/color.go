package logger

import (
	"fmt"
	"strings"
	"time"
)

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

func ts() string { return time.Now().Format("15:04:05") }

func logLine(color, icon, level, msg string) {
	t := ts()
	lbl := fmt.Sprintf(" %s %-4s", icon, level)
	fmt.Printf("%s %s%s %s\n", Dim+t+Reset, color+Bold+lbl+Reset, Dim+"│"+Reset, color+msg+Reset)
}

func Info(format string, args ...interface{}) {
	logLine(BrightBlue, "ℹ", "INFO", fmt.Sprintf(format, args...))
}

func Ok(format string, args ...interface{}) {
	logLine(BrightGreen, "✓", "OK", fmt.Sprintf(format, args...))
}

func Warn(format string, args ...interface{}) {
	logLine(BrightYellow, "◆", "WARN", fmt.Sprintf(format, args...))
}

func Fail(format string, args ...interface{}) {
	logLine(BrightRed, "✗", "FAIL", fmt.Sprintf(format, args...))
}

func Section(title string) {
	fmt.Printf("\n%s%s%s %s %s\n", Bold+BrightMagenta, "━━━", title, "━━━", Reset)
}

func Divider() {
	fmt.Println(Dim + strings.Repeat("─", 50) + Reset)
}

func Banner(title string, rows ...string) {
	fmt.Printf("\n%s%s%s\n", Bold+BrightCyan, "  ■  "+title, Reset)
	for _, r := range rows {
		fmt.Printf("  %s\n", r)
	}
	fmt.Println()
}

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

var Gray = "\x1b[90m"
