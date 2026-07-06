// Package auth implements Pluris's local authentication: password
// hashing, server-side sessions, and RBAC enforcement. This is the
// app's first authentication system — no external identity provider
// (Kanidm/AD/FreeIPA) is wired yet; that's explicitly a later phase.
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Argon2id parameters. These match the OWASP-recommended minimums for
// interactive login (not a high-throughput API): 19 MiB memory, 2
// iterations, 1 degree of parallelism, 32-byte output. Encoded into the
// hash string itself so parameters can change later without breaking
// verification of existing hashes.
const (
	argonMemoryKiB  = 19 * 1024
	argonIterations = 2
	argonThreads    = 1
	argonKeyLen     = 32
	argonSaltLen    = 16
)

// HashPassword returns a self-describing Argon2id hash string in the form
// "$argon2id$v=19$m=19456,t=2,p=1$<salt-b64>$<hash-b64>".
func HashPassword(password string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}

	hash := argon2.IDKey([]byte(password), salt, argonIterations, argonMemoryKiB, argonThreads, argonKeyLen)

	encoded := fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemoryKiB, argonIterations, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	)
	return encoded, nil
}

// VerifyPassword checks password against an encoded hash produced by
// HashPassword. Uses constant-time comparison to avoid timing side
// channels.
func VerifyPassword(password, encoded string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false, fmt.Errorf("invalid hash format")
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return false, fmt.Errorf("invalid version segment: %w", err)
	}

	var memory, iterations uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &threads); err != nil {
		return false, fmt.Errorf("invalid params segment: %w", err)
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, fmt.Errorf("invalid salt encoding: %w", err)
	}
	wantHash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false, fmt.Errorf("invalid hash encoding: %w", err)
	}

	gotHash := argon2.IDKey([]byte(password), salt, iterations, memory, threads, uint32(len(wantHash)))

	return subtle.ConstantTimeCompare(gotHash, wantHash) == 1, nil
}
