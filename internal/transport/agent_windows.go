//go:build windows

package transport

import (
	"net"

	winio "github.com/Microsoft/go-winio"
)

// sshAgentConn connects to the Windows OpenSSH agent via named pipe.
func sshAgentConn() net.Conn {
	conn, err := winio.DialPipe(`\\.\pipe\openssh-ssh-agent`, nil)
	if err != nil {
		return nil
	}
	return conn
}
