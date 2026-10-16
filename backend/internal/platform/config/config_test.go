package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDotEnvDoesNotExecuteOrReplaceExplicitEmptyEnvironment(t *testing.T) {
	t.Setenv("TOKENHUB_SMTP_HOST", "")
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("TOKENHUB_SMTP_HOST=should-not-enable.example\nTOKENHUB_CONFIG_TEST_LITERAL='$(touch should-not-exist)'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Unsetenv("TOKENHUB_CONFIG_TEST_LITERAL") })
	loadDotEnv(path)
	if os.Getenv("TOKENHUB_SMTP_HOST") != "" {
		t.Fatal("explicit disable overwritten")
	}
	if os.Getenv("TOKENHUB_CONFIG_TEST_LITERAL") != "$(touch should-not-exist)" {
		t.Fatal("literal shell expression changed")
	}
}

func TestRedactedConfigurationContainsNoSecretValuesOrFragments(t *testing.T) {
	secret := "unique-private-configuration-material"
	c := &Config{DatabaseURL: secret, RedisURL: secret, EncryptionKey: secret, SMTPPassword: secret, S3AccessKey: secret, S3SecretKey: secret, CloudflareAPIToken: secret}
	b, e := json.Marshal(c.RedactedMap())
	if e != nil {
		t.Fatal(e)
	}
	if strings.Contains(string(b), secret) || strings.Contains(string(b), "material") || strings.Contains(string(b), "_len") {
		t.Fatal("secret metadata leaked")
	}
}
