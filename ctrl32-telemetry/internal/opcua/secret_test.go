package opcua

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/iam"
)

func TestStoredPasswordIsEncrypted(t *testing.T) {
	b, err := iam.NewSecretBox("0123456789abcdef-test")
	if err != nil {
		t.Fatal(err)
	}
	SetSecretBox(b)
	defer SetSecretBox(nil)
	st := Settings{Endpoint: "opc.tcp://plc:4840", Auth: "username", Username: "operator", Password: "S3cret-of-the-PLC"}
	stored, err := SealStored(st)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stored, "S3cret-of-the-PLC") || strings.Contains(stored, `"password":`) {
		t.Fatalf("the stored form holds the password: %s", stored)
	}
	if !strings.Contains(stored, `"password_enc":"`) || !strings.Contains(stored, `"username":"operator"`) {
		t.Fatalf("stored form: %s", stored)
	}
	again, _ := SealStored(st)
	if again == stored {
		t.Fatal("two seals of one password must differ (random nonce)")
	}
	var back Settings
	if err := json.Unmarshal(OpenStored([]byte(stored)), &back); err != nil {
		t.Fatal(err)
	}
	if back.Password != "S3cret-of-the-PLC" || back.Username != "operator" || back.Endpoint != st.Endpoint {
		t.Fatalf("opened: %+v", back)
	}

	// another secret key: the password does not open and is not replaced by the ciphertext
	other, _ := iam.NewSecretBox("another-secret-key-000")
	SetSecretBox(other)
	var lost Settings
	_ = json.Unmarshal(OpenStored([]byte(stored)), &lost)
	if lost.Password != "" {
		t.Fatalf("a foreign key opened the password: %q", lost.Password)
	}
	SetSecretBox(b)

	// anonymous login and legacy plain-text rows pass through
	plain, _ := SealStored(Settings{Endpoint: "opc.tcp://plc:4840", Auth: "anonymous"})
	if strings.Contains(plain, "password") {
		t.Fatalf("anonymous settings: %s", plain)
	}
	legacy := `{"endpoint":"opc.tcp://plc:4840","auth":"username","username":"u","password":"old"}`
	if string(OpenStored([]byte(legacy))) != legacy {
		t.Fatal("legacy settings must come back unchanged")
	}

	// without a key a password is never written in plain text
	SetSecretBox(nil)
	if _, err := SealStored(st); err == nil {
		t.Fatal("sealing without a key must fail")
	}
}
