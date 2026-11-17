// tests/organizations/provenanceBackfill/provenanceBackfill_test.go
//
// The one-off backfill marks the memberships whose organization created the
// account through the import, from the signals the import leaves on the
// identity provider's account. Refs #599.
package provenancebackfill_tests

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"soli/formations/src/organizations/models"
	"soli/formations/src/organizations/services"
	testTools "soli/formations/tests/testTools"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type accountList []*casdoorsdk.User

func (a accountList) GetUsers() ([]*casdoorsdk.User, error) { return a, nil }

func newDB() *gorm.DB {
	return testTools.MemoryDB(&models.Organization{}, &models.OrganizationMember{})
}

func seedOrg(t *testing.T, db *gorm.DB, orgType models.OrganizationType) uuid.UUID {
	t.Helper()
	org := &models.Organization{Name: "org-" + uuid.NewString()[:8], DisplayName: "Org", OwnerUserID: "owner", OrganizationType: orgType, IsActive: true}
	org.ID = uuid.New()
	require.NoError(t, db.Omit("Metadata", "OwnerIDs", "Members", "Groups").Create(org).Error)
	return org.ID
}

func seedRow(t *testing.T, db *gorm.DB, orgID uuid.UUID, userID string, joinedAt, createdAt time.Time) {
	t.Helper()
	m := &models.OrganizationMember{OrganizationID: orgID, UserID: userID, Role: models.OrgRoleMember, JoinedAt: joinedAt, IsActive: true}
	m.CreatedAt = createdAt
	require.NoError(t, db.Omit("Metadata").Create(m).Error)
}

// offboardedRow turns the row into one offboarded with an erasure date and the block stamp.
func offboardedRow(t *testing.T, db *gorm.DB, orgID uuid.UUID, userID string) {
	t.Helper()
	require.NoError(t, db.Model(&models.OrganizationMember{}).Where("organization_id = ? AND user_id = ?", orgID, userID).
		Updates(map[string]any{"is_active": false, "left_at": time.Now(), "scheduled_erasure_at": time.Now().AddDate(0, 0, 30),
			"account_forbidden_by_offboarding": true}).Error)
}

// importedAccount looks like an account the import created at createdAt; extra
// properties are added to the ones the import sets.
func importedAccount(id string, createdAt time.Time, extra map[string]string) *casdoorsdk.User {
	props := map[string]string{"tos_accepted_at": createdAt.Format(time.RFC3339), "tos_version": createdAt.Format("2006-01-02")}
	for k, v := range extra {
		props[k] = v
	}
	return &casdoorsdk.User{Id: id, Name: fmt.Sprintf("ada-lovelace-%d", createdAt.Unix()), CreatedTime: createdAt.Format(time.RFC3339), Properties: props}
}

func load(t *testing.T, db *gorm.DB, orgID uuid.UUID, userID string) models.OrganizationMember {
	t.Helper()
	var m models.OrganizationMember
	require.NoError(t, db.Where("organization_id = ? AND user_id = ?", orgID, userID).First(&m).Error)
	return m
}

func ref(orgID uuid.UUID, userID string) services.MemberRef {
	return services.MemberRef{OrganizationID: orgID, UserID: userID}
}

func TestBackfillProvenance_ImportedAccount_Marked(t *testing.T) {
	db := newDB()
	created := time.Now().Add(-48 * time.Hour).Truncate(time.Second)
	importer := seedOrg(t, db, models.OrgTypeTeam)
	laterOrg := seedOrg(t, db, models.OrgTypeTeam)
	personal := seedOrg(t, db, models.OrgTypePersonal)
	seedRow(t, db, importer, "u1", created.Add(30*time.Second), created.Add(30*time.Second))
	seedRow(t, db, personal, "u1", created.Add(31*time.Second), created.Add(31*time.Second))
	seedRow(t, db, laterOrg, "u1", created.Add(24*time.Hour), created.Add(24*time.Hour))

	report, err := services.BackfillAccountProvenance(db, accountList{importedAccount("u1", created, nil)}, time.Now(), false)
	require.NoError(t, err)

	assert.True(t, load(t, db, importer, "u1").AccountCreatedByOrganization, "the importing organization created the account")
	assert.False(t, load(t, db, laterOrg, "u1").AccountCreatedByOrganization, "a membership joined later is an attachment")
	assert.False(t, load(t, db, personal, "u1").AccountCreatedByOrganization, "only team organizations are marked")
	assert.Equal(t, []services.MemberRef{ref(importer, "u1")}, report.Marked)
	assert.Contains(t, report.LeftUnmarked, ref(laterOrg, "u1"))
}

func TestBackfillProvenance_SelfSignupAttached_NotMarked(t *testing.T) {
	db := newDB()
	created := time.Now().Add(-48 * time.Hour).Truncate(time.Second)
	orgID := seedOrg(t, db, models.OrgTypeTeam)
	seedRow(t, db, orgID, "u1", created.Add(30*time.Second), created.Add(30*time.Second))
	offboardedRow(t, db, orgID, "u1")

	report, err := services.BackfillAccountProvenance(db,
		accountList{importedAccount("u1", created, map[string]string{"username": "ada"})}, time.Now(), false)
	require.NoError(t, err)

	m := load(t, db, orgID, "u1")
	assert.False(t, m.AccountCreatedByOrganization, "self-signup sets the username property; the import never does")
	assert.Nil(t, m.ScheduledErasureAt, "an attached member's erasure date is cleared")
	assert.Contains(t, report.LeftUnmarked, ref(orgID, "u1"))
	assert.Contains(t, report.ErasureDatesCleared, ref(orgID, "u1"))
	assert.Contains(t, report.BlockedAccounts, ref(orgID, "u1"), "a block set by an organization that did not create the account is listed for an operator")
	assert.Empty(t, report.Marked)
}

func TestBackfillProvenance_AdminCreated_NotMarked(t *testing.T) {
	db := newDB()
	created := time.Now().Add(-48 * time.Hour).Truncate(time.Second)
	orgID := seedOrg(t, db, models.OrgTypeTeam)
	seedRow(t, db, orgID, "u1", created.Add(30*time.Second), created.Add(30*time.Second))

	report, err := services.BackfillAccountProvenance(db,
		accountList{importedAccount("u1", created, map[string]string{"created_by_admin": "true"})}, time.Now(), false)
	require.NoError(t, err)

	assert.False(t, load(t, db, orgID, "u1").AccountCreatedByOrganization)
	assert.Empty(t, report.Marked)
}

func TestBackfillProvenance_RowAfterCutoff_Untouched(t *testing.T) {
	db := newDB()
	created := time.Now().Truncate(time.Second)
	orgID := seedOrg(t, db, models.OrgTypeTeam)
	seedRow(t, db, orgID, "u1", created.Add(30*time.Second), created.Add(30*time.Second))
	offboardedRow(t, db, orgID, "u1")
	before := load(t, db, orgID, "u1")

	report, err := services.BackfillAccountProvenance(db, accountList{importedAccount("u1", created, nil)}, time.Now().Add(-time.Hour), false)
	require.NoError(t, err)

	after := load(t, db, orgID, "u1")
	assert.False(t, after.AccountCreatedByOrganization, "rows written after the release record their provenance themselves")
	require.NotNil(t, after.ScheduledErasureAt)
	assert.Equal(t, before.ScheduledErasureAt.UnixMilli(), after.ScheduledErasureAt.UnixMilli())
	assert.Empty(t, report.Marked)
	assert.Empty(t, report.ErasureDatesCleared)
}

func TestBackfillProvenance_DryRun_WritesNothing(t *testing.T) {
	db := newDB()
	created := time.Now().Add(-48 * time.Hour).Truncate(time.Second)
	orgID := seedOrg(t, db, models.OrgTypeTeam)
	seedRow(t, db, orgID, "imported", created.Add(30*time.Second), created.Add(30*time.Second))
	seedRow(t, db, orgID, "attached", created.Add(30*time.Second), created.Add(30*time.Second))
	offboardedRow(t, db, orgID, "attached")

	report, err := services.BackfillAccountProvenance(db, accountList{
		importedAccount("imported", created, nil),
		importedAccount("attached", created, map[string]string{"username": "ada"}),
	}, time.Now(), true)
	require.NoError(t, err)

	assert.Equal(t, []services.MemberRef{ref(orgID, "imported")}, report.Marked, "the dry run reports what it would mark")
	assert.Equal(t, []services.MemberRef{ref(orgID, "attached")}, report.ErasureDatesCleared)
	assert.False(t, load(t, db, orgID, "imported").AccountCreatedByOrganization, "a dry run writes nothing")
	assert.NotNil(t, load(t, db, orgID, "attached").ScheduledErasureAt, "a dry run writes nothing")
}

func TestBackfillProvenance_SecondRun_MarksAndClearsNothingNew(t *testing.T) {
	db := newDB()
	created := time.Now().Add(-48 * time.Hour).Truncate(time.Second)
	orgID := seedOrg(t, db, models.OrgTypeTeam)
	seedRow(t, db, orgID, "imported", created.Add(30*time.Second), created.Add(30*time.Second))
	seedRow(t, db, orgID, "attached", created.Add(30*time.Second), created.Add(30*time.Second))
	offboardedRow(t, db, orgID, "attached")
	accounts := accountList{importedAccount("imported", created, nil), importedAccount("attached", created, map[string]string{"username": "ada"})}

	dry, err := services.BackfillAccountProvenance(db, accounts, time.Now(), true)
	require.NoError(t, err)
	first, err := services.BackfillAccountProvenance(db, accounts, time.Now(), false)
	require.NoError(t, err)
	second, err := services.BackfillAccountProvenance(db, accounts, time.Now(), false)
	require.NoError(t, err)

	assert.Equal(t, dry.Marked, first.Marked, "the dry run announces exactly what the real run marks")
	assert.Equal(t, dry.ErasureDatesCleared, first.ErasureDatesCleared)
	assert.Empty(t, second.Marked, "a second run marks nothing")
	assert.Empty(t, second.ErasureDatesCleared, "a second run clears nothing")
	assert.Equal(t, first.BlockedAccounts, second.BlockedAccounts, "blocked accounts stay listed until an operator lifts them")
	assert.True(t, load(t, db, orgID, "imported").AccountCreatedByOrganization)
}

// Pins the inclusive bounds the code takes: the name's timestamp within 5
// minutes of the account's creation, the join within 10 minutes of it.
func TestBackfillProvenance_TimeWindows_InclusiveBounds(t *testing.T) {
	created := time.Now().Add(-48 * time.Hour).Truncate(time.Second)
	for _, c := range []struct {
		name               string
		stampShift, joined time.Duration
		marked             bool
	}{
		{"stamp +5m, joined at creation", 5 * time.Minute, 0, true},
		{"stamp -5m", -5 * time.Minute, 0, true},
		{"stamp +5m1s", 5*time.Minute + time.Second, 0, false},
		{"joined +10m", 0, 10 * time.Minute, true},
		{"joined -10m", 0, -10 * time.Minute, true},
		{"joined +10m1s", 0, 10*time.Minute + time.Second, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			db := newDB()
			orgID := seedOrg(t, db, models.OrgTypeTeam)
			seedRow(t, db, orgID, "u1", created.Add(c.joined), created.Add(c.joined))
			account := importedAccount("u1", created, nil)
			account.Name = fmt.Sprintf("ada-lovelace-%d", created.Add(c.stampShift).Unix())

			_, err := services.BackfillAccountProvenance(db, accountList{account}, time.Now(), false)
			require.NoError(t, err)

			assert.Equal(t, c.marked, load(t, db, orgID, "u1").AccountCreatedByOrganization)
		})
	}
}

func TestBackfillProvenance_AccountMissingFromIdentityProvider_LeftUnmarked(t *testing.T) {
	db := newDB()
	orgID := seedOrg(t, db, models.OrgTypeTeam)
	seedRow(t, db, orgID, "gone", time.Now().Add(-time.Hour), time.Now().Add(-time.Hour))

	other := importedAccount("other", time.Now().Add(-time.Hour), nil)
	report, err := services.BackfillAccountProvenance(db, accountList{other}, time.Now(), false)
	require.NoError(t, err)

	assert.False(t, load(t, db, orgID, "gone").AccountCreatedByOrganization)
	assert.Contains(t, report.LeftUnmarked, ref(orgID, "gone"))
}

type failingAccounts struct{}

func (failingAccounts) GetUsers() ([]*casdoorsdk.User, error) {
	return nil, errors.New("identity provider down")
}

func TestBackfillProvenance_ListingFails_WritesNothing(t *testing.T) {
	db := newDB()
	orgID := seedOrg(t, db, models.OrgTypeTeam)
	seedRow(t, db, orgID, "attached", time.Now().Add(-time.Hour), time.Now().Add(-time.Hour))
	offboardedRow(t, db, orgID, "attached")

	_, err := services.BackfillAccountProvenance(db, failingAccounts{}, time.Now(), false)

	require.Error(t, err)
	assert.NotNil(t, load(t, db, orgID, "attached").ScheduledErasureAt, "without the account list no erasure date is cleared")
}

// An empty answer from the identity provider would leave every row unmarked
// and clear every legacy erasure date: the real run refuses, the dry run reports.
func TestBackfillProvenance_NoAccountListedWithCandidateRows_RealRunRefusesDryRunReports(t *testing.T) {
	db := newDB()
	orgID := seedOrg(t, db, models.OrgTypeTeam)
	seedRow(t, db, orgID, "u1", time.Now().Add(-time.Hour), time.Now().Add(-time.Hour))
	offboardedRow(t, db, orgID, "u1")

	dry, err := services.BackfillAccountProvenance(db, accountList{}, time.Now(), true)
	require.NoError(t, err)
	assert.Contains(t, dry.ErasureDatesCleared, ref(orgID, "u1"), "the dry run still reports")

	_, err = services.BackfillAccountProvenance(db, accountList{}, time.Now(), false)

	require.Error(t, err, "zero accounts against candidate rows is a failed listing, not an answer")
	assert.NotNil(t, load(t, db, orgID, "u1").ScheduledErasureAt, "nothing is written")
}

// A created member offboarded between the deploy and the backfill was treated
// as attached: no erasure date. The backfill marks the row and lists it for
// the operator, without changing it.
func TestBackfillProvenance_MarkedRowOffboardedWithoutErasureDate_Listed(t *testing.T) {
	db := newDB()
	created := time.Now().Add(-48 * time.Hour).Truncate(time.Second)
	orgID := seedOrg(t, db, models.OrgTypeTeam)
	seedRow(t, db, orgID, "u1", created.Add(30*time.Second), created.Add(30*time.Second))
	require.NoError(t, db.Model(&models.OrganizationMember{}).Where("organization_id = ? AND user_id = ?", orgID, "u1").
		Updates(map[string]any{"is_active": false, "left_at": time.Now()}).Error)

	report, err := services.BackfillAccountProvenance(db, accountList{importedAccount("u1", created, nil)}, time.Now(), false)
	require.NoError(t, err)

	assert.Equal(t, []services.MemberRef{ref(orgID, "u1")}, report.Marked)
	assert.Equal(t, []services.MemberRef{ref(orgID, "u1")}, report.OffboardedWithoutErasureDate)
	m := load(t, db, orgID, "u1")
	assert.True(t, m.AccountCreatedByOrganization)
	assert.False(t, m.IsActive)
	assert.Nil(t, m.ScheduledErasureAt, "listed, not modified")
}
