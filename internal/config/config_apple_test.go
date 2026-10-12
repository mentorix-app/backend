package config

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"strings"
	"testing"
)

const (
	testAppleTeamID = "TSQ6DL62KG"
	testAppleKeyID  = "AB12CD34EF"
)

func generateECKey(t *testing.T, curve elliptic.Curve) *ecdsa.PrivateKey {
	t.Helper()
	key, err := ecdsa.GenerateKey(curve, rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	return key
}

// pkcs8PEM is the text of a .p8 file for key.
func pkcs8PEM(t *testing.T, key crypto.PrivateKey) string {
	t.Helper()
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
}

func setAppleRevocationEnv(t *testing.T, teamID, keyID, key string) {
	t.Helper()
	setMinimalEnv(t)
	t.Setenv("APPLE_TEAM_ID", teamID)
	t.Setenv("APPLE_KEY_ID", keyID)
	t.Setenv("APPLE_PRIVATE_KEY", key)
}

// assertNoKeyMaterial fails when the error text holds the value or any line of its
// PEM body.
func assertNoKeyMaterial(t *testing.T, err error, value string) {
	t.Helper()
	text := err.Error()
	if strings.TrimSpace(value) != "" && strings.Contains(text, strings.TrimSpace(value)) {
		t.Errorf("error %q contains the whole value", text)
	}
	for _, line := range strings.FieldsFunc(value, func(r rune) bool { return r == '\n' || r == '\r' }) {
		line = strings.TrimSpace(line)
		if len(line) >= 8 && !strings.HasPrefix(line, "-----") && strings.Contains(text, line) {
			t.Errorf("error %q contains key material %q", text, line)
		}
	}
}

func TestLoad_appleRevocation_offWhenNothingIsSet(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  string
	}{
		{"empty", ""},
		{"whitespace only", " \t "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setAppleRevocationEnv(t, tc.raw, tc.raw, tc.raw)

			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if cfg.AppleTeamID != "" || cfg.AppleKeyID != "" || cfg.ApplePrivateKey != nil {
				t.Fatalf("team = %q, key id = %q, key set = %v, want all empty", cfg.AppleTeamID, cfg.AppleKeyID, cfg.ApplePrivateKey != nil)
			}
		})
	}
}

func TestLoad_appleRevocation_allSet(t *testing.T) {
	key := generateECKey(t, elliptic.P256())
	setAppleRevocationEnv(t, testAppleTeamID, testAppleKeyID, pkcs8PEM(t, key))

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.AppleTeamID != testAppleTeamID || cfg.AppleKeyID != testAppleKeyID {
		t.Errorf("team = %q, key id = %q, want %q and %q", cfg.AppleTeamID, cfg.AppleKeyID, testAppleTeamID, testAppleKeyID)
	}
	if cfg.ApplePrivateKey == nil || !cfg.ApplePrivateKey.Equal(key) {
		t.Fatal("ApplePrivateKey is not the configured key")
	}
}

func TestLoad_appleRevocation_trimsSurroundingSpace(t *testing.T) {
	key := generateECKey(t, elliptic.P256())
	setAppleRevocationEnv(t, " "+testAppleTeamID+" ", "\t"+testAppleKeyID, "\n"+pkcs8PEM(t, key)+"\n\n")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.AppleTeamID != testAppleTeamID || cfg.AppleKeyID != testAppleKeyID || cfg.ApplePrivateKey == nil {
		t.Fatalf("team = %q, key id = %q, key set = %v", cfg.AppleTeamID, cfg.AppleKeyID, cfg.ApplePrivateKey != nil)
	}
}

func TestLoad_appleRevocation_keyOnOneLineWithLiteralBackslashN(t *testing.T) {
	key := generateECKey(t, elliptic.P256())
	oneLine := strings.ReplaceAll(strings.TrimSpace(pkcs8PEM(t, key)), "\n", `\n`)
	if strings.Contains(oneLine, "\n") || !strings.Contains(oneLine, `\n`) {
		t.Fatalf("fixture is not a one-line value: %q", oneLine)
	}
	setAppleRevocationEnv(t, testAppleTeamID, testAppleKeyID, oneLine)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ApplePrivateKey == nil || !cfg.ApplePrivateKey.Equal(key) {
		t.Fatal("ApplePrivateKey is not the configured key")
	}
}

func TestLoad_appleRevocation_allOrNone(t *testing.T) {
	key := pkcs8PEM(t, generateECKey(t, elliptic.P256()))
	for _, tc := range []struct {
		name                string
		teamID, keyID, pkey string
	}{
		{"team id missing", "", testAppleKeyID, key},
		{"key id missing", testAppleTeamID, "", key},
		{"key missing", testAppleTeamID, testAppleKeyID, ""},
		{"only the team id", testAppleTeamID, "", ""},
		{"only the key id", "", testAppleKeyID, ""},
		{"only the key", "", "", key},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setAppleRevocationEnv(t, tc.teamID, tc.keyID, tc.pkey)

			_, err := Load()
			if err == nil {
				t.Fatal("expected an error")
			}
			for _, name := range []string{"APPLE_TEAM_ID", "APPLE_KEY_ID", "APPLE_PRIVATE_KEY"} {
				if !strings.Contains(err.Error(), name) {
					t.Errorf("error %q does not name %s", err, name)
				}
			}
			assertNoKeyMaterial(t, err, tc.pkey)
		})
	}
}

func TestLoad_appleRevocation_idShape(t *testing.T) {
	key := pkcs8PEM(t, generateECKey(t, elliptic.P256()))
	bad := map[string]string{
		"too short":        "ABCDE1234",
		"too long":         "ABCDE123456",
		"lower case":       "abcde12345",
		"punctuation":      "ABCDE-2345",
		"space inside":     "ABCDE 2345",
		"non-ASCII letter": "ABCDЕ12345",
	}
	for name, value := range bad {
		t.Run("team id "+name, func(t *testing.T) {
			setAppleRevocationEnv(t, value, testAppleKeyID, key)

			_, err := Load()
			if err == nil || !strings.Contains(err.Error(), "APPLE_TEAM_ID") {
				t.Fatalf("error = %v, want one that names APPLE_TEAM_ID", err)
			}
			assertNoKeyMaterial(t, err, key)
		})
		t.Run("key id "+name, func(t *testing.T) {
			setAppleRevocationEnv(t, testAppleTeamID, value, key)

			_, err := Load()
			if err == nil || !strings.Contains(err.Error(), "APPLE_KEY_ID") {
				t.Fatalf("error = %v, want one that names APPLE_KEY_ID", err)
			}
			assertNoKeyMaterial(t, err, key)
		})
	}
}

func TestLoad_appleRevocation_rejectsAKeyThatIsNotAP256PKCS8Key(t *testing.T) {
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	p256 := generateECKey(t, elliptic.P256())
	sec1, err := x509.MarshalECPrivateKey(p256)
	if err != nil {
		t.Fatalf("marshal sec1 key: %v", err)
	}
	p256Public, err := x509.MarshalPKIXPublicKey(&p256.PublicKey)
	if err != nil {
		t.Fatalf("marshal public key: %v", err)
	}
	validPEM := pkcs8PEM(t, p256)

	for _, tc := range []struct {
		name  string
		value string
	}{
		{"plain text", "this is not a key but it is secret-looking-text"},
		{"PEM block with garbage inside", string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte("garbage garbage garbage garbage")}))},
		{"RSA key", pkcs8PEM(t, rsaKey)},
		{"P-384 key", pkcs8PEM(t, generateECKey(t, elliptic.P384()))},
		{"P-521 key", pkcs8PEM(t, generateECKey(t, elliptic.P521()))},
		{"SEC1 block instead of PKCS#8", string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: sec1}))},
		{"public key", string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: p256Public}))},
		{"valid key cut in half", validPEM[:len(validPEM)/2]},
		{"base64 body without PEM lines", strings.Join(strings.Split(strings.TrimSpace(validPEM), "\n")[1:3], "")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setAppleRevocationEnv(t, testAppleTeamID, testAppleKeyID, tc.value)

			_, err := Load()
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), "APPLE_PRIVATE_KEY") {
				t.Errorf("error %q does not name APPLE_PRIVATE_KEY", err)
			}
			assertNoKeyMaterial(t, err, tc.value)
			// The same value with literal \n must not leak either.
			assertNoKeyMaterial(t, err, strings.ReplaceAll(tc.value, "\n", `\n`))
		})
	}
}
