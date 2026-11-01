package flashchat

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"unicode/utf16"
)

// Identity is a peer's RSASSA-PKCS1-v1_5 (SHA-256, 2048-bit) key pair, as
// crypto/Encryption.ts generates it for each join.
type Identity struct {
	key       *rsa.PrivateKey
	publicKey string
}

// NewIdentity generates a key pair.
func NewIdentity() (*Identity, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	spki, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return nil, err
	}
	return &Identity{key: key, publicKey: base64.StdEncoding.EncodeToString(spki)}, nil
}

// PublicKey is the SPKI public key, base64, as stringifyCryptoKey.
func (id *Identity) PublicKey() string { return id.publicKey }

// identityMessage binds the room in, so a proof made in one room cannot
// be replayed in another (identity.ts).
func identityMessage(roomID, userID string) string { return roomID + "_" + userID }

// Sign returns the proof for roomID and userID, base64.
func (id *Identity) Sign(roomID, userID string) (string, error) {
	digest := sha256.Sum256([]byte(identityMessage(roomID, userID)))
	signature, err := rsa.SignPKCS1v15(rand.Reader, id.key, crypto.SHA256, digest[:])
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(signature), nil
}

// VerifyIdentity checks a peer's advertised key and proof. ok is false
// when the key cannot be parsed (the metadata is then ignored); a key that
// parses but does not prove the identity yields Unverified.
func VerifyIdentity(publicKey, signature, roomID, userID string) (state VerificationState, ok bool) {
	spki, err := base64.StdEncoding.DecodeString(publicKey)
	if err != nil {
		return Unverified, false
	}
	parsed, err := x509.ParsePKIXPublicKey(spki)
	if err != nil {
		return Unverified, false
	}
	key, isRSA := parsed.(*rsa.PublicKey)
	if !isRSA {
		return Unverified, false
	}
	sig, err := base64.StdEncoding.DecodeString(signature)
	if err != nil {
		return Unverified, true
	}
	digest := sha256.Sum256([]byte(identityMessage(roomID, userID)))
	if rsa.VerifyPKCS1v15(key, crypto.SHA256, digest[:], sig) != nil {
		return Unverified, true
	}
	return Verified, true
}

// PeerName is the stable generated name of a user id (fun-animal-names):
// every peer derives the same one, so it is the name others can check.
func PeerName(userID string) string {
	hash := charCodeSum(userID)
	adjective := adjectives[hash%len(adjectives)]
	return adjective + " " + animalNames[(hash+charCodeSum(adjective))%len(animalNames)]
}

// charCodeSum is convertStringToInteger: UTF-16 code units weighted by
// their index.
func charCodeSum(s string) int {
	sum := 0
	for i, unit := range utf16.Encode([]rune(s)) {
		sum += int(unit) * i
	}
	return sum
}

// DisplayName shows a custom name with the generated one, which stays
// visible as the only identifier peers can verify (peerNames.ts).
func DisplayName(customUsername, userID string) string {
	if customUsername != "" {
		return customUsername + " (" + PeerName(userID) + ")"
	}
	return PeerName(userID)
}

// PeerDisplayName is the display name of a peer in hand.
func PeerDisplayName(p Peer) string { return DisplayName(p.CustomUsername, p.UserID) }
