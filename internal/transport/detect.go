package transport

import (
	"regexp"
	"strconv"
	"strings"
)

var ansiPattern = regexp.MustCompile(`\x1b[[(][^\x1b]*?[a-zA-Z]|\x1b\][^\x07]*\x07`)

func StripAnsi(s string) string {
	return ansiPattern.ReplaceAllString(s, "")
}

func ParseShellName(output string) string {
	cleaned := strings.ToLower(strings.TrimSpace(StripAnsi(output)))
	cleaned = strings.TrimPrefix(cleaned, "-")
	parts := strings.FieldsFunc(cleaned, func(r rune) bool { return r == '/' || r == '\\' })
	basename := cleaned
	if len(parts) > 0 {
		basename = parts[len(parts)-1]
	}
	basename = strings.TrimSuffix(basename, ".exe")

	switch basename {
	case "bash":
		return "bash"
	case "sh", "dash", "ash":
		return "sh"
	case "zsh":
		return "bash"
	case "pwsh", "powershell":
		return "pwsh"
	case "cmd":
		return "cmd"
	default:
		return "unknown"
	}
}

func ParsePlatform(output string) string {
	cleaned := strings.TrimSpace(StripAnsi(output))
	switch {
	case cleaned == "Linux":
		return "linux"
	case cleaned == "Darwin":
		return "darwin"
	case strings.HasPrefix(cleaned, "MINGW"),
		strings.HasPrefix(cleaned, "MSYS"),
		strings.HasPrefix(cleaned, "CYGWIN"),
		strings.HasPrefix(cleaned, "Windows_NT"),
		strings.HasPrefix(cleaned, "Windows"),
		strings.EqualFold(cleaned, "windows"):
		return "windows"
	default:
		return "unknown"
	}
}

func ParseArch(output string) string {
	cleaned := strings.ToLower(strings.TrimSpace(StripAnsi(output)))
	if len(cleaned) > 64 || cleaned == "" {
		return "unknown"
	}
	if cleaned == "arm64" {
		return "aarch64"
	}
	return cleaned
}

func ParsePwshVersion(output string) (int, bool) {
	cleaned := strings.TrimSpace(StripAnsi(output))
	v, err := strconv.Atoi(cleaned)
	if err != nil || v <= 0 {
		return 0, false
	}
	return v, true
}

func PwshPlatformCommand() string {
	return "if ($IsLinux) { 'linux' } elseif ($IsMacOS) { 'darwin' } else { 'windows' }"
}

func PwshArchCommand() string {
	return "[System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture"
}

func ParsePwshArch(output string) string {
	cleaned := strings.TrimSpace(StripAnsi(output))
	switch cleaned {
	case "X64":
		return "x86_64"
	case "Arm64":
		return "aarch64"
	case "X86":
		return "x86"
	case "Arm":
		return "arm"
	default:
		return "unknown"
	}
}
