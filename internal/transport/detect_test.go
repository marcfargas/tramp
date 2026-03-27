package transport

import "testing"

func TestStripAnsi(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"no ansi", "hello", "hello"},
		{"color code", "\x1b[31mhello\x1b[0m", "hello"},
		{"cursor move", "\x1b[2Jhello", "hello"},
		{"osc sequence", "\x1b]0;title\x07hello", "hello"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StripAnsi(tt.input)
			if got != tt.expected {
				t.Errorf("StripAnsi(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestParseShellName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"bash", "bash"},
		{"/bin/bash", "bash"},
		{"-bash", "bash"},
		{"sh", "sh"},
		{"/bin/sh", "sh"},
		{"dash", "sh"},
		{"ash", "sh"},
		{"/bin/ash", "sh"},
		{"zsh", "bash"},
		{"/bin/zsh", "bash"},
		{"-zsh", "bash"},
		{"pwsh", "pwsh"},
		{"powershell", "pwsh"},
		{"cmd", "cmd"},
		{"cmd.exe", "cmd"},
		{"unknown_shell", "unknown"},
		{"\x1b[31mbash\x1b[0m", "bash"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ParseShellName(tt.input)
			if got != tt.expected {
				t.Errorf("ParseShellName(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestParsePlatform(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Linux", "linux"},
		{"Darwin", "darwin"},
		{"MINGW64_NT-10.0", "windows"},
		{"MSYS_NT-10.0", "windows"},
		{"CYGWIN_NT-10.0", "windows"},
		{"Windows_NT", "windows"},
		{"FreeBSD", "unknown"},
		{"\x1b[0mLinux", "linux"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ParsePlatform(tt.input)
			if got != tt.expected {
				t.Errorf("ParsePlatform(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestParseArch(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"x86_64", "x86_64"},
		{"aarch64", "aarch64"},
		{"arm64", "aarch64"},
		{"armv7l", "armv7l"},
		{"i686", "i686"},
		{"", "unknown"},
		{"\x1b[0mx86_64", "x86_64"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ParseArch(tt.input)
			if got != tt.expected {
				t.Errorf("ParseArch(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestParsePwshVersion(t *testing.T) {
	tests := []struct {
		input    string
		expected int
		ok       bool
	}{
		{"7", 7, true},
		{"5", 5, true},
		{"not a number", 0, false},
		{"", 0, false},
		{"\x1b[0m7", 7, true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, ok := ParsePwshVersion(tt.input)
			if ok != tt.ok || (ok && got != tt.expected) {
				t.Errorf("ParsePwshVersion(%q) = (%d, %v), want (%d, %v)",
					tt.input, got, ok, tt.expected, tt.ok)
			}
		})
	}
}
