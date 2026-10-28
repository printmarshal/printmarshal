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

const policyCols = `id,name,description,items,created_by,created_at,updated_at`

func scanPolicy(r rowScanner) (*model.Policy, error) {
	var p model.Policy
	var items, cr, up string
	if err := r.Scan(&p.ID, &p.Name, &p.Description, &items, &p.CreatedBy, &cr, &up); err != nil {
		return nil, err
	}
	p.Items = []model.PolicyItem{}
	fromJSON(sql.NullString{String: items, Valid: true}, &p.Items)
	p.CreatedAt, p.UpdatedAt = parseTime(cr), parseTime(up)
	return &p, nil
}

func (s *SQLite) CreatePolicy(ctx context.Context, p *model.Policy) error {
	if p.ID == "" {
		p.ID = newID()
	}
	n := now()
	p.CreatedAt, p.UpdatedAt = parseTime(n), parseTime(n)
	if p.Items == nil {
		p.Items = []model.PolicyItem{}
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO security_policies(`+policyCols+`) VALUES(?,?,?,?,?,?,?)`,
		p.ID, p.Name, p.Description, toJSON(p.Items), p.CreatedBy, n, n)
	return err
}

func (s *SQLite) UpdatePolicy(ctx context.Context, p *model.Policy) error {
	n := now()
	p.UpdatedAt = parseTime(n)
	if p.Items == nil {
		p.Items = []model.PolicyItem{}
	}
	res, err := s.db.ExecContext(ctx, `UPDATE security_policies SET name=?, description=?, items=?, updated_at=? WHERE id=?`,
		p.Name, p.Description, toJSON(p.Items), n, p.ID)
	if err != nil {
		return err
	}
	if k, _ := res.RowsAffected(); k == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *SQLite) GetPolicy(ctx context.Context, id string) (*model.Policy, error) {
	p, err := scanPolicy(s.db.QueryRowContext(ctx, `SELECT `+policyCols+` FROM security_policies WHERE id=?`, id))
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	return p, err
}

func (s *SQLite) ListPolicies(ctx context.Context) ([]*model.Policy, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+policyCols+` FROM security_policies ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*model.Policy{}
	for rows.Next() {
		p, err := scanPolicy(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *SQLite) DeletePolicy(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM security_policies WHERE id=?`, id)
	if err != nil {
		return err
	}
	if k, _ := res.RowsAffected(); k == 0 {
		return ErrNotFound
	}
	return nil
}

// --- assessments -------------------------------------------------------------

const assessmentCols = `id,device_id,policy_id,job_id,items,passed,failed,unknown,score,compliant,at`

func scanAssessment(r rowScanner) (*model.Assessment, error) {
	var a model.Assessment
	var items, at string
	var compliant int
	if err := r.Scan(&a.ID, &a.DeviceID, &a.PolicyID, &a.JobID, &items, &a.Passed, &a.Failed, &a.Unknown, &a.Score, &compliant, &at); err != nil {
		return nil, err
	}
	a.Items = []model.ItemResult{}
	fromJSON(sql.NullString{String: items, Valid: true}, &a.Items)
	a.Compliant = compliant == 1
	a.At = parseTime(at)
	return &a, nil
}

func (s *SQLite) SaveAssessment(ctx context.Context, a *model.Assessment) error {
	if a.ID == "" {
		a.ID = newID()
	}
	if a.At.IsZero() {
		a.At = time.Now().UTC()
	}
	if a.Items == nil {
		a.Items = []model.ItemResult{}
	}
	compliant := 0
	if a.Compliant {
		compliant = 1
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO security_assessments(`+assessmentCols+`) VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
		a.ID, a.DeviceID, a.PolicyID, a.JobID, toJSON(a.Items), a.Passed, a.Failed, a.Unknown, a.Score, compliant, ts(a.At))
	return err
}

func (s *SQLite) ListAssessments(ctx context.Context, f AssessmentFilter) ([]*model.Assessment, error) {
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
		args = append(args, ts(f.From))
	}
	if !f.To.IsZero() {
		where = append(where, "at<=?")
		args = append(args, ts(f.To))
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
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*model.Assessment{}
	for rows.Next() {
		a, err := scanAssessment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *SQLite) LatestAssessments(ctx context.Context, policyID string, deviceIDs []string) ([]*model.Assessment, error) {
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
	rows, err := s.db.QueryContext(ctx, `SELECT `+strings.Join(cols, ",")+` FROM security_assessments a WHERE `+strings.Join(where, " AND ")+` ORDER BY a.device_id, a.policy_id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*model.Assessment{}
	for rows.Next() {
		a, err := scanAssessment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *SQLite) ComplianceHistory(ctx context.Context, policyID string, from, to time.Time) ([]model.CompliancePoint, error) {
	where := []string{"1=1"}
	var args []any
	if policyID != "" {
		where = append(where, "policy_id=?")
		args = append(args, policyID)
	}
	if !from.IsZero() {
		where = append(where, "at>=?")
		args = append(args, ts(from))
	}
	if !to.IsZero() {
		where = append(where, "at<=?")
		args = append(args, ts(to))
	}
	rows, err := s.db.QueryContext(ctx, `SELECT substr(at,1,10) AS day, COUNT(*), SUM(compliant), AVG(score) FROM security_assessments WHERE `+
		strings.Join(where, " AND ")+` GROUP BY day ORDER BY day`, args...)
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

const caProfileCols = `id,name,protocol,url,credential_id,ca_fingerprint,validity_days,key_type,options,created_at,updated_at`

func scanCAProfile(r rowScanner) (*model.CAProfile, error) {
	var p model.CAProfile
	var opts, cr, up string
	if err := r.Scan(&p.ID, &p.Name, &p.Protocol, &p.URL, &p.CredentialID, &p.CAFingerprint, &p.ValidityDays, &p.KeyType, &opts, &cr, &up); err != nil {
		return nil, err
	}
	p.Options = map[string]string{}
	fromJSON(sql.NullString{String: opts, Valid: true}, &p.Options)
	p.CreatedAt, p.UpdatedAt = parseTime(cr), parseTime(up)
	return &p, nil
}

func (s *SQLite) CreateCAProfile(ctx context.Context, p *model.CAProfile) error {
	if p.ID == "" {
		p.ID = newID()
	}
	n := now()
	p.CreatedAt, p.UpdatedAt = parseTime(n), parseTime(n)
	if p.Options == nil {
		p.Options = map[string]string{}
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO ca_profiles(`+caProfileCols+`) VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
		p.ID, p.Name, string(p.Protocol), p.URL, p.CredentialID, p.CAFingerprint, p.ValidityDays, p.KeyType, toJSON(p.Options), n, n)
	return err
}

func (s *SQLite) UpdateCAProfile(ctx context.Context, p *model.CAProfile) error {
	n := now()
	p.UpdatedAt = parseTime(n)
	if p.Options == nil {
		p.Options = map[string]string{}
	}
	res, err := s.db.ExecContext(ctx, `UPDATE ca_profiles SET name=?, protocol=?, url=?, credential_id=?, ca_fingerprint=?, validity_days=?, key_type=?, options=?, updated_at=? WHERE id=?`,
		p.Name, string(p.Protocol), p.URL, p.CredentialID, p.CAFingerprint, p.ValidityDays, p.KeyType, toJSON(p.Options), n, p.ID)
	if err != nil {
		return err
	}
	if k, _ := res.RowsAffected(); k == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *SQLite) GetCAProfile(ctx context.Context, id string) (*model.CAProfile, error) {
	p, err := scanCAProfile(s.db.QueryRowContext(ctx, `SELECT `+caProfileCols+` FROM ca_profiles WHERE id=?`, id))
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	return p, err
}

func (s *SQLite) ListCAProfiles(ctx context.Context) ([]*model.CAProfile, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+caProfileCols+` FROM ca_profiles ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*model.CAProfile{}
	for rows.Next() {
		p, err := scanCAProfile(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *SQLite) DeleteCAProfile(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM ca_profiles WHERE id=?`, id)
	if err != nil {
		return err
	}
	if k, _ := res.RowsAffected(); k == 0 {
		return ErrNotFound
	}
	return nil
}

// --- issued certificates -----------------------------------------------------

const issuedCols = `id,device_id,ca_profile_id,protocol,use,subject,issuer,serial,not_before,not_after,fingerprint,key_type,cert_pem,chain_pem,installed,installed_by,installed_at,job_id,created_at`

func scanIssued(r rowScanner) (*model.IssuedCertificate, error) {
	var c model.IssuedCertificate
	var nb, na, cr string
	var installed int
	var installedAt sql.NullString
	if err := r.Scan(&c.ID, &c.DeviceID, &c.CAProfileID, &c.Protocol, &c.Use, &c.Subject, &c.Issuer, &c.Serial, &nb, &na, &c.Fingerprint, &c.KeyType,
		&c.CertPEM, &c.ChainPEM, &installed, &c.InstalledBy, &installedAt, &c.JobID, &cr); err != nil {
		return nil, err
	}
	c.NotBefore, c.NotAfter, c.CreatedAt = parseTime(nb), parseTime(na), parseTime(cr)
	c.Installed = installed == 1
	c.InstalledAt = parseTimeP(installedAt)
	return &c, nil
}

func (s *SQLite) SaveIssuedCertificate(ctx context.Context, c *model.IssuedCertificate, keyPEM string) error {
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
	installed := 0
	if c.Installed {
		installed = 1
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO issued_certificates(`+issuedCols+`,key_enc) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		c.ID, c.DeviceID, c.CAProfileID, c.Protocol, c.Use, c.Subject, c.Issuer, c.Serial, ts(c.NotBefore), ts(c.NotAfter), c.Fingerprint, c.KeyType,
		c.CertPEM, c.ChainPEM, installed, c.InstalledBy, tsp(c.InstalledAt), c.JobID, ts(c.CreatedAt), keyEnc)
	return err
}

func (s *SQLite) GetIssuedCertificate(ctx context.Context, id string) (*model.IssuedCertificate, error) {
	c, err := scanIssued(s.db.QueryRowContext(ctx, `SELECT `+issuedCols+` FROM issued_certificates WHERE id=?`, id))
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	return c, err
}

func (s *SQLite) ListIssuedCertificates(ctx context.Context, deviceID string) ([]*model.IssuedCertificate, error) {
	q := `SELECT ` + issuedCols + ` FROM issued_certificates`
	var args []any
	if deviceID != "" {
		q += ` WHERE device_id=?`
		args = append(args, deviceID)
	}
	rows, err := s.db.QueryContext(ctx, q+` ORDER BY created_at DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*model.IssuedCertificate{}
	for rows.Next() {
		c, err := scanIssued(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *SQLite) IssuedCertificateKey(ctx context.Context, id string) (string, error) {
	var enc []byte
	err := s.db.QueryRowContext(ctx, `SELECT key_enc FROM issued_certificates WHERE id=?`, id).Scan(&enc)
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

func (s *SQLite) MarkCertificateInstalled(ctx context.Context, id, driver string, at time.Time) error {
	res, err := s.db.ExecContext(ctx, `UPDATE issued_certificates SET installed=1, installed_by=?, installed_at=? WHERE id=?`, driver, ts(at), id)
	if err != nil {
		return err
	}
	if k, _ := res.RowsAffected(); k == 0 {
		return ErrNotFound
	}
	return nil
}

// --- observed device certificates -------------------------------------------

func (s *SQLite) SaveDeviceCertificates(ctx context.Context, deviceID, source string, certs []model.Certificate, seenAt time.Time) error {
	if seenAt.IsZero() {
		seenAt = time.Now().UTC()
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM device_certificates WHERE device_id=? AND source=?`, deviceID, source); err != nil {
		return err
	}
	for i, c := range certs {
		id := c.ID
		if id == "" {
			id = fmt.Sprintf("%s:%d", source, i)
		}
		selfSigned := 0
		if c.SelfSigned {
			selfSigned = 1
		}
		if _, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO device_certificates(device_id,source,cert_id,use,subject,issuer,serial,not_before,not_after,fingerprint,self_signed,key_type,seen_at)
			VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`, deviceID, source, id, c.Use, c.Subject, c.Issuer, c.Serial, ts(c.NotBefore), ts(c.NotAfter), c.Fingerprint, selfSigned, c.KeyType, ts(seenAt)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *SQLite) ListDeviceCertificates(ctx context.Context, deviceID string) ([]model.DeviceCertificate, error) {
	q := `SELECT device_id,source,cert_id,use,subject,issuer,serial,not_before,not_after,fingerprint,self_signed,key_type,seen_at FROM device_certificates`
	var args []any
	if deviceID != "" {
		q += ` WHERE device_id=?`
		args = append(args, deviceID)
	}
	rows, err := s.db.QueryContext(ctx, q+` ORDER BY device_id, source, cert_id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.DeviceCertificate{}
	for rows.Next() {
		var c model.DeviceCertificate
		var nb, na, seen string
		var selfSigned int
		if err := rows.Scan(&c.DeviceID, &c.Source, &c.ID, &c.Use, &c.Subject, &c.Issuer, &c.Serial, &nb, &na, &c.Fingerprint, &selfSigned, &c.KeyType, &seen); err != nil {
			return nil, err
		}
		c.NotBefore, c.NotAfter, c.SeenAt = parseTime(nb), parseTime(na), parseTime(seen)
		c.SelfSigned = selfSigned == 1
		out = append(out, c)
	}
	return out, rows.Err()
}

// --- credential secret (internal use only) ----------------------------------

func (s *SQLite) CredentialSecret(ctx context.Context, id string) (*model.Credential, *model.Secret, error) {
	if s.box == nil {
		return nil, nil, fmt.Errorf("credential store has no encryption key")
	}
	var c model.Credential
	var enc []byte
	var cr, up string
	err := s.db.QueryRowContext(ctx, `SELECT id,name,type,scope,username,priority,secret,created_at,updated_at FROM credentials WHERE id=?`, id).
		Scan(&c.ID, &c.Name, &c.Type, &c.Scope, &c.Username, &c.Priority, &enc, &cr, &up)
	if err == sql.ErrNoRows {
		return nil, nil, ErrNotFound
	}
	if err != nil {
		return nil, nil, err
	}
	c.CreatedAt, c.UpdatedAt = parseTime(cr), parseTime(up)
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
