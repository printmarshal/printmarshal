package store

import (
	"context"
	"time"

	"printmarshal/internal/model"
)

// AssessmentFilter selects assessments.
type AssessmentFilter struct {
	DeviceID string
	PolicyID string
	From     time.Time
	To       time.Time
	Limit    int
}

// SecurityStore is the persistence surface for the security area (spec §8.5,
// §9). It is defined here (own file), implemented on *SQLite in
// sqlite_security.go, with tables in schema_security.sql. Keep it additive.
type SecurityStore interface {
	// Policies
	CreatePolicy(ctx context.Context, p *model.Policy) error
	UpdatePolicy(ctx context.Context, p *model.Policy) error
	GetPolicy(ctx context.Context, id string) (*model.Policy, error)
	ListPolicies(ctx context.Context) ([]*model.Policy, error)
	DeletePolicy(ctx context.Context, id string) error

	// Assessments (compliance results over time)
	SaveAssessment(ctx context.Context, a *model.Assessment) error
	ListAssessments(ctx context.Context, f AssessmentFilter) ([]*model.Assessment, error)
	// LatestAssessments returns the newest assessment per (device, policy).
	// policyID "" = every policy; deviceIDs nil = every device.
	LatestAssessments(ctx context.Context, policyID string, deviceIDs []string) ([]*model.Assessment, error)
	// ComplianceHistory aggregates a policy's assessments per day (reports).
	ComplianceHistory(ctx context.Context, policyID string, from, to time.Time) ([]model.CompliancePoint, error)

	// CA profiles
	CreateCAProfile(ctx context.Context, p *model.CAProfile) error
	UpdateCAProfile(ctx context.Context, p *model.CAProfile) error
	GetCAProfile(ctx context.Context, id string) (*model.CAProfile, error)
	ListCAProfiles(ctx context.Context) ([]*model.CAProfile, error)
	DeleteCAProfile(ctx context.Context, id string) error

	// Issued certificates. keyPEM is encrypted at rest and only readable
	// through IssuedCertificateKey (used by the install job, never by the API).
	SaveIssuedCertificate(ctx context.Context, c *model.IssuedCertificate, keyPEM string) error
	GetIssuedCertificate(ctx context.Context, id string) (*model.IssuedCertificate, error)
	ListIssuedCertificates(ctx context.Context, deviceID string) ([]*model.IssuedCertificate, error)
	IssuedCertificateKey(ctx context.Context, id string) (string, error)
	MarkCertificateInstalled(ctx context.Context, id, driver string, at time.Time) error

	// Observed device certificates (from drivers or the TLS probe), kept so
	// expiry checks need no network I/O.
	SaveDeviceCertificates(ctx context.Context, deviceID, source string, certs []model.Certificate, seenAt time.Time) error
	ListDeviceCertificates(ctx context.Context, deviceID string) ([]model.DeviceCertificate, error)

	// CredentialSecret decrypts one credential for the enrollment and rotation
	// code paths (CA challenge passwords, EST credentials, ACME account keys).
	// It is never reachable from the API.
	CredentialSecret(ctx context.Context, id string) (*model.Credential, *model.Secret, error)
}
