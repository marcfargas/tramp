package shell

import "testing"

func TestBashEscape(t *testing.T) {
	d := NewBashDriver()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"empty string", "", "''"},
		{"simple alphanumeric", "hello", "hello"},
		{"path with slashes", "/usr/bin/ls", "/usr/bin/ls"},
		{"safe chars", "a.b-c_d/e=f:g@h", "a.b-c_d/e=f:g@h"},
		{"space needs quoting", "hello world", "'hello world'"},
		{"single quote", "it's", "'it'\"'\"'s'"},
		{"multiple single quotes", "a'b'c", "'a'\"'\"'b'\"'\"'c'"},
		{"special chars", "foo;bar", "'foo;bar'"},
		{"backtick", "foo`bar", "'foo`bar'"},
		{"dollar sign", "$HOME", "'$HOME'"},
		{"newline", "line1\nline2", "'line1\nline2'"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := d.Escape(tt.input)
			if got != tt.expected {
				t.Errorf("Escape(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestBashEscapeNullByte(t *testing.T) {
	d := NewBashDriver()
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on null byte")
		}
	}()
	d.Escape("hello\x00world")
}

func TestBashStatCommand(t *testing.T) {
	d := NewBashDriver()
	got := d.StatCommand("/home/user/file.txt")
	expected := "if [ -f '/home/user/file.txt' ]; then echo file; elif [ -d '/home/user/file.txt' ]; then echo directory; elif [ -e '/home/user/file.txt' ]; then echo other; else echo missing; fi"
	if got != expected {
		t.Errorf("StatCommand = %q, want %q", got, expected)
	}
}

func TestBashMkdirCommand(t *testing.T) {
	d := NewBashDriver()
	got := d.MkdirCommand("/home/user/new dir")
	expected := "mkdir -p '/home/user/new dir'"
	if got != expected {
		t.Errorf("MkdirCommand = %q, want %q", got, expected)
	}
}

func TestBashCwdWrap(t *testing.T) {
	d := NewBashDriver()
	got := d.CwdWrap("ls -la", "/home/user/project")
	expected := "cd '/home/user/project' && ls -la"
	if got != expected {
		t.Errorf("CwdWrap = %q, want %q", got, expected)
	}
}

func TestBashCwdWrapEmpty(t *testing.T) {
	d := NewBashDriver()
	got := d.CwdWrap("ls -la", "")
	expected := "ls -la"
	if got != expected {
		t.Errorf("CwdWrap with empty cwd = %q, want %q", got, expected)
	}
}
