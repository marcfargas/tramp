package shell

import (
	"fmt"
	"regexp"
	"strings"
)

var bashSafeChars = regexp.MustCompile(`^[a-zA-Z0-9._\-/=:@]+$`)

type BashDriver struct{}

func NewBashDriver() *BashDriver {
	return &BashDriver{}
}

func (d *BashDriver) Type() ShellType {
	return ShellBash
}

func (d *BashDriver) Escape(arg string) string {
	if strings.ContainsRune(arg, 0) {
		panic("shell argument contains null byte — cannot be safely escaped")
	}
	if arg == "" {
		return "''"
	}
	if bashSafeChars.MatchString(arg) {
		return arg
	}
	return "'" + strings.ReplaceAll(arg, "'", "'\"'\"'") + "'"
}

// escapeForPath always single-quotes absolute paths to ensure safe use in
// compound commands where unquoted paths could be misinterpreted.
func (d *BashDriver) escapeForPath(absolutePath string) string {
	escaped := d.Escape(absolutePath)
	// If Escape returned an unquoted token (safe chars only), force single-quoting
	// so that absolute paths are never left bare in compound shell expressions.
	if len(escaped) > 0 && escaped[0] != '\'' {
		return "'" + absolutePath + "'"
	}
	return escaped
}

func (d *BashDriver) StatCommand(absolutePath string) string {
	escaped := d.escapeForPath(absolutePath)
	return fmt.Sprintf(
		"if [ -f %s ]; then echo file; elif [ -d %s ]; then echo directory; elif [ -e %s ]; then echo other; else echo missing; fi",
		escaped, escaped, escaped,
	)
}

func (d *BashDriver) MkdirCommand(absolutePath string) string {
	return fmt.Sprintf("mkdir -p %s", d.escapeForPath(absolutePath))
}

func (d *BashDriver) CwdWrap(command string, cwd string) string {
	if cwd == "" {
		return command
	}
	return fmt.Sprintf("cd %s && %s", d.escapeForPath(cwd), command)
}
