package agent

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"io"
	"os"
)

// Backup encryption in the format of `openssl enc -aes-256-cbc -pbkdf2 -iter 200000 -salt` ("Salted__", 8 bytes of
// salt, PBKDF2-HMAC-SHA256 deriving the key and the IV, AES-256-CBC with PKCS#7 padding), so a bundle can always be
// opened with the standard tool (RESTORE.txt) — but written and read in Go: Windows servers need no OpenSSL. The
// format has no authentication of its own; the SHA-256 checksum file and the manifest inside the bundle detect damage.

const encIter = 200000

var errBadPassphrase = errors.New("decryption failed: wrong passphrase or damaged file")

func deriveKeyIV(pass string, salt []byte) (key, iv []byte, err error) {
	kiv, err := pbkdf2.Key(sha256.New, pass, salt, encIter, 48)
	if err != nil {
		return nil, nil, err
	}
	return kiv[:32], kiv[32:], nil
}

// encryptFile writes src encrypted to dst.
func encryptFile(src, dst, pass string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if err := encryptStream(in, out, pass); err != nil {
		out.Close()
		os.Remove(dst)
		return err
	}
	return out.Close()
}

func encryptStream(r io.Reader, w io.Writer, pass string) error {
	salt := make([]byte, 8)
	if _, err := rand.Read(salt); err != nil {
		return err
	}
	key, iv, err := deriveKeyIV(pass, salt)
	if err != nil {
		return err
	}
	block, _ := aes.NewCipher(key)
	enc := cipher.NewCBCEncrypter(block, iv)
	if _, err := w.Write(append([]byte("Salted__"), salt...)); err != nil {
		return err
	}
	buf := make([]byte, 64*1024)
	var pending []byte
	for {
		n, rerr := io.ReadFull(r, buf)
		pending = append(pending, buf[:n]...)
		if rerr == io.EOF || rerr == io.ErrUnexpectedEOF {
			break
		}
		if rerr != nil {
			return rerr
		}
		// keep the tail that is not a whole block for the next round (the last block gets the padding)
		full := len(pending) - len(pending)%aes.BlockSize
		if full == len(pending) {
			full -= aes.BlockSize
		}
		enc.CryptBlocks(pending[:full], pending[:full])
		if _, err := w.Write(pending[:full]); err != nil {
			return err
		}
		pending = append(pending[:0], pending[full:]...)
	}
	pad := aes.BlockSize - len(pending)%aes.BlockSize
	pending = append(pending, bytes.Repeat([]byte{byte(pad)}, pad)...)
	enc.CryptBlocks(pending, pending)
	_, err = w.Write(pending)
	return err
}

// decryptFile writes src decrypted to dst.
func decryptFile(src, dst, pass string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if err := decryptStream(in, out, pass); err != nil {
		out.Close()
		os.Remove(dst)
		return err
	}
	return out.Close()
}

func decryptStream(r io.Reader, w io.Writer, pass string) error {
	head := make([]byte, 16)
	if _, err := io.ReadFull(r, head); err != nil || string(head[:8]) != "Salted__" {
		return errors.New("not an encrypted backup (missing the Salted__ header)")
	}
	key, iv, err := deriveKeyIV(pass, head[8:])
	if err != nil {
		return err
	}
	block, _ := aes.NewCipher(key)
	dec := cipher.NewCBCDecrypter(block, iv)
	buf := make([]byte, 64*1024)
	var pending []byte
	for {
		n, rerr := io.ReadFull(r, buf)
		pending = append(pending, buf[:n]...)
		if rerr == io.EOF || rerr == io.ErrUnexpectedEOF {
			break
		}
		if rerr != nil {
			return rerr
		}
		// hold back the last whole block: it carries the padding
		full := len(pending) - len(pending)%aes.BlockSize - aes.BlockSize
		if full > 0 {
			dec.CryptBlocks(pending[:full], pending[:full])
			if _, err := w.Write(pending[:full]); err != nil {
				return err
			}
			pending = append(pending[:0], pending[full:]...)
		}
	}
	if len(pending) == 0 || len(pending)%aes.BlockSize != 0 {
		return errBadPassphrase
	}
	dec.CryptBlocks(pending, pending)
	pad := int(pending[len(pending)-1])
	if pad == 0 || pad > aes.BlockSize || pad > len(pending) || !bytes.Equal(pending[len(pending)-pad:], bytes.Repeat([]byte{byte(pad)}, pad)) {
		return errBadPassphrase
	}
	_, err = w.Write(pending[:len(pending)-pad])
	return err
}
