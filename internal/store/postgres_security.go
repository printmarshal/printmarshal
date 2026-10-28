package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"printmarshal/internal/model"
)

// --- policies ----------------------------------------------------------------

func scanPolicyPG(r rowScanner) (*model.Policy, error) {
	var p model.Policy
	var items string
	var cr, up time.Time
	if err := r.Scan(&p.ID, &p.Name, &p.Description, &items, &p.CreatedBy, &cr, &up); err != nil {
		return nil, err
	}
	p.Items = []model.PolicyItem{}
	fromJSON(sql.NullString{String: items, Valid: true}, &p.Items)
	p.CreatedAt, p.UpdatedAt = cr.UTC(), up.UTC()
	return &p, nil
}

func (s *Postgres) CreatePolicy(ctx context.Context, p *model.Policy) error {
	if p.ID == "" {
		p.ID = newID()
	}
	n := nowUTC()
	p.CreatedAt, p.UpdatedAt = n, n
	if p.Items == nil {
		p.Items = []model.PolicyItem{}
	}
	_, err := pgExec(ctx, s.db, `INSERT INTO security_policies(`+policyCols+`) VALUES(?,?,?,?,?,?,?)`,
		p.ID, p.Name, p.Description, toJSON(p.Items), p.CreatedBy, n, n)
	return err
}

func (s *Postgres) UpdatePolicy(ctx context.Context, p *model.Policy) error {
	n := nowUTC()
	p.UpdatedAt = n
	if p.Items == nil {
		p.Items = []model.PolicyItem{}
	}
	return affected(pgExec(ctx, s.db, `UPDATE security_policies SET name=?, description=?, items=?, updated_at=? WHERE id=?`,
		p.Name, p.Description, toJSON(p.Items), n, p.ID))
}

func (s *Postgres) GetPolicy(ctx context.Context, id string) (*model.Policy, error) {
	p, err := scanPolicyPG(pgRow(ctx, s.db, `SELECT `+policyCols+` FROM security_policies WHERE id=?`, id))
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	return p, err
}

func (s *Postgres) ListPolicies(ctx context.Context) ([]*model.Policy, error) {
	rows, err := pgQuery(ctx, s.db, `SELECT `+policyCols+` FROM security_policies ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*model.Policy{}
	for rows.Next() {
		p, err := scanPolicyPG(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Postgres) DeletePolicy(ctx context.Context, id string) error {
	return affected(pgExec(ctx, s.db, `DELETE FROM security_policies WHERE id=?`, id))
}

// --- assessments -------------------------------------------------------------

func scanAssessmentPG(r rowScanner) (*model.Assessment, error) {
	var a model.Assessment
	var items string
	var at time.Time
	if err := r.Scan(&a.ID, &a.DeviceID, &a.PolicyID, &a.JobID, &items, &a.Passed, &a.Failed, &a.Unknown, &a.Score, &a.Compliant, &at); err != nil {
		return nil, err
	}
	a.Items = []model.ItemResult{}
	fromJSON(sql.NullString{String: items, Valid: true}, &a.Items)
	a.At = at.UTC()
	return &a, nil
}

func (s *Postgres) SaveAssessment(ctx context.Context, a *model.Assessment) error {
	if a.ID == "" {
		a.ID = newID()
	}
	if a.At.IsZero() {
		a.At = time.Now().UTC()
	}
	if a.Items == nil {
		a.Items = []model.ItemResult{}
	}
	_, err := pgExec(ctx, s.db, `INSERT INTO security_assessments(`+assessmentCols+`) VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
		a.ID, a.DeviceID, a.PolicyID, a.JobID, toJSON(a.Items), a.Passed, a.Failed, a.Unknown, a.Score, a.Compliant, pgt(a.At))
	return err
}

func (s *Postgres) ListAssessments(ctx context.Context, f AssessmentFilter) ([]*model.Assessment, error) {
	var where []string
	var args []any
	if f.DeviceID != "" {
		where = append(where, "device_id=?")
		args = append(args, f.DeviceID)
	}
	if f.PolicyID != "" {
		where = append(where, "policy_id=?")
		args = append(args, f.PolicyID)
	}
	if !f.From.IsZero() {
		where = append(where, "at>=?")
		args = append(args, pgt(f.From))
	}
	if !f.To.IsZero() {
		where = append(where, "at<=?")
		args = append(args, pgt(f.To))
	}
	q := `SELECT ` + assessmentCols + ` FROM security_assessments`
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	q += " ORDER BY at DESC"
	limit := 500
	if f.Limit > 0 {
		limit = f.Limit
	}
	q += fmt.Sprintf(" LIMIT %d", limit)
	rows, err := pgQuery(ctx, s.db, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*model.Assessment{}
	for rows.Next() {
		a, err := scanAssessmentPG(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Postgres) LatestAssessments(ctx context.Context, policyID string, deviceIDs []string) ([]*model.Assessment, error) {
	where := []string{`a.at = (SELECT MAX(b.at) FROM security_assessments b WHERE b.device_id=a.device_id AND b.policy_id=a.policy_id)`}
	var args []any
	if policyID != "" {
		where = append(where, "a.policy_id=?")
		args = append(args, policyID)
	}
	if len(deviceIDs) > 0 {
		where = append(where, "a.device_id IN ("+placeholders(len(deviceIDs))+")")
		args = append(args, toAny(deviceIDs)...)
	}
	cols := make([]string, 0, 11)
	for _, c := range strings.Split(assessmentCols, ",") {
		cols = append(cols, "a."+c)
	}
	rows, err := pgQuery(ctx, s.db, `SELECT `+strings.Join(cols, ",")+` FROM security_assessments a WHERE `+strings.Join(where, " AND ")+` ORDER BY a.device_id, a.policy_id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*model.Assessment{}
	for rows.Next() {
		a, err := scanAssessmentPG(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Postgres) ComplianceHistory(ctx context.Context, policyID string, from, to time.Time) ([]model.CompliancePoint, error) {
	where := []string{"1=1"}
	var args []any
	if policyID != "" {
		where = append(where, "policy_id=?")
		args = append(args, policyID)
	}
	if !from.IsZero() {
		where = append(where, "at>=?")
		args = append(args, pgt(from))
	}
	if !to.IsZero() {
		where = append(where, "at<=?")
		args = append(args, pgt(to))
	}
	rows, err := pgQuery(ctx, s.db, `SELECT `+pgDay+` AS day, COUNT(*), COUNT(*) FILTER (WHERE compliant), AVG(score)::float8 FROM security_assessments WHERE `+
		strings.Join(where, " AND ")+` GROUP BY 1 ORDER BY 1`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.CompliancePoint{}
	for rows.Next() {
		var p model.CompliancePoint
		if err := rows.Scan(&p.Date, &p.Assessed, &p.Compliant, &p.AvgScore); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// --- CA profiles -------------------------------------------------------------

func scanCAProfilePG(r rowScanner) (*model.CAProfile, error) {
	var p model.CAProfile
	var opts string
	var cr, up time.Time
	if err := r.Scan(&p.ID, &p.Name, &p.Protocol, &p.URL, &p.CredentialID, &p.CAFingerprint, &p.ValidityDays, &p.KeyType, &opts, &cr, &up); err != nil {
		return nil, err
	}
	p.Options = map[string]string{}
	fromJSON(sql.NullString{String: opts, Valid: true}, &p.Options)
	p.CreatedAt, p.UpdatedAt = cr.UTC(), up.UTC()
	return &p, nil
}

func (s *Postgres) CreateCAProfile(ctx context.Context, p *model.CAProfile) error {
	if p.ID == "" {
		p.ID = newID()
	}
	n := nowUTC()
	p.CreatedAt, p.UpdatedAt = n, n
	if p.Options == nil {
		p.Options = map[string]string{}
	}
	_, err := pgExec(ctx, s.db, `INSERT INTO ca_profiles(`+caProfileCols+`) VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
		p.ID, p.Name, string(p.Protocol), p.URL, p.CredentialID, p.CAFingerprint, p.ValidityDays, p.KeyType, toJSON(p.Options), n, n)
	return err
}

func (s *Postgres) UpdateCAProfile(ctx context.Context, p *model.CAProfile) error {
	n := nowUTC()
	p.UpdatedAt = n
	if p.Options == nil {
		p.Options = map[string]string{}
	}
	return affected(pgExec(ctx, s.db, `UPDATE ca_profiles SET name=?, protocol=?, url=?, credential_id=?, ca_fingerprint=?, validity_days=?, key_type=?, options=?, updated_at=? WHERE id=?`,
		p.Name, string(p.Protocol), p.URL, p.CredentialID, p.CAFingerprint, p.ValidityDays, p.KeyType, toJSON(p.Options), n, p.ID))
}

func (s *Postgres) GetCAProfile(ctx context.Context, id string) (*model.CAProfile, error) {
	p, err := scanCAProfilePG(pgRow(ctx, s.db, `SELECT `+caProfileCols+` FROM ca_profiles WHERE id=?`, id))
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	return p, err
}

func (s *Postgres) ListCAProfiles(ctx context.Context) ([]*model.CAProfile, error) {
	rows, err := pgQuery(ctx, s.db, `SELECT `+caProfileCols+` FROM ca_profiles ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*model.CAProfile{}
	for rows.Next() {
		p, err := scanCAProfilePG(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Postgres) DeleteCAProfile(ctx context.Context, id string) error {
	return affected(pgExec(ctx, s.db, `DELETE FROM ca_profiles WHERE id=?`, id))
}

// --- issued certificates -----------------------------------------------------

func scanIssuedPG(r rowScanner) (*model.IssuedCertificate, error) {
	var c model.IssuedCertificate
	var nb, na, cr time.Time
	var installedAt sql.NullTime
	if err := r.Scan(&c.ID, &c.DeviceID, &c.CAProfileID, &c.Protocol, &c.Use, &c.Subject, &c.Issuer, &c.Serial, &nb, &na, &c.Fingerprint, &c.KeyType,
		&c.CertPEM, &c.ChainPEM, &c.Installed, &c.InstalledBy, &installedAt, &c.JobID, &cr); err != nil {
		return nil, err
	}
	c.NotBefore, c.NotAfter, c.CreatedAt = nb.UTC(), na.UTC(), cr.UTC()
	c.InstalledAt = timeOf(installedAt)
	return &c, nil
}

func (s *Postgres) SaveIssuedCertificate(ctx context.Context, c *model.IssuedCertificate, keyPEM string) error {
	if c.ID == "" {
		c.ID = newID()
	}
	if c.CreatedAt.IsZero() {
		c.CreatedAt = time.Now().UTC()
	}
	var keyEnc []byte
	if keyPEM != "" {
		if s.box == nil {
			return fmt.Errorf("certificate store has no encryption key")
		}
		enc, err := s.box.Seal([]byte(keyPEM))
		if err != nil {
			return err
		}
		keyEnc = enc
	}
	_, err := pgExec(ctx, s.db, `INSERT INTO issued_certificates(`+issuedCols+`,key_enc) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		c.ID, c.DeviceID, c.CAProfileID, c.Protocol, c.Use, c.Subject, c.Issuer, c.Serial, pgt(c.NotBefore), pgt(c.NotAfter), c.Fingerprint, c.KeyType,
		c.CertPEM, c.ChainPEM, c.Installed, c.InstalledBy, pgtp(c.InstalledAt), c.JobID, pgt(c.CreatedAt), keyEnc)
	return err
}

func (s *Postgres) GetIssuedCertificate(ctx context.Context, id string) (*model.IssuedCertificate, error) {
	c, err := scanIssuedPG(pgRow(ctx, s.db, `SELECT `+issuedCols+` FROM issued_certificates WHERE id=?`, id))
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	return c, err
}

func (s *Postgres) ListIssuedCertificates(ctx context.Context, deviceID string) ([]*model.IssuedCertificate, error) {
	q := `SELECT ` + issuedCols + ` FROM issued_certificates`
	var args []any
	if deviceID != "" {
		q += ` WHERE device_id=?`
		args = append(args, deviceID)
	}
	rows, err := pgQuery(ctx, s.db, q+` ORDER BY created_at DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*model.IssuedCertificate{}
	for rows.Next() {
		c, err := scanIssuedPG(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Postgres) IssuedCertificateKey(ctx context.Context, id string) (string, error) {
	var enc []byte
	err := pgRow(ctx, s.db, `SELECT key_enc FROM issued_certificates WHERE id=?`, id).Scan(&enc)
	if err == sql.ErrNoRows {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	if len(enc) == 0 {
		return "", nil
	}
	if s.box == nil {
		return "", fmt.Errorf("certificate store has no encryption key")
	}
	plain, err := s.box.Open(enc)
	if err != nil {
		return "", fmt.Errorf("decrypt certificate key: %w", err)
	}
	return string(plain), nil
}

func (s *Postgres) MarkCertificateInstalled(ctx context.Context, id, driver string, at time.Time) error {
	return affected(pgExec(ctx, s.db, `UPDATE issued_certificates SET installed=TRUE, installed_by=?, installed_at=? WHERE id=?`, driver, pgt(at), id))
}

// --- observed device certificates -------------------------------------------

func (s *Postgres) SaveDeviceCertificates(ctx context.Context, deviceID, source string, certs []model.Certificate, seenAt time.Time) error {
	if seenAt.IsZero() {
		seenAt = time.Now().UTC()
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := pgExec(ctx, tx, `DELETE FROM device_certificates WHERE device_id=? AND source=?`, deviceID, source); err != nil {
		return err
	}
	for i, c := range certs {
		id := c.ID
		if id == "" {
			id = fmt.Sprintf("%s:%d", source, i)
		}
		if _, err := pgExec(ctx, tx, `INSERT INTO device_certificates(device_id,source,cert_id,use,subject,issuer,serial,not_before,not_after,fingerprint,self_signed,key_type,seen_at)
			VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)
			ON CONFLICT(device_id,source,cert_id) DO UPDATE SET use=excluded.use, subject=excluded.subject, issuer=excluded.issuer, serial=excluded.serial,
			not_before=excluded.not_before, not_after=excluded.not_after, fingerprint=excluded.fingerprint, self_signed=excluded.self_signed,
			key_type=excluded.key_type, seen_at=excluded.seen_at`,
			deviceID, source, id, c.Use, c.Subject, c.Issuer, c.Serial, pgt(c.NotBefore), pgt(c.NotAfter), c.Fingerprint, c.SelfSigned, c.KeyType, pgt(seenAt)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Postgres) ListDeviceCertificates(ctx context.Context, deviceID string) ([]model.DeviceCertificate, error) {
	q := `SELECT device_id,source,cert_id,use,subject,issuer,serial,not_before,not_after,fingerprint,self_signed,key_type,seen_at FROM device_certificates`
	var args []any
	if deviceID != "" {
		q += ` WHERE device_id=?`
		args = append(args, deviceID)
	}
	rows, err := pgQuery(ctx, s.db, q+` ORDER BY device_id, source, cert_id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.DeviceCertificate{}
	for rows.Next() {
		var c model.DeviceCertificate
		var nb, na, seen time.Time
		if err := rows.Scan(&c.DeviceID, &c.Source, &c.ID, &c.Use, &c.Subject, &c.Issuer, &c.Serial, &nb, &na, &c.Fingerprint, &c.SelfSigned, &c.KeyType, &seen); err != nil {
			return nil, err
		}
		c.NotBefore, c.NotAfter, c.SeenAt = nb.UTC(), na.UTC(), seen.UTC()
		out = append(out, c)
	}
	return out, rows.Err()
}

// --- credential secret (internal use only) ----------------------------------

func (s *Postgres) CredentialSecret(ctx context.Context, id string) (*model.Credential, *model.Secret, error) {
	if s.box == nil {
		return nil, nil, fmt.Errorf("credential store has no encryption key")
	}
	var c model.Credential
	var enc []byte
	var cr, up time.Time
	err := pgRow(ctx, s.db, `SELECT id,name,type,scope,username,priority,secret,created_at,updated_at FROM credentials WHERE id=?`, id).
		Scan(&c.ID, &c.Name, &c.Type, &c.Scope, &c.Username, &c.Priority, &enc, &cr, &up)
	if err == sql.ErrNoRows {
		return nil, nil, ErrNotFound
	}
	if err != nil {
		return nil, nil, err
	}
	c.CreatedAt, c.UpdatedAt = cr.UTC(), up.UTC()
	plain, err := s.box.Open(enc)
	if err != nil {
		return nil, nil, fmt.Errorf("decrypt credential: %w", err)
	}
	var sec model.Secret
	if err := json.Unmarshal(plain, &sec); err != nil {
		return nil, nil, err
	}
	return &c, &sec, nil
}
