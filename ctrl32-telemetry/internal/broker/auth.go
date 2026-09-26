package broker

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	// ProvisionUser is the MQTT user name of a provisioning session (ThingsBoard convention).
	ProvisionUser = "provision"
	// AccountPrefix starts the user name of every MQTT account (spec 005).
	AccountPrefix = "hs_"
)

// Account is an MQTT account (connector of kind mqtt_account): its clients use free topics inside Namespace.
type Account struct {
	ConnectorID string
	OrgID       string
	Username    string
	Namespace   string // "h/{12 hex}"
}

// Authenticator resolves MQTT credentials (a database in production, a fake in tests).
type Authenticator interface {
	Device(ctx context.Context, externalID, token string) (*Device, error)
	DeviceByToken(ctx context.Context, token string) (*Device, error)
	Account(ctx context.Context, username, password string) (*Account, error)
}

// SetAuthenticator replaces the credential lookup (tests).
func (b *Broker) SetAuthenticator(a Authenticator) { b.auth = a }

type dbAuth struct{ pool *pgxpool.Pool }

const deviceAuthSQL = `
		SELECT d.id::text, d.org_id::text, d.external_id, d.transport
		FROM core.device d
		JOIN core.organization o ON o.id = d.org_id
		JOIN core.device_credential c ON c.device_id = d.id
		WHERE d.revoked_at IS NULL AND o.disabled_at IS NULL AND (d.approval IS NULL OR d.approval = 'approved')
		  AND c.kind = 'token' AND c.secret_hash = $1 AND c.revoked_at IS NULL
		  AND (c.valid_to IS NULL OR c.valid_to > now())`

func (a dbAuth) Device(ctx context.Context, externalID, token string) (*Device, error) {
	var d Device
	err := a.pool.QueryRow(ctx, deviceAuthSQL+` AND d.external_id = $2 LIMIT 1`, TokenHash(token), externalID).Scan(&d.ID, &d.OrgID, &d.ExternalID, &d.Transport)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func (a dbAuth) DeviceByToken(ctx context.Context, token string) (*Device, error) {
	var d Device
	err := a.pool.QueryRow(ctx, deviceAuthSQL+` LIMIT 1`, TokenHash(token)).Scan(&d.ID, &d.OrgID, &d.ExternalID, &d.Transport)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func (a dbAuth) Account(ctx context.Context, username, password string) (*Account, error) {
	acc := Account{Username: username}
	err := a.pool.QueryRow(ctx, `
		SELECT c.id::text, c.org_id::text, c.settings->>'namespace'
		FROM core.connector c JOIN core.organization o ON o.id = c.org_id
		WHERE c.kind = 'mqtt_account' AND c.settings->>'username' = $1 AND c.secret_hash = $2
		  AND c.revoked_at IS NULL AND o.disabled_at IS NULL AND c.settings->>'namespace' LIKE 'h/%'`,
		username, TokenHash(password)).Scan(&acc.ConnectorID, &acc.OrgID, &acc.Namespace)
	if err != nil {
		return nil, err
	}
	return &acc, nil
}
