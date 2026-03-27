package shell

type ShellType string

const (
	ShellBash ShellType = "bash"
	ShellSh   ShellType = "sh"
	ShellPwsh ShellType = "pwsh"
)

type ShellDriver interface {
	Type() ShellType
	Escape(arg string) string
	StatCommand(absolutePath string) string
	MkdirCommand(absolutePath string) string
	CwdWrap(command string, cwd string) string
}
