package trystero

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"math/big"
	"strconv"
	"strings"
)

// libName prefixes topics and namespaces exactly as Trystero's utils.ts.
const libName = "Trystero"

// charSet is Trystero's id alphabet; ids are compared as strings, so the
// alphabet's order decides who leads a handshake.
const charSet = "0123456789AaBbCcDdEeFfGgHhIiJjKkLlMmNnOoPpQqRrSsTtUuVvWwXxYyZz"

// genID returns n characters of charSet, as Trystero's genId.
func genID(n int) string {
	var b strings.Builder
	limit := big.NewInt(int64(len(charSet)))
	for range n {
		i, err := rand.Int(rand.Reader, limit)
		if err != nil {
			panic(err)
		}
		b.WriteByte(charSet[i.Int64()])
	}
	return b.String()
}

// sha1Topic is Trystero's sha1: each byte of the digest in base 36,
// unpadded, concatenated. Topics and info hashes are derived from it.
func sha1Topic(s string) string {
	sum := sha1.Sum([]byte(s))
	var b strings.Builder
	for _, x := range sum {
		b.WriteString(strconv.FormatInt(int64(x), 36))
	}
	return b.String()
}

func topicPath(parts ...string) string { return strings.Join(parts, "@") }

// infoHash is the torrent strategy's info_hash for a topic: the first 20
// characters of its sha1Topic, sent to trackers as a string.
func infoHash(topic string) string { return sha1Topic(topic)[:20] }

// roomNamespace is the token that tags a room's frames on a peer
// connection shared by several rooms of an app (shared-peer.ts).
func roomNamespace(appID, roomID string) string {
	sum := sha256.Sum256([]byte(libName + ":" + appID + ":" + roomID))
	return hex.EncodeToString(sum[:])
}

// signalCipher is AES-GCM keyed by SHA-256("password:appId:roomId"), with
// WebCrypto's 16-byte IV: the key every peer of a room derives alike, so
// a peer without the password cannot read or answer its offers.
func signalCipher(password, appID, roomID string) (cipher.AEAD, error) {
	key := sha256.Sum256([]byte(password + ":" + appID + ":" + roomID))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCMWithNonceSize(block, 16)
}

// encryptSignal returns "iv,iv,...$base64(ciphertext+tag)", as crypto.ts.
func encryptSignal(aead cipher.AEAD, plaintext string) (string, error) {
	iv := make([]byte, 16)
	if _, err := rand.Read(iv); err != nil {
		return "", err
	}
	parts := make([]string, len(iv))
	for i, x := range iv {
		parts[i] = strconv.Itoa(int(x))
	}
	sealed := aead.Seal(nil, iv, []byte(plaintext), nil)
	return strings.Join(parts, ",") + "$" + base64.StdEncoding.EncodeToString(sealed), nil
}

var errDecrypt = errors.New("cannot decrypt signal")

// decryptSignal reverses encryptSignal; a wrong password fails the tag.
func decryptSignal(aead cipher.AEAD, raw string) (string, error) {
	ivText, data, ok := strings.Cut(raw, "$")
	if !ok {
		return "", errDecrypt
	}
	data, _, _ = strings.Cut(data, "$")
	fields := strings.Split(ivText, ",")
	if len(fields) != 16 {
		return "", errDecrypt
	}
	iv := make([]byte, len(fields))
	for i, f := range fields {
		n, err := strconv.Atoi(f)
		if err != nil || n < 0 || n > 255 {
			return "", errDecrypt
		}
		iv[i] = byte(n)
	}
	sealed, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return "", errDecrypt
	}
	plain, err := aead.Open(nil, iv, sealed, nil)
	if err != nil {
		return "", errDecrypt
	}
	return string(plain), nil
}

// handshakeHash answers a password challenge (handshake.ts).
func handshakeHash(challenge, password, appID, roomID string) string {
	sum := sha256.Sum256([]byte(challenge + ":" + password + ":" + appID + ":" + roomID))
	return hex.EncodeToString(sum[:])
}
