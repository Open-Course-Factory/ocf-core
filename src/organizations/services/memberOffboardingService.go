package services

import (
	"errors"
	"fmt"
	"time"

	"soli/formations/src/auth/casdoor"
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
	ErrMemberNotOffboarded = errors.New("member is not offboarded; erasure is the end of offboarding")
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
	EraseDepartedMember(orgID uuid.UUID, userID string) error
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
	// Offboard deactivates the memberships, stamps left_at / scheduled_erasure_at
	// from the organization's retention, forbids the accounts, terminates their
	// running terminals and releases their assigned seats.
	Offboard(orgID uuid.UUID, userIDs []string, actorID string) error
	// Reinstate reverses Offboard: the row is active again with both stamps
	// cleared and the account may sign in.
	Reinstate(orgID uuid.UUID, userID string) error
	// EraseNow erases an offboarded member ahead of its scheduled date.
	EraseNow(orgID uuid.UUID, userID string) error
	// Enrol makes userID an active member: it reinstates an existing inactive
	// row, leaves an active one alone, and creates the row otherwise.
	Enrol(orgID uuid.UUID, userID string) error
}

type memberOffboardingService struct {
	db       *gorm.DB
	accounts AccountForbidder
	licences LicenceRevoker
	eraser   DepartedMemberEraser
}

func NewMemberOffboardingService(db *gorm.DB, accounts AccountForbidder, licences LicenceRevoker, eraser DepartedMemberEraser) MemberOffboardingService {
	return &memberOffboardingService{db: db, accounts: accounts, licences: licences, eraser: eraser}
}

func (s *memberOffboardingService) Offboard(orgID uuid.UUID, userIDs []string, actorID string) error {
	if len(userIDs) == 0 {
		return ErrNoMembersSelected
	}
	var org organizationModels.Organization
	if err := s.db.First(&org, "id = ?", orgID).Error; err != nil {
		return fmt.Errorf("organization not found: %w", err)
	}

	now := time.Now()
	erasureAt := now.AddDate(0, 0, org.EffectiveRetentionDays())
	var forbidden []string
	err := s.db.Transaction(func(tx *gorm.DB) error {
		for _, userID := range userIDs {
			if err := s.offboardOne(tx, &org, userID, now, erasureAt); err != nil {
				return err
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
		s.releaseAssignedLicences(userID, actorID)
	}
	return nil
}

// offboardOne stamps a single membership and terminates the member's running
// terminals on the caller's transaction.
func (s *memberOffboardingService) offboardOne(tx *gorm.DB, org *organizationModels.Organization, userID string, now, erasureAt time.Time) error {
	member, err := findMembership(tx, org.ID, userID)
	if err != nil {
		return err
	}
	if member.IsOwner() || org.IsOwner(userID) {
		return ErrCannotOffboardOwner
	}
	if err := tx.Model(member).Updates(map[string]any{
		"is_active":            false,
		"left_at":              now,
		"scheduled_erasure_at": erasureAt,
	}).Error; err != nil {
		return fmt.Errorf("failed to offboard member %s: %w", userID, err)
	}
	if err := paymentServices.TerminateUserTerminals(tx, userID, nil); err != nil {
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

func (s *memberOffboardingService) Reinstate(orgID uuid.UUID, userID string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		member, err := findMembership(tx, orgID, userID)
		if err != nil {
			return err
		}
		return s.reactivate(tx, member)
	})
}

// reactivate is the one place a membership comes back to life: the row is
// active with both offboarding stamps cleared, and the account may sign in.
// The Casdoor call runs inside the transaction so a failure leaves the member
// offboarded rather than active-but-locked-out.
func (s *memberOffboardingService) reactivate(tx *gorm.DB, member *organizationModels.OrganizationMember) error {
	if err := tx.Model(member).Updates(map[string]any{
		"is_active":            true,
		"left_at":              nil,
		"scheduled_erasure_at": nil,
	}).Error; err != nil {
		return fmt.Errorf("failed to reinstate member %s: %w", member.UserID, err)
	}
	if err := s.accounts.SetForbidden(member.UserID, false); err != nil {
		return fmt.Errorf("failed to unforbid account %s: %w", member.UserID, err)
	}
	return nil
}

func (s *memberOffboardingService) EraseNow(orgID uuid.UUID, userID string) error {
	member, err := findMembership(s.db, orgID, userID)
	if err != nil {
		return err
	}
	if !member.IsOffboarded() {
		return ErrMemberNotOffboarded
	}
	return s.eraser.EraseDepartedMember(orgID, userID)
}

func (s *memberOffboardingService) Enrol(orgID uuid.UUID, userID string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		member, err := findMembership(tx, orgID, userID)
		switch {
		case errors.Is(err, ErrMemberNotFound):
			return createMembership(tx, orgID, userID)
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
// policy, as the CSV import always did.
func createMembership(tx *gorm.DB, orgID uuid.UUID, userID string) error {
	member := organizationModels.OrganizationMember{
		OrganizationID: orgID,
		UserID:         userID,
		Role:           organizationModels.OrgRoleMember,
		JoinedAt:       time.Now(),
		IsActive:       true,
	}
	if err := tx.Create(&member).Error; err != nil {
		return fmt.Errorf("failed to add user to organization: %w", err)
	}
	opts := utils.DefaultPermissionOptions()
	opts.WarnOnError = true
	utils.AddPolicy(casdoor.Enforcer, userID, fmt.Sprintf("/api/v1/organizations/%s", orgID), "GET|POST|PATCH", opts)
	return nil
}
