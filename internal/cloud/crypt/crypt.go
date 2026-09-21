// Package crypt es la criptografía de la nube de ccp (spec §10.1-10.2): la
// clave de cuenta (AK) y todo lo que sale de ella. El servidor nunca ve la AK ni
// nada que permita abrir lo que guarda: recibe textos sellados, ids opacos y
// firmas. Lo usa el cliente; el servidor no importa este paquete.
package crypt

import (
	"bytes"
	"compress/gzip"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"

	"github.com/JoseAFlores777/ccp/internal/snapshot"
	"github.com/JoseAFlores777/ccp/internal/vault"
)

// Usos de la AK (info de HKDF) y datos asociados de las envolturas. Cambiar
// cualquiera deja ilegible todo lo ya subido: son parte del formato.
const (
	infoData   = "ccp/v1/data"
	infoIDs    = "ccp/v1/ids"
	infoSign   = "ccp/v1/sign"
	adPassWrap = "ccp/v1/wrap/passphrase"
	adRecWrap  = "ccp/v1/wrap/recovery"
)

var (
	// ErrSignature: la firma no corresponde. El snapshot fue alterado, o no es de esta cuenta.
	ErrSignature = errors.New("crypt: la firma no corresponde: el snapshot fue alterado o no es de esta cuenta")
	// ErrWrongSecret: la frase o el código no abren la bóveda.
	ErrWrongSecret = errors.New("crypt: la frase o el código no abren la bóveda")
)

// Account son las claves derivadas de una AK.
type Account struct {
	data []byte
	ids  []byte
	sign ed25519.PrivateKey
}

// NewAccount deriva las claves de uso de la AK.
func NewAccount(ak []byte) (*Account, error) {
	data, err := vault.DeriveSubkey(ak, infoData)
	if err != nil {
		return nil, err
	}
	ids, err := vault.DeriveSubkey(ak, infoIDs)
	if err != nil {
		return nil, err
	}
	seed, err := vault.DeriveSubkey(ak, infoSign)
	if err != nil {
		return nil, err
	}
	return &Account{data: data, ids: ids, sign: ed25519.NewKeyFromSeed(seed)}, nil
}

func (a *Account) mac(kind, v string) string {
	m := hmac.New(sha256.New, a.ids)
	m.Write([]byte(kind))
	m.Write([]byte{0})
	m.Write([]byte(v))
	return hex.EncodeToString(m.Sum(nil))
}

// BlobID es el id en la nube del blob local localHash. Es opaco para el
// servidor, que no puede comprobar si guardas un archivo concreto, y estable
// para la cuenta: dos equipos con el mismo archivo lo suben una sola vez.
func (a *Account) BlobID(localHash string) string { return a.mac("blob", localHash) }

// SnapshotID es el id en la nube del snapshot local localID ("" si no hay).
func (a *Account) SnapshotID(localID string) string {
	if localID == "" {
		return ""
	}
	return a.mac("snapshot", localID)
}

// SealBlob comprime y sella el contenido de un blob. El id va como dato
// asociado: un blob copiado bajo otro id no abre.
func (a *Account) SealBlob(id string, data []byte) ([]byte, error) {
	var b bytes.Buffer
	w := gzip.NewWriter(&b)
	if _, err := w.Write(data); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return vault.Seal(a.data, b.Bytes(), []byte("blob:"+id))
}

// OpenBlob deshace SealBlob. El límite de snapshot.MaxBlobSize frena una bomba
// gzip: el sellado garantiza quién lo escribió, no que sea razonable.
func (a *Account) OpenBlob(id string, sealed []byte) ([]byte, error) {
	body, err := vault.Open(a.data, sealed, []byte("blob:"+id))
	if err != nil {
		return nil, err
	}
	r, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer func() { _ = r.Close() }()
	data, err := io.ReadAll(io.LimitReader(r, snapshot.MaxBlobSize+1))
	if err != nil {
		return nil, err
	}
	if len(data) > snapshot.MaxBlobSize {
		return nil, fmt.Errorf("crypt: blob de más de %d bytes", snapshot.MaxBlobSize)
	}
	return data, nil
}

// SealManifest sella el JSON de un manifiesto bajo el id del snapshot.
func (a *Account) SealManifest(id string, manifest []byte) ([]byte, error) {
	return vault.Seal(a.data, manifest, []byte("manifest:"+id))
}

// OpenManifest deshace SealManifest.
func (a *Account) OpenManifest(id string, sealed []byte) ([]byte, error) {
	return vault.Open(a.data, sealed, []byte("manifest:"+id))
}

// ManifestDigest es el hash del manifiesto sellado en hexadecimal: lo único
// del manifiesto que entra en la firma. Existe aparte porque la cadena se
// verifica desde un listado, donde el manifiesto no viaja —bajar megabytes
// para recalcular un hash que el servidor ya tiene sería pagar el contenido
// entero por comprobar la forma de la historia.
func ManifestDigest(sealedManifest []byte) string {
	sum := sha256.Sum256(sealedManifest)
	return hex.EncodeToString(sum[:])
}

// signedDigest es lo que se firma: id, padre y el hash del manifiesto sellado.
// Quien no tiene la AK no puede ni fabricar un snapshot ni reordenar la cadena.
func signedDigest(id, parent, digest string) []byte {
	return []byte("ccp/v1/snapshot\n" + id + "\n" + parent + "\n" + digest)
}

func signed(id, parent string, sealedManifest []byte) []byte {
	return signedDigest(id, parent, ManifestDigest(sealedManifest))
}

// Sign firma un snapshot sellado.
func (a *Account) Sign(id, parent string, sealedManifest []byte) []byte {
	return ed25519.Sign(a.sign, signed(id, parent, sealedManifest))
}

// Verify comprueba la firma de un snapshot. La clave pública sale de la AK, no
// del servidor: un servidor comprometido no puede sustituirla.
func (a *Account) Verify(id, parent string, sealedManifest, sig []byte) error {
	if !ed25519.Verify(a.SignPublic(), signed(id, parent, sealedManifest), sig) {
		return ErrSignature
	}
	return nil
}

// VerifyDigest es Verify cuando del manifiesto solo se tiene su digest, que es
// como llega la cadena. Falla igual que Verify si alguien tocó el id, el padre
// o el contenido del eslabón.
func (a *Account) VerifyDigest(id, parent, digest string, sig []byte) error {
	if !ed25519.Verify(a.SignPublic(), signedDigest(id, parent, digest), sig) {
		return ErrSignature
	}
	return nil
}

// SignPublic es la clave pública de firma de la cuenta.
func (a *Account) SignPublic() ed25519.PublicKey {
	return a.sign.Public().(ed25519.PublicKey)
}

// RevisionParts es lo que ata una revisión deseada al firmarla: qué orden es,
// qué eslabón encadena, a QUIÉN va dirigida y qué estado pide. El dispositivo
// entra en la firma a propósito: firmar solo el contenido dejaría al servidor
// servirle a una máquina la revisión que el portal escribió para otra, sin
// falsificar nada.
type RevisionParts struct {
	ID       string
	Prev     string
	Device   string
	Snapshot string
	Base     string
	Body     []byte
}

// signedRevision es lo que se firma. Prefijo de dominio propio para que la
// firma de una revisión no pueda pasar por la de un snapshot; los campos van
// separados por saltos de línea porque todos son ids validados (hex o UUID) y
// ninguno puede llevar uno dentro.
func signedRevision(p RevisionParts) []byte {
	sum := sha256.Sum256(p.Body)
	return []byte("ccp/v1/revision\n" + p.ID + "\n" + p.Prev + "\n" + p.Device + "\n" +
		p.Snapshot + "\n" + p.Base + "\n" + hex.EncodeToString(sum[:]))
}

// SignRevision firma una revisión deseada.
func (a *Account) SignRevision(p RevisionParts) []byte {
	return ed25519.Sign(a.sign, signedRevision(p))
}

// VerifyRevision comprueba la firma de una revisión deseada. El servidor no
// tiene con qué firmar: una orden que no verifique aquí no sale de él, viene
// de quien tenga la clave de cuenta.
func (a *Account) VerifyRevision(p RevisionParts, sig []byte) error {
	if !ed25519.Verify(a.SignPublic(), signedRevision(p), sig) {
		return ErrSignature
	}
	return nil
}

// Wraps es lo que el servidor guarda de la bóveda: dos envolturas de la AK que
// no sabe abrir y la clave pública de firma.
type Wraps struct {
	KDF        vault.KDFParams
	Passphrase []byte
	Recovery   []byte
	SignPub    []byte
}

// NewVault crea una AK nueva y sus dos envolturas: una con la frase (Argon2id)
// y otra con un código de recuperación, que se devuelve para enseñarlo UNA vez.
func NewVault(passphrase []byte) (ak []byte, recovery string, w Wraps, err error) {
	if ak, err = vault.NewKey(); err != nil {
		return nil, "", Wraps{}, err
	}
	if w.KDF, err = vault.NewKDFParams(); err != nil {
		return nil, "", Wraps{}, err
	}
	kek, err := vault.DeriveKey(passphrase, w.KDF)
	if err != nil {
		return nil, "", Wraps{}, err
	}
	if w.Passphrase, err = vault.Seal(kek, ak, []byte(adPassWrap)); err != nil {
		return nil, "", Wraps{}, err
	}
	if recovery, err = vault.NewRecoveryCode(); err != nil {
		return nil, "", Wraps{}, err
	}
	rk, err := vault.RecoveryKey(recovery)
	if err != nil {
		return nil, "", Wraps{}, err
	}
	if w.Recovery, err = vault.Seal(rk, ak, []byte(adRecWrap)); err != nil {
		return nil, "", Wraps{}, err
	}
	acct, err := NewAccount(ak)
	if err != nil {
		return nil, "", Wraps{}, err
	}
	w.SignPub = acct.SignPublic()
	return ak, recovery, w, nil
}

// UnlockPassphrase abre la bóveda con la frase.
func UnlockPassphrase(w Wraps, passphrase []byte) ([]byte, error) {
	kek, err := vault.DeriveKey(passphrase, w.KDF)
	if err != nil {
		return nil, err
	}
	ak, err := vault.Open(kek, w.Passphrase, []byte(adPassWrap))
	if err != nil {
		return nil, ErrWrongSecret
	}
	return checkAK(ak, w)
}

// UnlockRecovery abre la bóveda con el código de recuperación.
func UnlockRecovery(w Wraps, code string) ([]byte, error) {
	rk, err := vault.RecoveryKey(code)
	if err != nil {
		return nil, ErrWrongSecret
	}
	ak, err := vault.Open(rk, w.Recovery, []byte(adRecWrap))
	if err != nil {
		return nil, ErrWrongSecret
	}
	return checkAK(ak, w)
}

// checkAK exige que la AK abierta corresponda a la clave pública guardada: una
// bóveda que abre pero firma con otra clave es la de otra cuenta, y adoptarla
// dejaría al equipo subiendo snapshots que su cuenta no puede verificar.
func checkAK(ak []byte, w Wraps) ([]byte, error) {
	acct, err := NewAccount(ak)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(acct.SignPublic(), w.SignPub) {
		return nil, errors.New("crypt: la bóveda abre, pero su clave no corresponde a la cuenta")
	}
	return ak, nil
}
