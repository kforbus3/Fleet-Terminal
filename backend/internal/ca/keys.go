package ca

import (
	"crypto/ed25519"
	"encoding/pem"

	"golang.org/x/crypto/ssh"
)

// pemPrivate serializes an Ed25519 private key to OpenSSH PEM bytes, which is
// what ssh.ParsePrivateKey reads back after decryption.
func pemPrivate(priv ed25519.PrivateKey) ([]byte, error) {
	block, err := ssh.MarshalPrivateKey(priv, "fleet-ca")
	if err != nil {
		return nil, err
	}
	return pem.EncodeToMemory(block), nil
}
