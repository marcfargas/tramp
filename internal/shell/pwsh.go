package shell

import (
	"fmt"
	"regexp"
	"strings"
)

var pwshSafeChars = regexp.MustCompile(`^[a-zA-Z0-9._\-/\\=:@]+$`)

type PwshDriver struct{}

func NewPwshDriver() *PwshDriver {
	return &PwshDriver{}
}

func (d *PwshDriver) Type() ShellType {
	return ShellPwsh
}

func (d *PwshDriver) Escape(arg string) string {
	if strings.ContainsRune(arg, 0) {
		panic("shell argument contains null byte — cannot be safely escaped")
	}
	if arg == "" {
		return "''"
	}
	if pwshSafeChars.MatchString(arg) {
		return arg
	}
	return "'" + strings.ReplaceAll(arg, "'", "''") + "'"
}

func (d *PwshDriver) ForceQuote(arg string) string {
	return "'" + strings.ReplaceAll(arg, "'", "''") + "'"
}

func (d *PwshDriver) StatCommand(absolutePath string) string {
	escaped := d.Escape(absolutePath)
	return fmt.Sprintf(
		"if (Test-Path -PathType Leaf %s) { 'file' } elseif (Test-Path -PathType Container %s) { 'directory' } elseif (Test-Path %s) { 'other' } else { 'missing' }",
		escaped, escaped, escaped,
	)
}

func (d *PwshDriver) MkdirCommand(absolutePath string) string {
	return fmt.Sprintf("New-Item -ItemType Directory -Force -Path %s | Out-Null", d.Escape(absolutePath))
}

func (d *PwshDriver) CwdWrap(command string, cwd string) string {
	if cwd == "" {
		return command
	}
	return fmt.Sprintf("Set-Location %s; %s", d.Escape(cwd), command)
}

func (d *PwshDriver) SessionSetup() string {
	return "$PSStyle.OutputRendering = 'PlainText'; $ErrorActionPreference = 'Continue'; $ProgressPreference = 'SilentlyContinue'"
}
