// src/payment/services/organizationSubscriptionService.go
package services

import (
	"fmt"
	organizationModels "soli/formations/src/organizations/models"
	"soli/formations/src/payment/models"
	"soli/formations/src/payment/repositories"
	"soli/formations/src/utils"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type OrganizationSubscriptionService interface {
	// Subscription management
	GetOrganizationSubscription(orgID uuid.UUID) (*models.OrganizationSubscription, error)
	GetOrganizationSubscriptionByID(id uuid.UUID) (*models.OrganizationSubscription, error)
	// CreateOrganizationSubscription assigns a plan to an organization.
	//
	// It takes no seat quantity: an organization's plan is not sold per seat.
	// Seats are a trainer's licence batch, counted on SubscriptionBatch, and a
	// school or OF is on a bespoke plan whose terms are the plan itself (#456).
	CreateOrganizationSubscription(orgID uuid.UUID, planID uuid.UUID, ownerUserID string, isAdminAssigned bool) (*models.OrganizationSubscription, error)
	UpdateOrganizationSubscription(orgID uuid.UUID, planID uuid.UUID) (*models.OrganizationSubscription, error)
	CancelOrganizationSubscription(orgID uuid.UUID, cancelAtPeriodEnd bool) error

	// Admin bulk access
	GetAllActiveOrganizationSubscriptions() ([]models.OrganizationSubscription, error)

	// Feature access (for members)
	GetOrganizationFeatures(orgID uuid.UUID) (*models.SubscriptionPlan, error)
	GetOrganizationUsageLimits(orgID uuid.UUID) (*OrganizationLimits, error)

	// User-level feature aggregation
	GetUserEffectiveFeatures(userID string) (*UserEffectiveFeatures, error)
}

// Business types for organization limits
type OrganizationLimits struct {
	OrganizationID   uuid.UUID
	CurrentTerminals int
	CurrentCourses   int
}

type UserEffectiveFeatures struct {
	HighestPlan   *models.SubscriptionPlan
	AllFeatures   []string
	Organizations []OrganizationFeatureInfo
}

type OrganizationFeatureInfo struct {
	OrganizationID   uuid.UUID
	OrganizationName string
	SubscriptionPlan models.SubscriptionPlan
	IsOwner          bool
	IsManager        bool
}

type organizationSubscriptionService struct {
	repository  repositories.OrganizationSubscriptionRepository
	paymentRepo repositories.PaymentRepository
	db          *gorm.DB
}

func NewOrganizationSubscriptionService(db *gorm.DB) OrganizationSubscriptionService {
	return &organizationSubscriptionService{
		repository:  repositories.NewOrganizationSubscriptionRepository(db),
		paymentRepo: repositories.NewPaymentRepository(db),
		db:          db,
	}
}

// GetOrganizationSubscription retrieves the active subscription for an organization
func (oss *organizationSubscriptionService) GetOrganizationSubscription(orgID uuid.UUID) (*models.OrganizationSubscription, error) {
	return oss.repository.GetActiveOrganizationSubscription(orgID)
}

// GetOrganizationSubscriptionByID retrieves a subscription by its ID
func (oss *organizationSubscriptionService) GetOrganizationSubscriptionByID(id uuid.UUID) (*models.OrganizationSubscription, error) {
	return oss.repository.GetOrganizationSubscription(id)
}

// GetAllActiveOrganizationSubscriptions retrieves all active or trialing organization subscriptions
func (oss *organizationSubscriptionService) GetAllActiveOrganizationSubscriptions() ([]models.OrganizationSubscription, error) {
	return oss.repository.GetAllActiveOrganizationSubscriptions()
}

// CreateOrganizationSubscription creates a new organization subscription
// For free plans (PriceAmount == 0), creates an active subscription
// For paid plans, creates an incomplete subscription that will be activated by Stripe webhook
// When isAdminAssigned is true, paid plans are activated immediately (no Stripe flow)
func (oss *organizationSubscriptionService) CreateOrganizationSubscription(orgID uuid.UUID, planID uuid.UUID, ownerUserID string, isAdminAssigned bool) (*models.OrganizationSubscription, error) {
	// Verify the organization exists
	var org organizationModels.Organization
	if err := oss.db.Where("id = ?", orgID).First(&org).Error; err != nil {
		return nil, fmt.Errorf("organization not found: %w", err)
	}

	// A personal organization cannot hold a subscription. Plan resolution
	// short-circuits personal orgs to the user's own subscription, so a row
	// created here would never be read — the assignment appeared to succeed,
	// changed nothing, and reported no error, which is the worst of the three
	// (#458). Refusing makes the ignored state unrepresentable instead.
	if org.IsPersonalOrg() {
		return nil, fmt.Errorf(
			"organization %q is personal and cannot hold a subscription: personal "+
				"workspaces use their owner's own plan", org.Name)
	}

	// Get the plan to check if it's free
	var plan models.SubscriptionPlan
	if err := oss.db.Where("id = ?", planID).First(&plan).Error; err != nil {
		return nil, fmt.Errorf("invalid plan ID: %w", err)
	}

	now := time.Now()

	subscription := &models.OrganizationSubscription{
		OrganizationID:     orgID,
		SubscriptionPlanID: planID,
	}

	// FREE PLAN or ADMIN-ASSIGNED: Activate immediately without Stripe
	if plan.PriceAmount == 0 || isAdminAssigned {
		subscription.Status = "active"
		subscription.CurrentPeriodStart = now
		subscription.CurrentPeriodEnd = now.AddDate(1, 0, 0)

		if isAdminAssigned {
			utils.Info("Creating admin-assigned organization subscription for org %s (plan: %s)", orgID, plan.Name)
		} else {
			utils.Info("Creating free organization subscription for org %s (plan: %s)", orgID, plan.Name)
		}
	} else {
		// A paid org plan cannot be bought here. Nothing creates a Stripe checkout
		// carrying organization_id, so the "activated by Stripe webhook" this used
		// to wait for could never arrive — it recorded an `incomplete` row, told
		// the caller it had succeeded, and charged nobody (#450).
		//
		// Team orgs are not self-service purchasable by design: structures are
		// "contact us" and get their plan admin-assigned, trainers buy personally
		// and their orgs inherit. Refusing keeps that unrepresentable state
		// unrepresentable.
		return nil, fmt.Errorf(
			"plan %q is paid and cannot be subscribed to by an organization directly: "+
				"organization plans are assigned by an administrator, or inherited from the "+
				"owner's personal plan", plan.Name)
	}

	// Use the atomic variant so any existing active/trialing subscription
	// for this org is deactivated in the same transaction. Enforces the
	// "one active subscription per organization" invariant.
	err := oss.repository.CreateOrganizationSubscriptionAtomic(subscription)
	if err != nil {
		return nil, err
	}

	oss.syncOrgPlanPointer(orgID)

	return oss.GetOrganizationSubscriptionByID(subscription.ID)
}

// UpdateOrganizationSubscription updates an organization's subscription plan
func (oss *organizationSubscriptionService) UpdateOrganizationSubscription(orgID uuid.UUID, planID uuid.UUID) (*models.OrganizationSubscription, error) {
	// Get the organization's active subscription
	subscription, err := oss.repository.GetActiveOrganizationSubscription(orgID)
	if err != nil {
		return nil, fmt.Errorf("no active subscription found for organization: %w", err)
	}

	// Get the new plan to verify it exists
	var newPlan models.SubscriptionPlan
	if err := oss.db.Where("id = ?", planID).First(&newPlan).Error; err != nil {
		return nil, fmt.Errorf("invalid plan ID: %w", err)
	}

	// Update subscription plan ID
	subscription.SubscriptionPlanID = planID
	subscription.SubscriptionPlan = newPlan

	err = oss.repository.UpdateOrganizationSubscription(subscription)
	if err != nil {
		return nil, fmt.Errorf("failed to update subscription: %w", err)
	}

	oss.syncOrgPlanPointer(orgID)

	return oss.repository.GetOrganizationSubscription(subscription.ID)
}

// CancelOrganizationSubscription cancels an organization's subscription
func (oss *organizationSubscriptionService) CancelOrganizationSubscription(orgID uuid.UUID, cancelAtPeriodEnd bool) error {
	subscription, err := oss.repository.GetActiveOrganizationSubscription(orgID)
	if err != nil {
		return fmt.Errorf("no active subscription found for organization: %w", err)
	}

	if cancelAtPeriodEnd {
		subscription.CancelAtPeriodEnd = true
		utils.Info("Organization subscription %s will be cancelled at period end", subscription.ID)
	} else {
		subscription.Status = "cancelled"
		now := time.Now()
		subscription.CancelledAt = &now
		utils.Info("Organization subscription %s cancelled immediately", subscription.ID)
	}

	if err := oss.repository.UpdateOrganizationSubscription(subscription); err != nil {
		return err
	}

	oss.syncOrgPlanPointer(orgID)

	// Terminate active terminals for org members on immediate cancellation.
	// For cancel-at-period-end, terminals are terminated when Stripe fires
	// the subscription.deleted webhook at the end of the billing period.
	if !cancelAtPeriodEnd {
		TerminateOrganizationMemberTerminals(oss.db, orgID)
	}

	return nil
}

// syncOrgPlanPointer recomputes organizations.subscription_plan_id from the org's
// active subscription.
//
// That column is a denormalised copy of "which plan does this org have", and the
// active subscription is what actually decides it. Keeping the copy meant two
// expressions of one rule, and they drifted: it was written on a purchase that
// never activated (marc-corp claimed Formateur while running Trial) and never
// cleared on cancellation, so an org kept advertising a plan it no longer had —
// which ocf-front renders as a "has subscription" badge.
//
// So the column gets exactly one writer, and that writer derives rather than
// assumes. Call it after any change to the org's subscription state; never set
// the column directly at a call site (#449).
func (oss *organizationSubscriptionService) syncOrgPlanPointer(orgID uuid.UUID) {
	var planID *uuid.UUID
	if sub, err := oss.repository.GetActiveOrganizationSubscription(orgID); err == nil && sub != nil {
		planID = &sub.SubscriptionPlanID
	}

	err := oss.db.Model(&organizationModels.Organization{}).
		Where("id = ?", orgID).
		Update("subscription_plan_id", planID).Error
	if err != nil {
		utils.Warn("Failed to sync organization %s subscription_plan_id: %v", orgID, err)
	}
}

// GetOrganizationFeatures returns the subscription plan features for an organization
func (oss *organizationSubscriptionService) GetOrganizationFeatures(orgID uuid.UUID) (*models.SubscriptionPlan, error) {
	subscription, err := oss.repository.GetActiveOrganizationSubscription(orgID)
	if err != nil {
		return nil, fmt.Errorf("no active subscription found for organization: %w", err)
	}

	return &subscription.SubscriptionPlan, nil
}

// GetOrganizationUsageLimits returns the current usage and limits for an organization.
//
// Thin wrapper kept for backward compatibility with existing callers and
// test mocks. The actual quota-counting and limit-extraction logic lives
// in QuotaService.GetOrgQuota — see src/payment/services/quotaService.go.
func (oss *organizationSubscriptionService) GetOrganizationUsageLimits(orgID uuid.UUID) (*OrganizationLimits, error) {
	eps := NewEffectivePlanService(oss.db)
	quotaSvc := NewQuotaService(oss.db, eps)
	return quotaSvc.GetOrgQuota(orgID)
}

// GetUserEffectiveFeatures returns the highest-tier features from all user's organizations
func (oss *organizationSubscriptionService) GetUserEffectiveFeatures(userID string) (*UserEffectiveFeatures, error) {
	// Get all organization subscriptions for the user
	subscriptions, err := oss.repository.GetUserOrganizationSubscriptions(userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user organization subscriptions: %w", err)
	}

	if len(subscriptions) == 0 {
		return nil, fmt.Errorf("user has no organization subscriptions")
	}

	// Aggregate features
	features := &UserEffectiveFeatures{
		AllFeatures:   make([]string, 0),
		Organizations: make([]OrganizationFeatureInfo, 0),
	}

	featureSet := make(map[string]bool)
	var highestPriority int = -1

	// Batch-fetch all organizations and member records in 2 queries (not 2N)
	orgIDs := make([]uuid.UUID, 0, len(subscriptions))
	for _, sub := range subscriptions {
		orgIDs = append(orgIDs, sub.OrganizationID)
	}

	var orgs []organizationModels.Organization
	if err := oss.db.Where("id IN ?", orgIDs).Find(&orgs).Error; err != nil {
		return nil, fmt.Errorf("failed to batch-fetch organizations: %w", err)
	}
	orgMap := make(map[uuid.UUID]organizationModels.Organization, len(orgs))
	for _, org := range orgs {
		orgMap[org.ID] = org
	}

	var members []organizationModels.OrganizationMember
	if err := oss.db.Where("organization_id IN ? AND user_id = ?", orgIDs, userID).Find(&members).Error; err != nil {
		return nil, fmt.Errorf("failed to batch-fetch member records: %w", err)
	}
	memberMap := make(map[uuid.UUID]organizationModels.OrganizationMember, len(members))
	for _, m := range members {
		memberMap[m.OrganizationID] = m
	}

	for _, sub := range subscriptions {
		org, orgFound := orgMap[sub.OrganizationID]
		if !orgFound {
			utils.Warn("Organization %s not found in batch", sub.OrganizationID)
			continue
		}

		member, memberFound := memberMap[sub.OrganizationID]
		if !memberFound {
			utils.Warn("Member info not found for org %s", sub.OrganizationID)
			continue
		}

		plan := sub.SubscriptionPlan

		// Track highest priority plan
		if plan.Priority > highestPriority {
			highestPriority = plan.Priority
			features.HighestPlan = &plan
		}

		// Aggregate entitlements (union across plans — capabilities compose).
		// Project each plan's TYPED fields via derivePlanEntitlements (SSOT)
		// rather than unioning the legacy free-form plan.Features strings.
		for _, feature := range derivePlanEntitlements(&plan) {
			featureSet[feature] = true
		}

		// Add organization info
		features.Organizations = append(features.Organizations, OrganizationFeatureInfo{
			OrganizationID:   org.ID,
			OrganizationName: org.DisplayName,
			SubscriptionPlan: plan,
			IsOwner:          member.IsOwner(),
			IsManager:        member.IsManager(),
		})
	}

	// Convert feature set to slice
	for feature := range featureSet {
		features.AllFeatures = append(features.AllFeatures, feature)
	}

	return features, nil
}

