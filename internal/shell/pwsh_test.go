package shell

import "testing"

func TestPwshEscape(t *testing.T) {
	d := NewPwshDriver()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"empty string", "", "''"},
		{"simple alphanumeric", "hello", "hello"},
		{"path with forward slashes", "/usr/bin/ls", "/usr/bin/ls"},
		{"path with backslashes", `C:\Users\marc`, `C:\Users\marc`},
		{"safe chars", "a.b-c_d/e=f:g@h", "a.b-c_d/e=f:g@h"},
		{"space needs quoting", "hello world", "'hello world'"},
		{"single quote doubled", "it's", "'it''s'"},
		{"multiple single quotes", "a'b'c", "'a''b''c'"},
		{"special chars", "foo;bar", "'foo;bar'"},
		{"dollar sign", "$HOME", "'$HOME'"},
		{"backtick", "foo`bar", "'foo`bar'"},
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

func TestPwshEscapeNullByte(t *testing.T) {
	d := NewPwshDriver()
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on null byte")
		}
	}()
	d.Escape("hello\x00world")
}

func TestPwshForceQuote(t *testing.T) {
	d := NewPwshDriver()
	tests := []struct {
		input    string
		expected string
	}{
		{"simple", "'simple'"},
		{"it's", "'it''s'"},
		{"/usr/bin/ls", "'/usr/bin/ls'"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := d.ForceQuote(tt.input)
			if got != tt.expected {
				t.Errorf("ForceQuote(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestPwshStatCommand(t *testing.T) {
	d := NewPwshDriver()
	got := d.StatCommand(`C:\Users\file.txt`)
	expected := `if (Test-Path -PathType Leaf C:\Users\file.txt) { 'file' } elseif (Test-Path -PathType Container C:\Users\file.txt) { 'directory' } elseif (Test-Path C:\Users\file.txt) { 'other' } else { 'missing' }`
	if got != expected {
		t.Errorf("StatCommand = %q, want %q", got, expected)
	}
}

func TestPwshMkdirCommand(t *testing.T) {
	d := NewPwshDriver()
	got := d.MkdirCommand("/home/user/new dir")
	expected := "New-Item -ItemType Directory -Force -Path '/home/user/new dir' | Out-Null"
	if got != expected {
		t.Errorf("MkdirCommand = %q, want %q", got, expected)
	}
}

func TestPwshCwdWrap(t *testing.T) {
	d := NewPwshDriver()
	got := d.CwdWrap("Get-ChildItem", `C:\Users\project`)
	expected := `Set-Location C:\Users\project; Get-ChildItem`
	if got != expected {
		t.Errorf("CwdWrap = %q, want %q", got, expected)
	}
}

func TestPwshCwdWrapEmpty(t *testing.T) {
	d := NewPwshDriver()
	got := d.CwdWrap("Get-ChildItem", "")
	expected := "Get-ChildItem"
	if got != expected {
		t.Errorf("CwdWrap with empty cwd = %q, want %q", got, expected)
	}
}
