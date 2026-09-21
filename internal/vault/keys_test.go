package vault

import (
	"bytes"
	"regexp"
	"strings"
	"testing"
)

func TestDeriveSubkey(t *testing.T) {
	master := bytes.Repeat([]byte{9}, KeySize)
	a, err := DeriveSubkey(master, "ccp/v1/data")
	if err != nil || len(a) != KeySize {
		t.Fatalf("DeriveSubkey = %d bytes, %v", len(a), err)
	}
	again, _ := DeriveSubkey(master, "ccp/v1/data")
	other, _ := DeriveSubkey(master, "ccp/v1/sign")
	if !bytes.Equal(a, again) || bytes.Equal(a, other) {
		t.Fatal("mismo uso debe dar la misma subclave, y usos distintos, subclaves distintas")
	}
	if _, err := DeriveSubkey([]byte("corta"), "x"); err == nil {
		t.Fatal("aceptó una clave maestra corta")
	}
}

func TestRecoveryCode(t *testing.T) {
	code, err := NewRecoveryCode()
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^[A-Z2-7]{4}(-[A-Z2-7]{4}){7}$`).MatchString(code) {
		t.Fatalf("formato del código: %q", code)
	}
	k1, err := RecoveryKey(code)
	if err != nil || len(k1) != KeySize {
		t.Fatalf("RecoveryKey = %v", err)
	}
	// Se acepta escrito a mano: minúsculas, sin guiones, con espacios.
	messy := strings.ToLower(strings.ReplaceAll(code, "-", " "))
	if k2, err := RecoveryKey(messy); err != nil || !bytes.Equal(k1, k2) {
		t.Fatalf("código escrito a mano: %v", err)
	}
	other, _ := NewRecoveryCode()
	if k3, _ := RecoveryKey(other); bytes.Equal(k1, k3) {
		t.Fatal("dos códigos distintos dan la misma clave")
	}
	for _, bad := range []string{"", "ABCD", code + "-ABCD", "0000-1111-2222-3333-4444-5555-6666-7777"} {
		if _, err := RecoveryKey(bad); err == nil {
			t.Errorf("RecoveryKey(%q) sin error", bad)
		}
	}
}
