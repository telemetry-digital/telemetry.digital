package opcua

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
)

// The password of a user-name login is stored encrypted (AES-256-GCM with the server's secret key) as
// "password_enc" in core.connector.settings; "password" exists in memory and in API requests only. A database
// dump or a read-only database account therefore does not reveal the PLC or OPC UA server credentials.

// Sealer encrypts and decrypts connector secrets (iam.SecretBox).
type Sealer interface {
	Seal(plaintext []byte) ([]byte, error)
	Open(blob []byte) ([]byte, error)
}

var box Sealer

// SetSecretBox installs the key used for stored passwords; call it once at start, before Start.
func SetSecretBox(b Sealer) { box = b }

// SealStored returns the settings as they are written to the database: the password encrypted.
func SealStored(st Settings) (string, error) {
	pw := st.Password
	st.Password = ""
	b, err := json.Marshal(st)
	if err != nil {
		return "", err
	}
	if pw == "" {
		return string(b), nil
	}
	if box == nil {
		return "", errors.New("the secret key is not configured: the password cannot be stored")
	}
	enc, err := box.Seal([]byte(pw))
	if err != nil {
		return "", err
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		return "", err
	}
	m["password_enc"], _ = json.Marshal(base64.StdEncoding.EncodeToString(enc))
	out, err := json.Marshal(m)
	return string(out), err
}

// OpenStored returns stored settings with the password decrypted, for use in memory. Settings without an
// encrypted password come back unchanged (also a legacy plain-text password, until MigrateStored has run); a
// password that cannot be decrypted (another secret key) comes back empty, so the connector reports a login error.
func OpenStored(raw []byte) json.RawMessage {
	var m map[string]json.RawMessage
	if json.Unmarshal(raw, &m) != nil {
		return raw
	}
	encRaw, ok := m["password_enc"]
	if !ok {
		return raw
	}
	delete(m, "password_enc")
	var enc string
	if json.Unmarshal(encRaw, &enc) == nil && enc != "" && box != nil {
		if blob, err := base64.StdEncoding.DecodeString(enc); err == nil {
			if pw, err := box.Open(blob); err == nil {
				m["password"], _ = json.Marshal(string(pw))
			}
		}
	}
	out, err := json.Marshal(m)
	if err != nil {
		return raw
	}
	return out
}

// MigrateStored encrypts plain-text passwords written by versions before 0.90.2. It returns how many connectors
// were rewritten; it is safe to run at every start.
func (m *Manager) MigrateStored(ctx context.Context) (int, error) {
	if box == nil {
		return 0, errors.New("the secret key is not configured")
	}
	rows, err := m.pool.Query(ctx, `SELECT id::text, settings::text FROM core.connector WHERE kind = 'opcua' AND COALESCE(settings->>'password', '') <> ''`)
	if err != nil {
		return 0, err
	}
	type row struct{ id, raw string }
	var list []row
	for rows.Next() {
		var r row
		if rows.Scan(&r.id, &r.raw) == nil {
			list = append(list, r)
		}
	}
	rows.Close()
	n := 0
	for _, r := range list {
		var keep map[string]json.RawMessage
		if json.Unmarshal([]byte(r.raw), &keep) != nil {
			continue
		}
		var pw string
		if json.Unmarshal(keep["password"], &pw) != nil || pw == "" {
			continue
		}
		enc, err := box.Seal([]byte(pw))
		if err != nil {
			return n, err
		}
		// Only the two keys change, so settings this version does not know stay as they are.
		if _, err := m.pool.Exec(ctx, `UPDATE core.connector SET settings = (settings - 'password') || jsonb_build_object('password_enc', $2::text)
			WHERE id = $1::uuid AND settings->>'password' = $3`, r.id, base64.StdEncoding.EncodeToString(enc), pw); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}
