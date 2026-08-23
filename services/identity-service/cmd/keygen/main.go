// Command keygen prepares the shared JWT key volume before the non-root
// Identity and Gateway containers start. Production should mount keys from a
// secret manager instead of generating them inside the deployment.
package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	dir := env("JWT_KEY_DIR", "/keys")
	privatePath := filepath.Join(dir, "private.pem")
	publicPath := filepath.Join(dir, "public.pem")

	// Keep an existing pair so access tokens remain valid across restarts.
	if readable(privatePath) && readable(publicPath) {
		grantAppReadAccess(dir, privatePath, publicPath)
		fmt.Println("JWT key pair already exists")
		return
	}

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	must(err)
	must(os.MkdirAll(dir, 0o755))
	must(os.WriteFile(privatePath, pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	}), 0o600))

	publicDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	must(err)
	must(os.WriteFile(publicPath, pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: publicDER,
	}), 0o644))
	grantAppReadAccess(dir, privatePath, publicPath)
	fmt.Println("JWT key pair created")
}

func grantAppReadAccess(dir, privatePath, publicPath string) {
	// UID/GID 1000 is the app user created by deploy/Dockerfile. Identity owns
	// the private key; Gateway only needs the world-readable public key.
	must(os.Chown(privatePath, 1000, 1000))
	must(os.Chmod(privatePath, 0o600))
	must(os.Chown(publicPath, 1000, 1000))
	must(os.Chmod(publicPath, 0o644))
	must(os.Chmod(dir, 0o755))
}

func readable(path string) bool {
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	return file.Close() == nil
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func must(err error) {
	if err != nil && !errors.Is(err, os.ErrExist) {
		panic(err)
	}
}
