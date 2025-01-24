package auth

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/go-git/go-git/v5/plumbing/transport/ssh"
)

func GetSSHAuth() (*ssh.PublicKeys, error) {
	sshPath := os.Getenv("SSH_KEY_PATH")
	if sshPath == "" {
		// Default to standard SSH key location
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		sshPath = filepath.Join(homeDir, ".ssh", "id_rsa")
	}

	publicKeys, err := ssh.NewPublicKeysFromFile("git", sshPath, "")
	if err != nil {
		return nil, fmt.Errorf("error loading SSH key: %v", err)
	}
	return publicKeys, nil
}
