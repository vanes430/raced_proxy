package logger

import (
	"fmt"
	"strings"
	"time"
)

const (
	Reset   = "\x1b[0m"
	Bold    = "\x1b[1m"
	Dim     = "\x1b[2m"
	Red     = "\x1b[31m"
	Green   = "\x1b[32m"
	Yellow  = "\x1b[33m"
	Blue    = "\x1b[34m"
	Magenta = "\x1b[35m"
	Cyan    = "\x1b[36m"
	White   = "\x1b[37m"
	Gray    = "\x1b[90m"
	BgCyan  = "\x1b[46m"
)

func ts() string {
	return Gray + time.Now().Format("2006-01-02 15:04:05") + Reset
}

func Info(format string, args ...interface{}) {
	fmt.Printf("%s %sℹ%s %s\n", ts(), Cyan+Bold, Reset, White+fmt.Sprintf(format, args...)+Reset)
}

func Ok(format string, args ...interface{}) {
	fmt.Printf("%s %s✓%s %s\n", ts(), Green+Bold, Reset, Green+fmt.Sprintf(format, args...)+Reset)
}

func Warn(format string, args ...interface{}) {
	fmt.Printf("%s %s◆%s %s\n", ts(), Yellow+Bold, Reset, Yellow+fmt.Sprintf(format, args...)+Reset)
}

func Fail(format string, args ...interface{}) {
	fmt.Printf("%s %s✗%s %s\n", ts(), Red+Bold, Reset, Red+fmt.Sprintf(format, args...)+Reset)
}

func Banner(title string, rows ...string) {
	width := 50
	hr := Dim + strings.Repeat("─", width) + Reset
	fmt.Println()
	fmt.Printf("%s%s%s %s\n", BgCyan, Bold+White, "  "+title+"  ", Reset)
	fmt.Println(hr)
	for _, r := range rows {
		fmt.Printf("  %s\n", r)
	}
	fmt.Println(hr)
}

func ShowBanner(version string) {
	b := Blue + Bold
	r := Reset
	fmt.Println()
	fmt.Printf("%s▄▄▄▄▄▄▄ ▄▄▄▄▄▄▄ ▄▄▄▄▄▄▄ ▄▄▄ ▄▄▄ ▄▄▄ ▄▄▄%s\n", b, r)
	fmt.Printf("%s█ ▄▄▄ █ █ ▄▄▄ █ █ ▄▄▄ █ █▄▀█▀▄█ █▄▀█▀▄█%s\n", b, r)
	fmt.Printf("%s█ ▄▄▄▄█ █ ▄ ▄▄█ █ █▄█ █ ▄█▀▄▀█▄  ▀█ █▀%s\n", b, r)
	fmt.Printf("%s█▄█     █▄█▄▄▄█ █▄▄▄▄▄█ █▄█▀█▄█   █▄█  %s\n", b, r)
	fmt.Printf("\n  %sv%s%s  |  %shttps://github.com/vanes430/raced_proxy%s\n\n", Gray, version, Reset, Blue, Reset)
}

func Section(title string) {
	fmt.Printf("\n%s━━━ %s %s ━━━%s\n", Bold+Blue, title, Bold+Blue, Reset)
}

func Divider() {
	fmt.Println(Dim + strings.Repeat("─", 50) + Reset)
}
