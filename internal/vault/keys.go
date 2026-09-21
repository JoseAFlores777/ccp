package vault

import (
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"errors"
	"fmt"
	"strings"
)

// DeriveSubkey deriva de master una subclave de KeySize bytes para un uso (info).
// Con usos distintos las claves son independientes: filtrar la de firmar no da
// la de cifrar.
func DeriveSubkey(master []byte, info string) ([]byte, error) {
	if len(master) != KeySize {
		return nil, fmt.Errorf("vault: la clave maestra mide %d bytes y debería medir %d", len(master), KeySize)
	}
	return hkdf.Key(sha256.New, master, nil, info, KeySize)
}

// Un código de recuperación son 20 bytes aleatorios (160 bits) en base32, en 8
// grupos de 4 («ABCD-EFGH-…»). Tiene entropía de sobra para no necesitar
// Argon2: se deriva con HKDF. Se enseña una sola vez y nunca sale del equipo.
const recoveryBytes = 20

var recoveryEnc = base32.StdEncoding.WithPadding(base32.NoPadding)

// NewRecoveryCode genera un código de recuperación nuevo.
func NewRecoveryCode() (string, error) {
	raw := make([]byte, recoveryBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("vault: sin entropía: %w", err)
	}
	s := recoveryEnc.EncodeToString(raw)
	groups := make([]string, 0, len(s)/4)
	for i := 0; i < len(s); i += 4 {
		groups = append(groups, s[i:i+4])
	}
	return strings.Join(groups, "-"), nil
}

// ErrRecoveryCode: el texto no es un código de recuperación.
var ErrRecoveryCode = errors.New("vault: código de recuperación inválido")

// RecoveryKey convierte un código (con o sin guiones o espacios, en cualquier
// caja) en su clave.
func RecoveryKey(code string) ([]byte, error) {
	clean := strings.ToUpper(strings.NewReplacer("-", "", " ", "").Replace(strings.TrimSpace(code)))
	raw, err := recoveryEnc.DecodeString(clean)
	if err != nil || len(raw) != recoveryBytes {
		return nil, ErrRecoveryCode
	}
	return hkdf.Key(sha256.New, raw, nil, "ccp/v1/recovery", KeySize)
}
