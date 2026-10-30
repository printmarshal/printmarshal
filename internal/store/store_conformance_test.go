package store

import (
	"testing"
)

// testStore is a Store under test plus the raw SQL access a few tests need
// to look behind the interface (that a secret is sealed on disk, or to
// back-date an alert). Queries use '?' placeholders; each backend adapts them
// and converts time.Time arguments to its own timestamp representation.
type testStore interface {
	Store
	rawExec(q string, args ...any) error
	rawRow(q string, args ...any) rowScanner
}

// runStoreTests is the conformance suite: every behaviour the rest of the
// code relies on, run against whichever backend open returns. SQLite runs it
// always (TestSQLite); PostgreSQL when PM_TEST_PG_DSN is set (TestPostgres).
func runStoreTests(t *testing.T, open func(t *testing.T) testStore) {
	t.Helper()
	for _, tc := range []struct {
		name string
		fn   func(t *testing.T, s testStore)
	}{
		{"OperatorFieldsSurviveScan", testOperatorFieldsSurviveScan},
		{"CredentialSecretsStayEncrypted", testCredentialSecretsStayEncrypted},
		{"AlertsLifecycle", testAlertsLifecycle},
		{"GroupsAndRules", testGroupsAndRules},
		{"JobsAndTelemetry", testJobsAndTelemetry},
		{"ConsumablesStockAndLinks", testConsumablesStockAndLinks},
		{"FamilyQualifications", testFamilyQualifications},
		{"ConfigStoreRoundTrip", testConfigStoreRoundTrip},
		{"FirmwareStoreRoundTrip", testFirmwareStoreRoundTrip},
		{"ReportAggregates", testReportAggregates},
		{"SecurityPoliciesCRUD", testSecurityPoliciesCRUD},
		{"AssessmentsLatestAndHistory", testAssessmentsLatestAndHistory},
		{"CAProfilesAndCertificates", testCAProfilesAndCertificates},
		{"UsersSessionsOutboxAudit", testUsersSessionsOutboxAudit},
		{"SecretSettings", testSecretSettings},
		{"APITokens", testAPITokens},
		{"CredentialPurposeResolution", testCredentialPurposeResolution},
		{"SyncDevicesWithTombstones", testSyncDevicesWithTombstones},
		{"SyncAuditAlertsJobs", testSyncAuditAlertsJobs},
		{"DeviceLogs", testDeviceLogs},
		{"IdentifyErrorsFollowLatestScan", testIdentifyErrorsFollowLatestScan},
	} {
		t.Run(tc.name, func(t *testing.T) { tc.fn(t, open(t)) })
	}
}
