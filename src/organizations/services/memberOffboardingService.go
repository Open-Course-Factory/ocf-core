package services

import (
	"errors"
	"fmt"
	"time"

	"soli/formations/src/auth/casdoor"
	authModels "soli/formations/src/auth/models"
	organizationModels "soli/formations/src/organizations/models"
	paymentModels "soli/formations/src/payment/models"
	paymentServices "soli/formations/src/payment/services"
	"soli/formations/src/utils"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

var (
	ErrNoMembersSelected   = errors.New("no members selected")
	ErrMemberNotFound      = errors.New("organization member not found")
	ErrCannotOffboardOwner = errors.New("the organization owner cannot be offboarded")
	ErrOutranksCaller      = errors.New("a member who outranks the caller is out of reach")
	ErrMemberNotOffboarded = errors.New("member is not offboarded")
)

// AccountForbidder is the slice of the identity provider offboarding needs:
// block and unblock sign-in. auth/services' CasdoorUserClient satisfies it; the
// organization module cannot import that package (userService imports this one).
type AccountForbidder interface {
	SetForbidden(userID string, forbidden bool) error
}

// DepartedMemberEraser is the erasure entry point for a member an organization
// offboarded. auth/services' UserDeletionService satisfies it.
type DepartedMemberEraser interface {
	EraseDepartedMember(orgID uuid.UUID, userID string, byAdministrator bool) error
}

// LicenceRevoker is the existing bulk-licence revoke path, narrowed to what
// offboarding calls. paymentServices.BulkLicenseService satisfies it.
type LicenceRevoker interface {
	RevokeLicense(licenseID uuid.UUID, requestingUserID string) error
}

// MemberOffboardingService owns the lifecycle of a departing member: offboard,
// reinstate, erase — and enrolment, because re-enrolling an offboarded member
// IS reinstating, so both must share one helper.
type MemberOffboardingService interface {
	// Offboard deactivates the memberships, stamps left_at, terminates their
	// running terminals and releases their assigned seats. For an account this
	// organization created it also schedules the erasure from the organization's
	// retention, and forbids the account unless the person pays for it or owns
	// an organization or group elsewhere (being active in another organization
	// does not prevent it). A platform administrator also forbids an account
	// this organization did not create when it belongs to this organization
	// alone. A target who outranks actorID refuses the whole call.
	Offboard(orgID uuid.UUID, userIDs []string, actorID string) error
	// Reinstate reverses Offboard: the row is active again with both stamps
	// cleared, and offboarding blocks are lifted as LiftOffboardingBlock says.
	// Refuses a member who is not offboarded or who outranks actorID.
	Reinstate(orgID uuid.UUID, userID string, actorID string) error
	// EraseNow erases an offboarded member ahead of its scheduled date. Only a
	// platform administrator actorID erases an account the organization did not create.
	EraseNow(orgID uuid.UUID, userID string, actorID string) error
	// Enrol makes userID an active member: it reinstates an existing inactive
	// row, leaves an active one alone, and creates the row otherwise, recording
	// accountCreated (the organization created the account). An existing row
	// keeps its provenance.
	Enrol(orgID uuid.UUID, userID string, accountCreated bool) error
}

type memberOffboardingService struct {
	db       *gorm.DB
	accounts AccountForbidder
	licences LicenceRevoker
	eraser   DepartedMemberEraser
	alone    OrganizationAloneChecker
}

func NewMemberOffboardingService(db *gorm.DB, accounts AccountForbidder, licences LicenceRevoker, eraser DepartedMemberEraser, alone OrganizationAloneChecker) MemberOffboardingService {
	return &memberOffboardingService{db: db, accounts: accounts, licences: licences, eraser: eraser, alone: alone}
}

func (s *memberOffboardingService) Offboard(orgID uuid.UUID, userIDs []string, actorID string) error {
	if len(userIDs) == 0 {
		return ErrNoMembersSelected
	}
	var org organizationModels.Organization
	if err := s.db.First(&org, "id = ?", orgID).Error; err != nil {
		return fmt.Errorf("organization not found: %w", err)
	}

	checkRank, actorIsAdmin, err := newRankOnlyCheck(s.db, &org, actorID)
	if err != nil {
		return err
	}
	forbid := make(map[string]bool, len(userIDs))
	schedule := make(map[string]bool, len(userIDs))
	setsBlock := make(map[string]bool, len(userIDs))
	skip := make(map[string]bool, len(userIDs))
	for _, userID := range userIDs {
		target, err := checkRank(userID)
		if err != nil {
			if errors.Is(err, errOutranksActor) {
				return fmt.Errorf("%w: %s", ErrOutranksCaller, userID)
			}
			return err
		}
		if target.IsOffboarded() {
			// Re-offboarding is a no-op: the row keeps its dates and its block origin.
			skip[userID] = true
			continue
		}
		// The nightly erasure job erases only accounts the organization created.
		schedule[userID] = target.AccountCreatedByOrganization
		// An unknown answer does not forbid: blocking a whole account is the harmful direction.
		forbid[userID] = (target.AccountCreatedByOrganization || actorIsAdmin) &&
			controlsAccount(s.alone, org.ID, target)
		if forbid[userID] {
			// A block someone else set is left alone, so reinstating never lifts it.
			blocked, err := authModels.IsAccountBlocked(s.db, userID)
			if err != nil {
				return fmt.Errorf("failed to read account block of %s: %w", userID, err)
			}
			setsBlock[userID] = !blocked
		}
	}

	now := time.Now()
	retention := now.AddDate(0, 0, org.EffectiveRetentionDays())
	var forbidden []string
	err = s.db.Transaction(func(tx *gorm.DB) error {
		for _, userID := range userIDs {
			if skip[userID] {
				continue
			}
			var erasureAt *time.Time
			if schedule[userID] {
				erasureAt = &retention
			}
			if err := s.offboardOne(tx, &org, userID, now, erasureAt, forbid[userID], setsBlock[userID]); err != nil {
				return err
			}
			if !setsBlock[userID] {
				continue
			}
			if err := s.accounts.SetForbidden(userID, true); err != nil {
				return fmt.Errorf("failed to forbid account %s: %w", userID, err)
			}
			forbidden = append(forbidden, userID)
		}
		return nil
	})
	if err != nil {
		// The DB rolled back; the accounts already forbidden must not stay
		// locked out of a membership that is still active.
		s.unforbid(forbidden)
		return err
	}

	for _, userID := range userIDs {
		if !skip[userID] {
			s.releaseAssignedLicences(userID, actorID)
		}
	}
	return nil
}

// offboardOne stamps a single membership and terminates the member's running
// terminals on the caller's transaction: all of them when the account is
// forbidden, only this organization's otherwise. setsBlock stamps that this
// offboarding is the one blocking the account; a nil erasureAt schedules no erasure.
func (s *memberOffboardingService) offboardOne(tx *gorm.DB, org *organizationModels.Organization, userID string, now time.Time, erasureAt *time.Time, forbidden, setsBlock bool) error {
	member, err := findMembership(tx, org.ID, userID)
	if err != nil {
		return err
	}
	if member.IsOwner() || org.IsOwner(userID) {
		return ErrCannotOffboardOwner
	}
	if err := tx.Model(member).Updates(map[string]any{
		"is_active":                        false,
		"left_at":                          now,
		"scheduled_erasure_at":             erasureAt,
		"account_forbidden_by_offboarding": setsBlock,
	}).Error; err != nil {
		return fmt.Errorf("failed to offboard member %s: %w", userID, err)
	}
	scope := &org.ID
	if forbidden {
		scope = nil
	}
	if err := paymentServices.TerminateUserTerminals(tx, userID, scope); err != nil {
		return fmt.Errorf("failed to terminate terminals of %s: %w", userID, err)
	}
	return nil
}

func (s *memberOffboardingService) unforbid(userIDs []string) {
	for _, userID := range userIDs {
		if err := s.accounts.SetForbidden(userID, false); err != nil {
			utils.Error("Offboarding rolled back but account %s is still forbidden: %v", userID, err)
		}
	}
}

// releaseAssignedLicences returns every seat assigned to the member to its
// batch through the existing revoke path. It runs after the membership commit
// because RevokeLicense opens its own transaction. A seat the actor may not
// manage (bought by a purchaser outside this organization) is logged and kept:
// it is not this organization's to release.
func (s *memberOffboardingService) releaseAssignedLicences(userID, actorID string) {
	var licenceIDs []uuid.UUID
	if err := s.db.Model(&paymentModels.UserSubscription{}).
		Where("user_id = ? AND subscription_batch_id IS NOT NULL", userID).
		Scopes(paymentModels.ScopeEntitling).
		Pluck("id", &licenceIDs).Error; err != nil {
		utils.Error("Failed to list assigned licences of %s: %v", userID, err)
		return
	}
	for _, licenceID := range licenceIDs {
		if err := s.licences.RevokeLicense(licenceID, actorID); err != nil {
			utils.Warn("Licence %s of offboarded member %s was not released: %v", licenceID, userID, err)
		}
	}
}

func (s *memberOffboardingService) Reinstate(orgID uuid.UUID, userID string, actorID string) error {
	var org organizationModels.Organization
	if err := s.db.First(&org, "id = ?", orgID).Error; err != nil {
		return fmt.Errorf("organization not found: %w", err)
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		checkRank, _, err := newRankOnlyCheck(tx, &org, actorID)
		if err != nil {
			return err
		}
		member, err := checkRank(userID)
		if errors.Is(err, errOutranksActor) {
			return fmt.Errorf("%w: %s", ErrOutranksCaller, userID)
		}
		if err != nil {
			return err
		}
		if !member.IsOffboarded() {
			return ErrMemberNotOffboarded
		}
		return s.reactivate(tx, member)
	})
}

// reactivate is the one place a membership comes back to life: the row is
// active with both offboarding stamps cleared, and offboarding blocks are
// lifted as LiftOffboardingBlock says. The Casdoor call runs inside the
// transaction so a failure leaves the member offboarded rather than
// active-but-locked-out.
func (s *memberOffboardingService) reactivate(tx *gorm.DB, member *organizationModels.OrganizationMember) error {
	if err := tx.Model(member).Updates(map[string]any{
		"is_active":            true,
		"left_at":              nil,
		"scheduled_erasure_at": nil,
	}).Error; err != nil {
		return fmt.Errorf("failed to reinstate member %s: %w", member.UserID, err)
	}
	return LiftOffboardingBlock(tx, s.accounts, member.OrganizationID, member.UserID)
}

// LiftOffboardingBlock is called whenever userID becomes an active member of
// orgID. It clears the block stamp of orgID's own row and of every row whose
// organization did not create the account (that block stood because the
// account belonged to it alone, which it no longer does); a creating
// organization's stamp elsewhere stays, since that organization keeps control.
// The rows stay offboarded. The account is unblocked only when a stamp was
// cleared and none remains; a block set otherwise (no stamp) stays.
func LiftOffboardingBlock(tx *gorm.DB, accounts AccountForbidder, orgID uuid.UUID, userID string) error {
	cleared := tx.Model(&organizationModels.OrganizationMember{}).
		Where("user_id = ? AND account_forbidden_by_offboarding = ? AND (account_created_by_organization = ? OR organization_id = ?)",
			userID, true, false, orgID).
		Update("account_forbidden_by_offboarding", false)
	if cleared.Error != nil {
		return fmt.Errorf("failed to clear the offboarding block of %s: %w", userID, cleared.Error)
	}
	if cleared.RowsAffected == 0 {
		return nil
	}
	var remaining int64
	if err := tx.Model(&organizationModels.OrganizationMember{}).
		Where("user_id = ? AND account_forbidden_by_offboarding = ?", userID, true).
		Count(&remaining).Error; err != nil {
		return fmt.Errorf("failed to read the offboarding blocks of %s: %w", userID, err)
	}
	if remaining > 0 {
		return nil
	}
	if err := accounts.SetForbidden(userID, false); err != nil {
		return fmt.Errorf("failed to unforbid account %s: %w", userID, err)
	}
	return nil
}

func (s *memberOffboardingService) EraseNow(orgID uuid.UUID, userID string, actorID string) error {
	member, err := findMembership(s.db, orgID, userID)
	if err != nil {
		return err
	}
	if !member.IsOffboarded() {
		return ErrMemberNotOffboarded
	}
	return s.eraser.EraseDepartedMember(orgID, userID, IsPlatformAdmin(actorID))
}

func (s *memberOffboardingService) Enrol(orgID uuid.UUID, userID string, accountCreated bool) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		member, err := findMembership(tx, orgID, userID)
		switch {
		case errors.Is(err, ErrMemberNotFound):
			return createMembership(tx, orgID, userID, accountCreated)
		case err != nil:
			return err
		case member.IsActive:
			return nil
		default:
			return s.reactivate(tx, member)
		}
	})
}

func findMembership(tx *gorm.DB, orgID uuid.UUID, userID string) (*organizationModels.OrganizationMember, error) {
	var member organizationModels.OrganizationMember
	err := tx.Where("organization_id = ? AND user_id = ?", orgID, userID).First(&member).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("%w: %s in %s", ErrMemberNotFound, userID, orgID)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to load membership of %s: %w", userID, err)
	}
	return &member, nil
}

// createMembership inserts a plain member row and grants the organization
// policy, as the CSV import always did. A removed membership's created mark is kept.
func createMembership(tx *gorm.DB, orgID uuid.UUID, userID string, accountCreated bool) error {
	if !accountCreated {
		carried, err := RemovedMembershipWasCreatedHere(tx, orgID, userID)
		if err != nil {
			return err
		}
		accountCreated = carried
	}
	member := organizationModels.OrganizationMember{
		OrganizationID: orgID,
		UserID:         userID,
		Role:           organizationModels.OrgRoleMember,
		JoinedAt:       time.Now(),
		IsActive:       true,

		AccountCreatedByOrganization: accountCreated,
	}
	if err := tx.Create(&member).Error; err != nil {
		return fmt.Errorf("failed to add user to organization: %w", err)
	}
	opts := utils.DefaultPermissionOptions()
	opts.WarnOnError = true
	utils.AddPolicy(casdoor.Enforcer, userID, fmt.Sprintf("/api/v1/organizations/%s", orgID), "GET|POST|PATCH", opts)
	return nil
}

// RemovedMembershipWasCreatedHere reports whether a removed (soft-deleted)
// membership of userID in orgID was marked created by orgID: re-adding the
// member in the same organization keeps the mark.
func RemovedMembershipWasCreatedHere(db *gorm.DB, orgID uuid.UUID, userID string) (bool, error) {
	var marked int64
	if err := db.Unscoped().Model(&organizationModels.OrganizationMember{}).
		Where("organization_id = ? AND user_id = ? AND deleted_at IS NOT NULL AND account_created_by_organization = ?", orgID, userID, true).
		Count(&marked).Error; err != nil {
		return false, fmt.Errorf("failed to read the removed membership of %s: %w", userID, err)
	}
	return marked > 0, nil
}
