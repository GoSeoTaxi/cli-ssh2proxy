package sshclient

import (
	"context"
	"net"
	"os"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

type DialFunc func(ctx context.Context, network, addr string) (net.Conn, error)

func New(user, pass, host, port, keyPath string) (*Reconnector, DialFunc, error) {
	cfg := buildConfig(user, pass, keyPath)
	addr := net.JoinHostPort(host, port)

	reConnector, err := NewReconnector(addr, cfg)
	if err != nil {
		return nil, nil, err
	}
	return reConnector, reConnector.Dial, nil
}

func buildConfig(user, pass, keyPath string) *ssh.ClientConfig {
	var auths []ssh.AuthMethod

	if sock := os.Getenv("SSH_AUTH_SOCK"); sock != "" {
		if conn, err := net.Dial("unix", sock); err == nil {
			auths = append(auths,
				ssh.PublicKeysCallback(agent.NewClient(conn).Signers))
		}
	}

	if keyPath != "" {
		if key, err := os.ReadFile(expandHome(keyPath)); err == nil {
			if signer, err := ssh.ParsePrivateKey(key); err == nil {
				auths = append(auths, ssh.PublicKeys(signer))
			}
		}
	}

	if pass != "" {
		auths = append(auths, ssh.Password(pass))
		auths = append(auths, ssh.KeyboardInteractive(
			func(user, instr string, qs []string, echos []bool) ([]string, error) {
				ans := make([]string, len(qs))
				for i := range qs {
					ans[i] = pass
				}
				return ans, nil
			}))
	}

	return &ssh.ClientConfig{
		User:            user,
		Auth:            auths,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}
}

func expandHome(p string) string {
	if len(p) > 1 && p[:2] == "~/" {
		if h, _ := os.UserHomeDir(); h != "" {
			return h + p[1:]
		}
	}
	return p
}
