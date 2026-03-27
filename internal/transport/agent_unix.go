//go:build !windows

package transport

import (
	"net"
	"os"
)

// sshAgentConn connects to the SSH agent via SSH_AUTH_SOCK.
func sshAgentConn() net.Conn {
	socket := os.Getenv("SSH_AUTH_SOCK")
	if socket == "" {
		return nil
	}
	conn, err := net.Dial("unix", socket)
	if err != nil {
		return nil
	}
	return conn
}
