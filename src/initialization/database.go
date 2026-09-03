package initialization

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/google/uuid"
	"gorm.io/gorm"

	auditModels "soli/formations/src/audit/models"
	"soli/formations/src/auth/casdoor"
	authModels "soli/formations/src/auth/models"
	scenarioServices "soli/formations/src/scenarios/services"
	configModels "soli/formations/src/configuration/models"
	courseModels "soli/formations/src/courses/models"
	emailModels "soli/formations/src/email/models"
	groupModels "soli/formations/src/groups/models"
	organizationModels "soli/formations/src/organizations/models"
	paymentModels "soli/formations/src/payment/models"
	paymentServices "soli/formations/src/payment/services"
	scenarioModels "soli/formations/src/scenarios/models"
	terminalModels "soli/formations/src/terminalTrainer/models"
	testtools "soli/formations/tests/testTools"
)

// SweepAutoAssignedOrgTrials cancels the free Trial subscriptions that team
// organizations were automatically given before #448.
//
// Those Trials were never bought or chosen: they were assigned on creation and
// re-healed at every startup. Because resolveForOrg prefers any org subscription
// over the personal fallback, each one outranked its owner's paid personal plan
// — a trainer who bought Formateur lost it inside the very org they created to
// use it. Removing the auto-assignment fixes new orgs; existing ones need this.
//
// Scoped to the free plan on team organizations. A deliberately assigned Trial
// would be swept too, and that is intended rather than collateral: Trial has
// group_management_enabled = false, so holding one actively disables classroom
// features, and inheriting is never worse than holding a free plan.
//
// Cancelled rather than deleted — the record is real history, and 'cancelled'
// releases the partial unique index on one active subscription per org, which a
// soft delete alone would not.
//
// Idempotent: once swept, the WHERE matches nothing.
func SweepAutoAssignedOrgTrials(db *gorm.DB) {
	freePlan, err := paymentServices.FindFreePlan(db)
	if err != nil {
		log.Printf("[ORG-TRIAL-SWEEP] no free plan found, nothing to sweep: %v", err)
		return
	}

	teamOrgIDs := db.Model(&organizationModels.Organization{}).
		Select("id").
		Where("organization_type = ?", "team")

	now := time.Now()
	res := db.Model(&paymentModels.OrganizationSubscription{}).
		Scopes(paymentModels.ScopeEntitling).
		Where("subscription_plan_id = ?", freePlan.ID).
		Where("organization_id IN (?)", teamOrgIDs).
		Updates(map[string]any{"status": "cancelled", "cancelled_at": now})
	if res.Error != nil {
		log.Printf("[ORG-TRIAL-SWEEP] failed to cancel auto-assigned org Trials: %v", res.Error)
		return
	}

	// The denormalised pointer has to follow, or the org keeps advertising a plan
	// whose subscription was just cancelled (#449).
	ptr := db.Model(&organizationModels.Organization{}).
		Where("organization_type = ?", "team").
		Where("subscription_plan_id = ?", freePlan.ID).
		Update("subscription_plan_id", nil)
	if ptr.Error != nil {
		log.Printf("[ORG-TRIAL-SWEEP] failed to clear organization plan pointers: %v", ptr.Error)
	}

	if res.RowsAffected > 0 || ptr.RowsAffected > 0 {
		log.Printf("[ORG-TRIAL-SWEEP] cancelled %d auto-assigned org Trial subscription(s) "+
			"and cleared %d plan pointer(s); those orgs now inherit their members' plans",
			res.RowsAffected, ptr.RowsAffected)
	}
}

// AutoMigrateAll performs database migrations for all entities
func AutoMigrateAll(db *gorm.DB) {
	// Course entities
	db.AutoMigrate(&courseModels.Page{})
	db.AutoMigrate(&courseModels.Section{})
	db.AutoMigrate(&courseModels.Chapter{})
	db.AutoMigrate(&courseModels.Course{})
	db.AutoMigrate(&courseModels.Session{})

	// Course many-to-many relationships
	db.AutoMigrate(&courseModels.CourseChapters{})
	errJTChapterC := db.SetupJoinTable(&courseModels.Course{}, "Chapters", &courseModels.CourseChapters{})
	if errJTChapterC != nil {
		log.Default().Println(errJTChapterC)
	}
	errJTCoursesC := db.SetupJoinTable(&courseModels.Chapter{}, "Courses", &courseModels.CourseChapters{})
	if errJTCoursesC != nil {
		log.Default().Println(errJTCoursesC)
	}

	db.AutoMigrate(&courseModels.ChapterSections{})
	errJTSectionC := db.SetupJoinTable(&courseModels.Chapter{}, "Sections", &courseModels.ChapterSections{})
	if errJTSectionC != nil {
		log.Default().Println(errJTSectionC)
	}
	errJTChaptersS := db.SetupJoinTable(&courseModels.Section{}, "Chapters", &courseModels.ChapterSections{})
	if errJTChaptersS != nil {
		log.Default().Println(errJTChaptersS)
	}

	db.AutoMigrate(&courseModels.SectionPages{})
	errJTPage := db.SetupJoinTable(&courseModels.Section{}, "Pages", &courseModels.SectionPages{})
	if errJTPage != nil {
		log.Default().Println(errJTPage)
	}
	errJTSectionP := db.SetupJoinTable(&courseModels.Page{}, "Sections", &courseModels.SectionPages{})
	if errJTSectionP != nil {
		log.Default().Println(errJTSectionP)
	}

	// Other course entities
	db.AutoMigrate(&courseModels.Schedule{})
	db.AutoMigrate(&courseModels.Theme{})
	db.AutoMigrate(&courseModels.Generation{})

	// Auth entities
	db.AutoMigrate(&authModels.SshKey{})
	db.AutoMigrate(&authModels.UserSettings{})
	db.AutoMigrate(&authModels.TokenBlacklist{})
	db.AutoMigrate(&authModels.PasswordResetToken{})
	db.AutoMigrate(&authModels.EmailVerificationToken{})
	db.AutoMigrate(&authModels.ImpersonationSession{})

	// Email entities
	db.AutoMigrate(&emailModels.EmailTemplate{})

	// Terminal entities
	db.AutoMigrate(&terminalModels.Terminal{})
	db.AutoMigrate(&terminalModels.UserTerminalKey{})
	db.AutoMigrate(&terminalModels.ExposedPort{})
	// MR !239 (SSOT consolidation): the legacy `status` column on `terminals`
	// was a parallel field that drifted from `state` and caused zombie-resume
	// and dashboard banner bugs. The model field is gone; this drops the
	// orphan column. Idempotent — HasColumn returns false once dropped.
	dropOrphanTerminalColumns(db)

	// Group entities
	db.AutoMigrate(&groupModels.ClassGroup{})
	db.AutoMigrate(&groupModels.GroupMember{})

	// Organization entities (Phase 1)
	db.AutoMigrate(&organizationModels.Organization{})
	db.AutoMigrate(&organizationModels.OrganizationMember{})

	// Scenario entities
	db.AutoMigrate(&scenarioModels.ProjectFile{})
	db.AutoMigrate(&scenarioModels.Scenario{})
	// After the model, never before: the estimate is carried into a column that
	// AutoMigrate has just created, and the prose one is dropped only once it
	// has somewhere to land.
	scenarioModels.MigrateEstimatedTimeToMinutes(db)
	db.AutoMigrate(&scenarioModels.ScenarioStep{})
	db.AutoMigrate(&scenarioModels.ScenarioStepHint{})
	db.AutoMigrate(&scenarioModels.ScenarioSession{})
	db.AutoMigrate(&scenarioModels.ScenarioStepProgress{})
	db.AutoMigrate(&scenarioModels.ScenarioFlag{})
	db.AutoMigrate(&scenarioModels.ScenarioAssignment{})
	db.AutoMigrate(&scenarioModels.ScenarioInstanceType{})
	db.AutoMigrate(&scenarioModels.ScenarioStepQuestion{})
	db.AutoMigrate(&scenarioModels.ScenarioTranslation{})
	db.AutoMigrate(&scenarioModels.ScenarioStepTranslation{})
	// Widen the lexicon key index before the model rebuilds it: an earlier
	// shape could not express one object sitting in two places.
	scenarioModels.MigrateLexiconKeyIndex(db)
	db.AutoMigrate(&scenarioModels.ScenarioLexiconEntry{})
	db.AutoMigrate(&scenarioModels.ScenarioLexiconName{})

	// Scenario indexes
	scenarioModels.MigrateUniqueActiveSessionIndex(db)

	// Payment entities
	db.AutoMigrate(&paymentModels.SubscriptionPlan{})
	// MR !239 (SSOT consolidation): persistent_sessions_enabled and
	// max_persistent_sessions were duplicates of data_persistence_enabled /
	// data_persistence_gb. The model fields were removed; AutoMigrate leaves
	// orphan columns behind, so we explicitly drop them here. Idempotent —
	// HasColumn returns false once the column is gone.
	dropOrphanSubscriptionPlanColumns(db)
	db.AutoMigrate(&paymentModels.SubscriptionBatch{})
	db.AutoMigrate(&paymentModels.UserSubscription{})         // DEPRECATED in Phase 2 (kept for backward compat)
	db.AutoMigrate(&paymentModels.OrganizationSubscription{}) // NEW: Phase 2 - Organization subscriptions
	// #374: TrialEnd was removed (no paid trials); drop the orphan trial_end
	// columns AutoMigrate leaves behind on both subscription tables.
	dropOrphanSubscriptionTrialEndColumns(db)
	// NOTE: The partial unique index on organization_subscriptions is
	// created AFTER BackfillSingleActiveOrgSubscription so the cleanup of
	// legacy duplicate rows has a chance to run first. See line ~149.
	db.AutoMigrate(&paymentModels.OrganizationRolePlan{}) // NEW: role-based plan entitlements within an organization
	db.AutoMigrate(&paymentModels.Invoice{})
	db.AutoMigrate(&paymentModels.PaymentMethod{})
	db.AutoMigrate(&paymentModels.UsageMetrics{})
	// One-shot cleanup: the legacy `concurrent_terminals` usage metric is
	// dead infrastructure. The CPU/RAM budget engine
	// (SubscriptionPlan.MaxCPU / MaxMemoryMB enforced by
	// QuotaService.CheckBudget) is now the sole authoritative quota gate
	// for terminals. The seed for this row was removed; any rows still
	// present in usage_metrics are leftovers from previous deployments
	// and must be scrubbed so they cannot resurface through the generic
	// /usage-metrics entity endpoint. Idempotent — once the rows are
	// gone, subsequent runs are no-ops.
	DeleteOrphanConcurrentTerminalsRows(db)
	// One-shot rescale: SubscriptionPlan.MaxCPU and Terminal.SizeCPU
	// changed unit from integer vCPU to integer millicores (mCPU) so the
	// budget engine could price XS containers correctly (XS runs at
	// cpu_allowance=50% on tt-backend → 500 mCPU, not 1 vCPU). Idempotent
	// via the >0 AND <100 guard: any legacy value (1..99) is a vCPU
	// reading and gets ×1000; anything ≥100 is already in mCPU (the new
	// catalog/plan values start at 500) and is left alone.
	RescaleVCPUToMillicores(db)
	db.AutoMigrate(&paymentModels.BillingAddress{})
	db.AutoMigrate(&paymentModels.WebhookEvent{}) // ✅ SECURITY: Track processed webhooks in database
	db.AutoMigrate(&paymentModels.StripeSync{})   // Persistent Stripe sync queue (issue #326)

	// Configuration entities
	db.AutoMigrate(&configModels.Feature{})

	// Audit logging entities (compliance & security)
	db.AutoMigrate(&auditModels.AuditLog{})

	// Harmonize group roles: admin → manager, assistant → member
	migrateGroupRoles(db)

	// Migrate existing hint_content to progressive hint records
	migrateHintContentToHints(db)

	// Migrate inline scripts/markdown to ProjectFile records
	migrateInlineContentToProjectFiles(db)

	// Give the public plans the names the offer uses. Runs BEFORE the two routines
	// below: EnsureFreePlanExists keys on the current name and would otherwise
	// create a second free plan beside the one being renamed, and MarkDefaultFreePlan
	// looks the row up by that name.
	RenameLegacyPlans(db)

	// Elect the plan new signups receive. Runs before EnsureFreePlanExists so an
	// existing free plan is marked rather than duplicated.
	MarkDefaultFreePlan(db)

	// Ensure the free default plan always exists (regardless of environment)
	EnsureFreePlanExists(db)

	// #439: retire the 'trialing' status from existing rows. Must run BEFORE both
	// the backfill and the index migration below, which now speak only of 'active'.
	MigrateTrialingStatusToActive(db)

	// #441: seed the new BulkPurchasable flag from the rule it replaces, so plans
	// that were sellable in bulk yesterday still are today.
	BackfillBulkPurchasableFromGroupManagement(db)

	// Enforce "one active subscription per organization" on existing data.
	// Runs BEFORE ensureOrganizationsHaveTrialPlan so the latter sees a
	// clean state (zero or one active sub per org). Idempotent and safe
	// to run on every startup.
	BackfillSingleActiveOrgSubscription(db)

	// Create the DB-level partial unique index AFTER the backfill cleanup
	// so legacy duplicate-active rows cannot block the CREATE INDEX. From
	// this point on, the index serializes cross-pod concurrent inserts —
	// closes #312.
	paymentModels.MigrateUniqueActiveOrgSubscriptionIndex(db)

	// Heal users that are missing their Trial subscription (all environments).
	//
	// Organizations are deliberately NOT healed: a team org holds no plan of its
	// own and inherits the acting member's entitlement. Granting one a free Trial
	// made that Trial outrank its owner's paid plan, because resolveForOrg prefers
	// any org subscription over the personal fallback — so every trainer who
	// created an org lost the plan they had just bought, on every restart (#448).
	ensureUsersHaveTrialPlan(db)

	// One-shot cleanup for #448. Removing the auto-assignment stopped new team
	// orgs acquiring a shadowing Trial, but every org created before that still
	// holds one — and a Trial outranks its owner's paid plan, so those trainers
	// stay locked out of the classroom features they paid for until this runs.
	SweepAutoAssignedOrgTrials(db)

	// Report subscriptions whose plan no longer exists. Resolution now refuses
	// them (#481), which protects the platform but shows the user a missing
	// entitlement and the operator nothing — this names the rows to repair.
	if report := paymentServices.ReportDanglingPlanReferences(db); report.Any() {
		log.Printf("[PLAN-REFERENCES] dangling plan references found — these subscriptions "+
			"cannot resolve and grant nothing until repaired: %d user subscription(s), "+
			"%d organization subscription(s), %d organization role plan(s)",
			report.UserSubscriptions, report.OrganizationSubscriptions, report.OrganizationRolePlans)
	}

	// Drop the orphan subscription_plans columns whose Go fields were removed.
	// This runs LAST and subsumes the standalone group-management backfill: it
	// performs a FINAL backfill pass reading the raw `features` column, THEN drops
	// it (and the other orphan columns), so no later startup step reads a dropped
	// column. Idempotent.
	DropOrphanPlanColumns(db)

	// Same treatment for scenarios.gsh_enabled, whose Go field is gone.
	DropOrphanScenarioColumns(db)
}

// InitDevelopmentData sets up development data in debug mode
func InitDevelopmentData(db *gorm.DB) {
	env := os.Getenv("ENVIRONMENT")
	if env == "development" || env == "test" {
		db = db.Debug()
		SetupDefaultSubscriptionPlans(db)
		setupExternalUsersData()
		syncCasdoorRolesToCasbin()
		ensureUsersHaveTrialPlan(db)
		// NOTE: the legacy features[] "group_management" self-heal is no longer
		// needed here — DropOrphanPlanColumns (run in AutoMigrateAll) performs the
		// final backfill and drops the `features` column, and the dev seed sets the
		// typed GroupManagementEnabled bool directly.
	}
}

// setupExternalUsersData initializes test users if none exist
func setupExternalUsersData() {
	users, _ := casdoorsdk.GetUsers()
	var notDeletedUser []*casdoorsdk.User
	for _, user := range users {
		if !user.IsDeleted {
			notDeletedUser = append(notDeletedUser, user)
		}
	}
	if len(notDeletedUser) == 0 {
		testtools.SetupBasicRoles()
		testtools.SetupUsers()
		testtools.SetupGroups()
		testtools.SetupRoles()
	}
}

// syncCasdoorRolesToCasbin ensures all Casdoor role assignments are reflected
// as Casbin grouping policies. This fixes cases where the casbin_rule table
// was reset but users still exist in Casdoor, leaving them with no roles.
func syncCasdoorRolesToCasbin() {
	orgName := os.Getenv("CASDOOR_ORGANIZATION_NAME")

	roles, err := casdoorsdk.GetRoles()
	if err != nil {
		log.Printf("[ROLE-SYNC] Could not get Casdoor roles: %v", err)
		return
	}

	users, err := casdoorsdk.GetUsers()
	if err != nil {
		log.Printf("[ROLE-SYNC] Could not get Casdoor users: %v", err)
		return
	}

	if err := casdoor.Enforcer.LoadPolicy(); err != nil {
		log.Printf("[ROLE-SYNC] Could not load Casbin policy: %v", err)
		return
	}

	// Build mapping: "orgName/username" -> userID
	userIDMap := make(map[string]string)
	for _, user := range users {
		if user != nil && !user.IsDeleted {
			userIDMap[orgName+"/"+user.Name] = user.Id
		}
	}

	// Ensure every active user has at least the "member" role
	for _, user := range users {
		if user == nil || user.IsDeleted {
			continue
		}
		existingRoles, _ := casdoor.Enforcer.GetRolesForUser(user.Id)
		hasMember := false
		for _, r := range existingRoles {
			if r == "member" {
				hasMember = true
				break
			}
		}
		if !hasMember {
			if _, err := casdoor.Enforcer.AddGroupingPolicy(user.Id, "member"); err != nil {
				log.Printf("[ROLE-SYNC] Failed to add 'member' role to user %s: %v", user.Id, err)
			} else {
				log.Printf("[ROLE-SYNC] Added missing 'member' role to user %s (%s)", user.Name, user.Id)
			}
		}
	}

	// Sync each Casdoor role to Casbin grouping policies
	for _, role := range roles {
		if role == nil {
			continue
		}
		for _, userRef := range role.Users {
			userID, ok := userIDMap[userRef]
			if !ok {
				continue
			}
			existingRoles, _ := casdoor.Enforcer.GetRolesForUser(userID)
			hasRole := false
			for _, r := range existingRoles {
				if r == role.Name {
					hasRole = true
					break
				}
			}
			if !hasRole {
				if _, err := casdoor.Enforcer.AddGroupingPolicy(userID, role.Name); err != nil {
					log.Printf("[ROLE-SYNC] Failed to add '%s' role to user %s: %v", role.Name, userID, err)
				} else {
					log.Printf("[ROLE-SYNC] Added missing '%s' role to user %s", role.Name, userID)
				}
			}
		}
	}

	log.Println("[ROLE-SYNC] Casdoor-to-Casbin role sync complete")
}

// RenameLegacyPlans gives the three public plans the names the offer actually
// uses. It renames the ROW: these are the same commercial products, carrying
// live subscriptions and Stripe ids, so creating new plans beside them would
// strand every existing customer on a plan nobody sells.
//
// Skips a rename whose target name is already taken by a different row — that
// means the catalogue has already been named, and two plans sharing a name is
// worse than one keeping an old one.
//
// Idempotent: once renamed, the WHERE matches nothing.
func RenameLegacyPlans(db *gorm.DB) {
	renames := []struct{ from, to string }{
		{paymentServices.LegacyFreePlanName, paymentServices.FreePlanName},
		{"Member Pro", "Solo"},
		{"Trainer Plan", "Formateur"},
	}

	for _, r := range renames {
		var taken int64
		db.Model(&paymentModels.SubscriptionPlan{}).Where("name = ?", r.to).Count(&taken)
		if taken > 0 {
			continue
		}
		res := db.Model(&paymentModels.SubscriptionPlan{}).
			Where("name = ?", r.from).
			Update("name", r.to)
		if res.Error != nil {
			log.Printf("[PLAN-RENAME] failed to rename %q to %q: %v", r.from, r.to, res.Error)
			continue
		}
		if res.RowsAffected > 0 {
			log.Printf("[PLAN-RENAME] renamed %q to %q (%d row)", r.from, r.to, res.RowsAffected)
		}
	}
}

// MarkDefaultFreePlan elects the single plan new signups are given.
//
// The election used to be "the plan named Trial", which is why naming the offer
// needed this: FindFreePlan now reads the typed marker, and this is what puts it
// on the right row in a database that predates the column.
//
// Runs after RenameLegacyPlans, so it matches the current name; falls back to
// the legacy one for a database migrated out of order.
func MarkDefaultFreePlan(db *gorm.DB) {
	var marked int64
	db.Model(&paymentModels.SubscriptionPlan{}).Where("is_default_free = ?", true).Count(&marked)
	if marked == 1 {
		return
	}
	if marked > 1 {
		log.Printf("[FREE-PLAN] %d plans claim to be the default free plan; leaving them alone "+
			"— pick one by hand, this routine will not choose for you", marked)
		return
	}

	var plan paymentModels.SubscriptionPlan
	err := db.Where("price_amount = 0 AND name IN ?",
		[]string{paymentServices.FreePlanName, paymentServices.LegacyFreePlanName}).
		Order("created_at ASC").First(&plan).Error
	if err != nil {
		// Nothing to elect yet: a fresh database gets its marker from the seed.
		return
	}
	if err := db.Model(&plan).Update("is_default_free", true).Error; err != nil {
		log.Printf("[FREE-PLAN] failed to mark %q as the default free plan: %v", plan.Name, err)
		return
	}
	log.Printf("[FREE-PLAN] marked %q as the default free plan", plan.Name)
}

// EnsureFreePlanExists ensures the free default plan always exists in the database
// and keeps its key fields in sync with the code-defined values.
func EnsureFreePlanExists(db *gorm.DB) {
	// Clean up duplicate free plans (keep oldest, delete newer duplicates)
	var duplicates []paymentModels.SubscriptionPlan
	db.Where("name = ? AND price_amount = 0", paymentServices.FreePlanName).
		Order("created_at ASC").Find(&duplicates)
	if len(duplicates) > 1 {
		for _, dup := range duplicates[1:] {
			log.Printf("[FREE-PLAN] Removing duplicate free plan %s", dup.ID)
			// Reassign any org subscriptions pointing to the duplicate
			db.Model(&paymentModels.OrganizationSubscription{}).
				Where("subscription_plan_id = ?", dup.ID).
				Update("subscription_plan_id", duplicates[0].ID)
			db.Model(&paymentModels.UserSubscription{}).
				Where("subscription_plan_id = ?", dup.ID).
				Update("subscription_plan_id", duplicates[0].ID)
			db.Delete(&dup)
		}
	}

	// The free tier is described in the same catalogue as the paid plans — it is
	// an offer, not an implementation detail — so its values are read from there
	// rather than repeated here. Repeating them is how the seed and the free-plan
	// sync came to disagree about the description.
	template, err := paymentServices.FreePlanTemplate()
	if err != nil {
		log.Printf("Warning: cannot read the free plan from the catalogue: %v\n", err)
		return
	}

	// FirstOrCreate is atomic, preventing a TOCTOU race when two pods start
	// simultaneously. Attrs are applied only on creation.
	var existing paymentModels.SubscriptionPlan
	result := db.Where("name = ? AND price_amount = 0", paymentServices.FreePlanName).
		Attrs(template).
		FirstOrCreate(&existing)
	if result.Error != nil {
		log.Printf("Warning: Failed to find or create the %s plan: %v\n", paymentServices.FreePlanName, result.Error)
		return
	}
	if result.RowsAffected > 0 {
		log.Printf("Created missing %s plan", paymentServices.FreePlanName)
	}

	// Re-assert the governed fields on every startup, so a plan edited by hand
	// drifts back to the offer. Sourced from the same template as the creation
	// above — the two used to be separate literals, and only one of them was
	// updated when the free tier was renamed.
	db.Model(&existing).Updates(map[string]interface{}{
		"description":                    template.Description,
		"max_session_duration_minutes":   template.MaxSessionDurationMinutes,
		"max_cpu":                        template.MaxCPU,
		"max_memory_mb":                  template.MaxMemoryMB,
		"network_access_enabled":         template.NetworkAccessEnabled,
		"data_persistence_enabled":       template.DataPersistenceEnabled,
		"data_persistence_gb":            template.DataPersistenceGB,
		"is_active":                      template.IsActive,
		"is_default_free":                true,
		"command_history_retention_days": template.CommandHistoryRetentionDays,
	})
}

// SetupDefaultSubscriptionPlans initializes default subscription plans
func SetupDefaultSubscriptionPlans(db *gorm.DB) {
	// Always ensure Trial plan exists first
	EnsureFreePlanExists(db)

	// Vérifier si les other plans existent déjà
	var count int64
	db.Model(&paymentModels.SubscriptionPlan{}).Where("price_amount > 0").Count(&count)
	if count > 0 {
		return // Paid plans déjà créés
	}

	// The offer itself lives in src/payment/services/catalogue.json, which the
	// production bootstrap script reads too. Defining it here as well is how a
	// price ends up correct in one environment and stale in the other.
	catalogue, err := paymentServices.PaidCatalogue()
	if err != nil {
		log.Printf("Warning: cannot read the plan catalogue: %v\n", err)
		return
	}

	plans := make([]*paymentModels.SubscriptionPlan, 0, len(catalogue))
	for i := range catalogue {
		plans = append(plans, &catalogue[i])
	}

	for _, plan := range plans {
		// Read the intent BEFORE Create: GORM omits zero-value bools on insert for
		// a column declared `default:true`, then writes the applied default back
		// into the struct — so after Create, a plan meant to be hidden reports
		// IsCatalog=true and its own intent is gone. Same defect as #447 on the
		// entity API.
		wantsHiding := !plan.IsCatalog

		if err := db.Create(plan).Error; err != nil {
			log.Printf("Warning: Failed to create subscription plan %s: %v\n", plan.Name, err)
			continue
		}
		// Writing it back explicitly is the documented workaround; Select("*") on
		// Create does not work despite older comments saying so.
		if wantsHiding {
			if err := db.Model(plan).Update("is_catalog", false).Error; err != nil {
				log.Printf("Warning: Failed to hide plan %s from the catalogue: %v\n", plan.Name, err)
			}
		}
		log.Printf("Created subscription plan: %s\n", plan.Name)
	}
}

// BackfillGroupManagementEntitlement sets GroupManagementEnabled=true on every
// plan whose legacy features[] JSON still contains "group_management", so the
// typed entitlement matches the historical string. The model no longer has a
// Features field, so this reads the RAW `features` column (an orphaned JSON-text
// column left behind by the removal) and decodes it in Go, exact-matching the
// element to avoid substring false positives. Idempotent — re-runs only touch
// rows not already migrated. Mirrors the other ensureXXX/Backfill helpers.
func BackfillGroupManagementEntitlement(db *gorm.DB) {
	type planRow struct {
		ID                     uuid.UUID
		Features               string // raw JSON array text from the orphan column
		GroupManagementEnabled bool
	}
	var rows []planRow
	if err := db.Table("subscription_plans").
		Select("id, features, group_management_enabled").
		Scan(&rows).Error; err != nil {
		log.Printf("Warning: BackfillGroupManagementEntitlement failed to load plans: %v\n", err)
		return
	}
	for _, r := range rows {
		if r.GroupManagementEnabled {
			continue // already migrated — idempotent
		}
		if r.Features == "" {
			continue
		}
		var features []string
		if err := json.Unmarshal([]byte(r.Features), &features); err != nil {
			continue // unparseable legacy value — nothing to migrate
		}
		hasLegacyString := false
		for _, f := range features {
			if f == "group_management" {
				hasLegacyString = true
				break
			}
		}
		if !hasLegacyString {
			continue
		}
		if err := db.Table("subscription_plans").
			Where("id = ?", r.ID).
			Update("group_management_enabled", true).Error; err != nil {
			log.Printf("Warning: BackfillGroupManagementEntitlement failed for plan %s: %v\n", r.ID, err)
		}
	}
}

// BackfillBulkPurchasableFromGroupManagement marks the plans that were already
// bulk-purchasable under the old gate (#441).
//
// Until the gate was split, a plan was sellable in bulk precisely when it
// granted group management. Introducing the explicit BulkPurchasable flag with a
// default of false would therefore have silently stopped every existing bulk
// purchase, so the old rule is replayed once to seed the new flag.
//
// Idempotent: it only touches rows where the flag is still false, so a plan an
// admin has deliberately unmarked is not re-marked on the next restart.
func BackfillBulkPurchasableFromGroupManagement(db *gorm.DB) {
	// Seed ONCE. "Where bulk_purchasable = false" is not a once-guard — it is the
	// condition that re-marks, so an administrator who deliberately unmarks a
	// legacy plan would find it sellable again after the next restart. Treat any
	// already-marked plan as proof the seeding has happened.
	//
	// Limitation, accepted: if every plan is unmarked the seeding runs again. That
	// is indistinguishable from a fresh database without a migrations ledger, and
	// unmarking the entire catalogue is not a state worth carrying machinery for.
	var alreadyMarked int64
	if err := db.Model(&paymentModels.SubscriptionPlan{}).
		Where("bulk_purchasable = ?", true).Count(&alreadyMarked).Error; err != nil {
		log.Printf("[MIGRATION] BackfillBulkPurchasableFromGroupManagement probe failed: %v", err)
		return
	}
	if alreadyMarked > 0 {
		return
	}

	// The old gate was IsActive AND IsCatalog AND GroupManagementEnabled — all
	// three. Replaying only the group-management half would MARK PLANS THAT WERE
	// NEVER SELLABLE: a hidden bespoke plan carrying group management was rejected
	// before (IsCatalog=false) and would become bulk-purchasable by any eligible
	// trainer. Catalog membership is therefore part of the replay, even though it
	// is no longer part of the gate.
	res := db.Model(&paymentModels.SubscriptionPlan{}).
		Where("group_management_enabled = ? AND is_catalog = ? AND bulk_purchasable = ?",
			true, true, false).
		Update("bulk_purchasable", true)
	if res.Error != nil {
		log.Printf("[MIGRATION] BackfillBulkPurchasableFromGroupManagement failed: %v", res.Error)
		return
	}
	if res.RowsAffected > 0 {
		log.Printf("[MIGRATION] marked %d plan(s) bulk-purchasable from the legacy group-management rule",
			res.RowsAffected)
	}
}

// MigrateTrialingStatusToActive converts subscriptions still sitting in the
// removed 'trialing' status over to 'active' (#439).
//
// OCF never sold paid trials, so in practice this touches zero rows. It exists
// because the one outcome the removal must not produce is a row that silently
// stops entitling: once 'trialing' leaves the entitling predicate, any row left
// in it grants nothing while still looking live in the database.
//
// Converting is safe against the organization uniqueness invariant: the legacy
// partial index permitted at most one active-OR-trialing subscription per org,
// so no org can be holding both and no conversion can collide.
//
// Must run BEFORE MigrateUniqueActiveOrgSubscriptionIndex, which narrows that
// index to 'active' alone.
func MigrateTrialingStatusToActive(db *gorm.DB) {
	for _, table := range []string{"user_subscriptions", "organization_subscriptions"} {
		res := db.Exec(fmt.Sprintf("UPDATE %s SET status = 'active' WHERE status = 'trialing'", table))
		if res.Error != nil {
			log.Printf("[TRIALING-MIGRATION] Failed to convert trialing rows in %s: %v", table, res.Error)
			continue
		}
		if res.RowsAffected > 0 {
			log.Printf("[TRIALING-MIGRATION] Converted %d trialing subscription(s) to active in %s",
				res.RowsAffected, table)
		}
	}
}

// ensureUsersHaveTrialPlan checks all Casdoor users and assigns the free Trial
// plan to any user who doesn't have an active subscription. This heals cases
// where the subscription assignment failed during user creation (e.g. due to
// initialization order issues or Casdoor resets).
func ensureUsersHaveTrialPlan(db *gorm.DB) {
	// Fail fast on a missing free plan so a broken catalogue logs once here rather
	// than once per user inside the loop.
	if _, err := paymentServices.FindFreePlan(db); err != nil {
		log.Printf("[TRIAL-SYNC] %v", err)
		return
	}

	users, err := casdoorsdk.GetUsers()
	if err != nil {
		log.Printf("[TRIAL-SYNC] Could not get Casdoor users: %v", err)
		return
	}

	fixed := 0
	for _, user := range users {
		if user == nil || user.IsDeleted {
			continue
		}

		// Shares the assignment path with signup and bulk import: same liveness
		// test, and usage metrics get initialised, which the inline copy skipped.
		assigned, err := paymentServices.EnsureFreeTrialAssigned(db, user.Id)
		if err != nil {
			log.Printf("[TRIAL-SYNC] Failed to assign Trial plan to user %s: %v", user.Id, err)
			continue
		}
		if assigned {
			fixed++
		}
	}

	if fixed > 0 {
		log.Printf("[TRIAL-SYNC] Assigned Trial plan to %d users who were missing subscriptions", fixed)
	}
}

// BackfillSingleActiveOrgSubscription enforces the "one active subscription
// per organization" invariant on the existing data. For each org with more
// than one active (or trialing) subscription, it keeps the most recently
// created row and marks the rest as cancelled.
//
// Idempotent: re-runs are no-ops because the loop only matches orgs that
// still have duplicates. Re-running on a clean database affects zero rows.
//
// Dialect-agnostic: walks the data in Go rather than relying on window
// functions, so it works identically against SQLite and PostgreSQL.
//
// Background: until the assignment paths were wrapped in a transaction that
// deactivates the previous active subscription, assigning a new plan to an
// org would leave the old subscription active. Different queries
// (`ORDER BY created_at DESC` vs joins) then resolved different rows for
// the same org, causing inconsistent plan resolution across endpoints.
func BackfillSingleActiveOrgSubscription(db *gorm.DB) {
	type row struct {
		ID             uuid.UUID
		OrganizationID uuid.UUID
		CreatedAt      time.Time
	}

	var rows []row
	// Deliberately NOT the entitling predicate. This backfill enforces the same
	// invariant as the partial UNIQUE INDEX on organization_subscriptions
	// (models/organizationSubscription.go), whose WHERE clause is literally
	// `status = 'active'`. The two must state the same set or the backfill
	// cancels rows the index never constrained. Change them together.
	if err := db.Table("organization_subscriptions").
		Select("id, organization_id, created_at").
		Where("status = ? AND deleted_at IS NULL", "active").
		Order("organization_id, created_at DESC").
		Scan(&rows).Error; err != nil {
		log.Printf("[ORG-SUB-BACKFILL] Failed to scan org subscriptions: %v", err)
		return
	}

	var toCancel []uuid.UUID
	var currentOrg uuid.UUID
	for _, r := range rows {
		if r.OrganizationID != currentOrg {
			currentOrg = r.OrganizationID
			continue // keep first (newest) row per org
		}
		toCancel = append(toCancel, r.ID)
	}

	if len(toCancel) == 0 {
		return
	}

	if err := db.Exec(`UPDATE organization_subscriptions
		SET status = 'cancelled',
			cancelled_at = COALESCE(cancelled_at, ?)
		WHERE id IN ?`, time.Now(), toCancel).Error; err != nil {
		log.Printf("[ORG-SUB-BACKFILL] Failed to cancel duplicate subscriptions: %v", err)
		return
	}
	log.Printf("[ORG-SUB-BACKFILL] Cancelled %d duplicate active organization subscriptions (kept newest per org)", len(toCancel))
}

// migrateGroupRoles harmonizes the old 4-level group role model (owner, admin,
// assistant, member) into the new 3-level model (owner, manager, member).
// Idempotent: only updates rows that still use the old role names.
// dropOrphanSubscriptionPlanColumns drops columns that used to back removed
// model fields. AutoMigrate adds columns but never drops them, so without
// this we leave behind dead columns that confuse SELECTs and Stripe sync
// (drift the SSOT). Idempotent: HasColumn returns false once the column is
// gone, so re-runs are safe.
//
// Dropped columns:
//   - persistent_sessions_enabled / max_persistent_sessions (MR !239 — collapsed
//     into DataPersistenceEnabled / DataPersistenceGB).
//   - quota_model / max_concurrent_terminals / allowed_machine_sizes
//     (dual-mode cleanup — the CPU/RAM budget is now the only quota model).
//   - trial_days (#374 — OCF has no paid trial period; the free Trial plan is
//     the only "trial").
func dropOrphanSubscriptionPlanColumns(db *gorm.DB) {
	orphans := []string{
		"persistent_sessions_enabled",
		"max_persistent_sessions",
		"quota_model",
		"max_concurrent_terminals",
		"allowed_machine_sizes",
		"trial_days",
	}
	migrator := db.Migrator()
	for _, col := range orphans {
		if !migrator.HasColumn(&paymentModels.SubscriptionPlan{}, col) {
			continue
		}
		if err := migrator.DropColumn(&paymentModels.SubscriptionPlan{}, col); err != nil {
			log.Printf("[MIGRATION] failed to drop orphan column subscription_plans.%s: %v", col, err)
			continue
		}
		log.Printf("[MIGRATION] dropped orphan column subscription_plans.%s", col)
	}
}

// orphanPlanColumns are the subscription_plans columns whose Go model fields were
// deleted across prior cleanup campaigns but which still exist physically in prod
// (AutoMigrate adds columns, never drops them). DropOrphanPlanColumns removes them.
var orphanPlanColumns = []string{
	"max_concurrent_users",
	"allowed_templates",
	"max_courses",
	"planned_features",
	"features",
	"addon_network_price_id",
	"addon_storage_price_id",
	"addon_terminal_price_id",
}

// orphanScenarioColumns are the scenarios columns whose Go model fields were
// removed. AutoMigrate adds columns and never drops them, so they survive in
// prod until something takes them out.
//
// gsh_enabled: the in-terminal gsh helper was dropped — ScenarioPanel already
// carries goal, hints, verify, flag and quiz — and the flag was read by nothing
// afterwards. The editor surface went in ocf-front !324; this removes the
// column, the DTOs and the import/export plumbing behind it.
var orphanScenarioColumns = []string{
	"gsh_enabled",
}

// DropOrphanScenarioColumns is the guarded one-time migration that removes the
// orphanScenarioColumns from scenarios.
//
// Same mechanism as DropOrphanPlanColumns, and for the same reason: GORM's
// Migrator().DropColumn is a silent no-op on gorm.io/driver/sqlite, so this
// issues the raw ALTER and guards it on HasColumn for idempotency.
//
// Nothing needs a final read here — unlike `features`, this column was already
// inert before it was deleted, so there is nothing to back up or migrate out of
// it.
func DropOrphanScenarioColumns(db *gorm.DB) {
	migrator := db.Migrator()

	for _, col := range orphanScenarioColumns {
		if !migrator.HasColumn(&scenarioModels.Scenario{}, col) {
			continue
		}
		if err := db.Exec("ALTER TABLE scenarios DROP COLUMN " + col).Error; err != nil {
			log.Printf("[MIGRATION] failed to drop orphan column scenarios.%s: %v", col, err)
			continue
		}
		log.Printf("[MIGRATION] dropped orphan column scenarios.%s", col)
	}
}

// DropOrphanPlanColumns is the guarded one-time migration that removes the
// orphanPlanColumns from subscription_plans.
//
// Ordering — final backfill THEN drop: the `features` column is the last reader
// of the legacy features[] "group_management" string. This migration runs a FINAL
// BackfillGroupManagementEntitlement pass (reading the raw `features` column while
// it still exists) BEFORE dropping the columns, and it SUBSUMES the standalone
// recurring backfill calls that used to run at startup — once `features` is gone,
// no later step may read it. Guarded on HasColumn(features) so the backfill is
// skipped once the column is dropped.
//
// Mechanism — raw ALTER, not migrator.DropColumn: GORM's Migrator().DropColumn is
// a silent no-op on gorm.io/driver/sqlite (returns nil, the column survives), so
// the test-env drops would never take effect. Postgres (prod) executes DropColumn
// correctly, but a raw `ALTER TABLE ... DROP COLUMN` is equivalent there and is the
// only mechanism that also works on SQLite. Each drop is guarded on HasColumn,
// standing in for the `DROP COLUMN IF EXISTS` that SQLite lacks.
//
// Idempotent: a second run finds the columns already gone (HasColumn false) and is
// a no-op; an already-migrated plan (GroupManagementEnabled=true) is left untouched
// by the backfill.
func DropOrphanPlanColumns(db *gorm.DB) {
	migrator := db.Migrator()

	// FINAL backfill pass — must read the raw `features` column before it is
	// dropped below, so no future startup step needs it.
	if migrator.HasColumn(&paymentModels.SubscriptionPlan{}, "features") {
		BackfillGroupManagementEntitlement(db)
	}

	for _, col := range orphanPlanColumns {
		if !migrator.HasColumn(&paymentModels.SubscriptionPlan{}, col) {
			continue
		}
		if err := db.Exec("ALTER TABLE subscription_plans DROP COLUMN " + col).Error; err != nil {
			log.Printf("[MIGRATION] failed to drop orphan column subscription_plans.%s: %v", col, err)
			continue
		}
		log.Printf("[MIGRATION] dropped orphan column subscription_plans.%s", col)
	}
}

// dropOrphanSubscriptionTrialEndColumns drops the trial_end column from
// user_subscriptions and organization_subscriptions. OCF has no paid trial
// period, so the TrialEnd model fields were removed (#374); AutoMigrate never
// drops columns, so we drop them explicitly. Mirrors
// dropOrphanSubscriptionPlanColumns and is idempotent.
func dropOrphanSubscriptionTrialEndColumns(db *gorm.DB) {
	migrator := db.Migrator()
	targets := []struct {
		name  string
		model interface{}
	}{
		{"user_subscriptions", &paymentModels.UserSubscription{}},
		{"organization_subscriptions", &paymentModels.OrganizationSubscription{}},
	}
	for _, t := range targets {
		if !migrator.HasColumn(t.model, "trial_end") {
			continue
		}
		if err := migrator.DropColumn(t.model, "trial_end"); err != nil {
			log.Printf("[MIGRATION] failed to drop orphan column %s.trial_end: %v", t.name, err)
			continue
		}
		log.Printf("[MIGRATION] dropped orphan column %s.trial_end", t.name)
	}
}

// dropOrphanTerminalColumns drops legacy columns from the `terminals` table
// that used to back removed model fields. Mirrors
// dropOrphanSubscriptionPlanColumns and is idempotent.
//
// MR !239 (SSOT consolidation): the `status` column was a duplicate of
// `state` and drifted in ways that broke Resume + dashboard banners. The
// model field is gone; this drops the column so reads/writes that still
// reference it (none should remain) fail loudly instead of silently
// reading stale data.
func dropOrphanTerminalColumns(db *gorm.DB) {
	orphans := []string{"status"}
	migrator := db.Migrator()
	for _, col := range orphans {
		if !migrator.HasColumn(&terminalModels.Terminal{}, col) {
			continue
		}
		if err := migrator.DropColumn(&terminalModels.Terminal{}, col); err != nil {
			log.Printf("[MIGRATION] failed to drop orphan column terminals.%s: %v", col, err)
			continue
		}
		log.Printf("[MIGRATION] dropped orphan column terminals.%s", col)
	}
}

// DeleteOrphanConcurrentTerminalsRows deletes any `usage_metrics` row whose
// `metric_type = 'concurrent_terminals'`.
//
// Why: the legacy concurrent_terminals usage metric was retired when the
// CPU/RAM budget engine (SubscriptionPlan.MaxCPU / MaxMemoryMB) became the
// sole authoritative quota gate for terminals. The seed for this row was
// removed from initializeUserMetrics; rows still present in existing
// databases are leftovers from previous deployments and must be scrubbed
// so they cannot resurface through the generic /usage-metrics entity
// endpoint or any other code path that reads the model directly.
//
// We do NOT drop the column or the table — other metric types
// (courses_created, ...) continue to use them.
//
// Idempotent: once the rows are gone, subsequent runs are no-ops. Safe to
// run on every boot.
//
// Exported so tests in `tests/payment/` can drive it directly.
func DeleteOrphanConcurrentTerminalsRows(db *gorm.DB) {
	result := db.Exec(`DELETE FROM usage_metrics WHERE metric_type = 'concurrent_terminals'`)
	if result.Error != nil {
		log.Printf("[MIGRATION] failed to delete orphan concurrent_terminals rows: %v", result.Error)
		return
	}
	if n := result.RowsAffected; n > 0 {
		log.Printf("[MIGRATION] deleted %d orphan concurrent_terminals rows from usage_metrics", n)
	}
}

// RescaleVCPUToMillicores rescales legacy integer-vCPU budget values to
// integer millicores (mCPU) on both subscription_plans.max_cpu and
// terminals.size_cpu. The unit switch was made so the budget engine can
// price sub-vCPU sizes correctly (XS = 500 mCPU because tt-backend runs
// XS at cpu_allowance=50%); under the old vCPU unit XS rounded to 1 and
// the budget over-counted by 2×.
//
// Idempotency guard: only rows with value >0 AND <100 are rescaled. The
// new catalog/plan values start at 500 mCPU (XS); any value ≥100 is
// already in mCPU and must NOT be multiplied again. The seed plans (Trial
// 500, Member Pro 6000, Trainer 40000) all clear the threshold from the
// first migration onwards.
//
// Exported so tests in `tests/payment/` can drive it directly.
func RescaleVCPUToMillicores(db *gorm.DB) {
	if r := db.Exec(`UPDATE subscription_plans SET max_cpu = max_cpu * 1000 WHERE max_cpu > 0 AND max_cpu < 100`); r.Error != nil {
		log.Printf("[MIGRATION] failed to rescale subscription_plans.max_cpu vCPU→mCPU: %v", r.Error)
	} else if n := r.RowsAffected; n > 0 {
		log.Printf("[MIGRATION] rescaled %d subscription_plans rows from vCPU to mCPU", n)
	}
	if r := db.Exec(`UPDATE terminals SET size_cpu = size_cpu * 1000 WHERE size_cpu > 0 AND size_cpu < 100`); r.Error != nil {
		log.Printf("[MIGRATION] failed to rescale terminals.size_cpu vCPU→mCPU: %v", r.Error)
	} else if n := r.RowsAffected; n > 0 {
		log.Printf("[MIGRATION] rescaled %d terminals rows from vCPU to mCPU", n)
	}
}

func migrateGroupRoles(db *gorm.DB) {
	sqldb := db.Session(&gorm.Session{})

	// assistant → member
	result := sqldb.Exec("UPDATE group_members SET role = 'member' WHERE role = 'assistant'")
	if result.RowsAffected > 0 {
		log.Printf("[MIGRATION] Migrated %d group members from 'assistant' to 'member'", result.RowsAffected)
	}

	// admin → manager
	result = sqldb.Exec("UPDATE group_members SET role = 'manager' WHERE role = 'admin'")
	if result.RowsAffected > 0 {
		log.Printf("[MIGRATION] Migrated %d group members from 'admin' to 'manager'", result.RowsAffected)
	}
}

// migrateHintContentToHints converts existing HintContent on ScenarioStep records
// into ScenarioStepHint records. Idempotent: only creates hints for steps that
// don't already have any.
func migrateHintContentToHints(db *gorm.DB) {
	// Skip if hints already exist (migration already ran)
	var hintCount int64
	db.Model(&scenarioModels.ScenarioStepHint{}).Count(&hintCount)
	if hintCount > 0 {
		return
	}

	var steps []scenarioModels.ScenarioStep
	db.Where("hint_content != '' AND hint_content IS NOT NULL").Find(&steps)
	for _, step := range steps {
		var count int64
		db.Model(&scenarioModels.ScenarioStepHint{}).Where("step_id = ?", step.ID).Count(&count)
		if count > 0 {
			continue
		}
		parts := scenarioServices.SplitHintContent(step.HintContent)
		for i, part := range parts {
			db.Create(&scenarioModels.ScenarioStepHint{
				StepID:  step.ID,
				Level:   i + 1,
				Content: part,
			})
		}
	}
}

// migrateInlineContentToProjectFiles converts inline scripts and markdown content
// on ScenarioStep and Scenario records into ProjectFile records. Idempotent: only
// migrates fields where the corresponding FK is still NULL.
func migrateInlineContentToProjectFiles(db *gorm.DB) {
	// --- Step-level migration ---
	var steps []scenarioModels.ScenarioStep
	db.Where(
		"(verify_script != '' AND verify_script IS NOT NULL AND verify_script_id IS NULL) OR "+
			"(background_script != '' AND background_script IS NOT NULL AND background_script_id IS NULL) OR "+
			"(foreground_script != '' AND foreground_script IS NOT NULL AND foreground_script_id IS NULL) OR "+
			"(text_content != '' AND text_content IS NOT NULL AND text_file_id IS NULL) OR "+
			"(hint_content != '' AND hint_content IS NOT NULL AND hint_file_id IS NULL)",
	).Find(&steps)

	if len(steps) > 0 {
		// Group steps by scenario for per-scenario transactions
		scenarioSteps := make(map[string][]scenarioModels.ScenarioStep)
		for _, step := range steps {
			key := step.ScenarioID.String()
			scenarioSteps[key] = append(scenarioSteps[key], step)
		}

		for _, stepsForScenario := range scenarioSteps {
			err := db.Transaction(func(tx *gorm.DB) error {
				for _, step := range stepsForScenario {
					stepDir := fmt.Sprintf("step%d", step.Order+1)

					if step.VerifyScript != "" && step.VerifyScriptID == nil {
						file := scenarioModels.ProjectFile{
							Name:        "verify.sh",
							RelPath:     stepDir + "/verify.sh",
							ContentType: "script",
							Content:     step.VerifyScript,
							StorageType: "database",
							SizeBytes:   int64(len(step.VerifyScript)),
						}
						if err := tx.Create(&file).Error; err != nil {
							return fmt.Errorf("failed to create verify ProjectFile for step %s: %w", step.ID, err)
						}
						if err := tx.Model(&step).Update("verify_script_id", file.ID).Error; err != nil {
							return fmt.Errorf("failed to update verify_script_id for step %s: %w", step.ID, err)
						}
					}

					if step.BackgroundScript != "" && step.BackgroundScriptID == nil {
						file := scenarioModels.ProjectFile{
							Name:        "background.sh",
							RelPath:     stepDir + "/background.sh",
							ContentType: "script",
							Content:     step.BackgroundScript,
							StorageType: "database",
							SizeBytes:   int64(len(step.BackgroundScript)),
						}
						if err := tx.Create(&file).Error; err != nil {
							return fmt.Errorf("failed to create background ProjectFile for step %s: %w", step.ID, err)
						}
						if err := tx.Model(&step).Update("background_script_id", file.ID).Error; err != nil {
							return fmt.Errorf("failed to update background_script_id for step %s: %w", step.ID, err)
						}
					}

					if step.ForegroundScript != "" && step.ForegroundScriptID == nil {
						file := scenarioModels.ProjectFile{
							Name:        "foreground.sh",
							RelPath:     stepDir + "/foreground.sh",
							ContentType: "script",
							Content:     step.ForegroundScript,
							StorageType: "database",
							SizeBytes:   int64(len(step.ForegroundScript)),
						}
						if err := tx.Create(&file).Error; err != nil {
							return fmt.Errorf("failed to create foreground ProjectFile for step %s: %w", step.ID, err)
						}
						if err := tx.Model(&step).Update("foreground_script_id", file.ID).Error; err != nil {
							return fmt.Errorf("failed to update foreground_script_id for step %s: %w", step.ID, err)
						}
					}

					if step.TextContent != "" && step.TextFileID == nil {
						file := scenarioModels.ProjectFile{
							Name:        "text.md",
							RelPath:     stepDir + "/text.md",
							ContentType: "markdown",
							Content:     step.TextContent,
							StorageType: "database",
							SizeBytes:   int64(len(step.TextContent)),
						}
						if err := tx.Create(&file).Error; err != nil {
							return fmt.Errorf("failed to create text ProjectFile for step %s: %w", step.ID, err)
						}
						if err := tx.Model(&step).Update("text_file_id", file.ID).Error; err != nil {
							return fmt.Errorf("failed to update text_file_id for step %s: %w", step.ID, err)
						}
					}

					if step.HintContent != "" && step.HintFileID == nil {
						file := scenarioModels.ProjectFile{
							Name:        "hint.md",
							RelPath:     stepDir + "/hint.md",
							ContentType: "markdown",
							Content:     step.HintContent,
							StorageType: "database",
							SizeBytes:   int64(len(step.HintContent)),
						}
						if err := tx.Create(&file).Error; err != nil {
							return fmt.Errorf("failed to create hint ProjectFile for step %s: %w", step.ID, err)
						}
						if err := tx.Model(&step).Update("hint_file_id", file.ID).Error; err != nil {
							return fmt.Errorf("failed to update hint_file_id for step %s: %w", step.ID, err)
						}
					}
				}
				return nil
			})
			if err != nil {
				log.Printf("[MIGRATION] Failed to migrate inline content for scenario steps: %v", err)
			}
		}
	}

	// --- Scenario-level migration (intro_text, finish_text) ---
	var scenarios []scenarioModels.Scenario
	db.Where(
		"(intro_text != '' AND intro_text IS NOT NULL AND intro_file_id IS NULL) OR "+
			"(finish_text != '' AND finish_text IS NOT NULL AND finish_file_id IS NULL)",
	).Find(&scenarios)

	for _, scenario := range scenarios {
		err := db.Transaction(func(tx *gorm.DB) error {
			if scenario.IntroText != "" && scenario.IntroFileID == nil {
				file := scenarioModels.ProjectFile{
					Name:        "intro.md",
					RelPath:     "intro.md",
					ContentType: "markdown",
					Content:     scenario.IntroText,
					StorageType: "database",
					SizeBytes:   int64(len(scenario.IntroText)),
				}
				if err := tx.Create(&file).Error; err != nil {
					return fmt.Errorf("failed to create intro ProjectFile for scenario %s: %w", scenario.ID, err)
				}
				if err := tx.Model(&scenario).Update("intro_file_id", file.ID).Error; err != nil {
					return fmt.Errorf("failed to update intro_file_id for scenario %s: %w", scenario.ID, err)
				}
			}

			if scenario.FinishText != "" && scenario.FinishFileID == nil {
				file := scenarioModels.ProjectFile{
					Name:        "finish.md",
					RelPath:     "finish.md",
					ContentType: "markdown",
					Content:     scenario.FinishText,
					StorageType: "database",
					SizeBytes:   int64(len(scenario.FinishText)),
				}
				if err := tx.Create(&file).Error; err != nil {
					return fmt.Errorf("failed to create finish ProjectFile for scenario %s: %w", scenario.ID, err)
				}
				if err := tx.Model(&scenario).Update("finish_file_id", file.ID).Error; err != nil {
					return fmt.Errorf("failed to update finish_file_id for scenario %s: %w", scenario.ID, err)
				}
			}

			return nil
		})
		if err != nil {
			log.Printf("[MIGRATION] Failed to migrate inline content for scenario %s: %v", scenario.ID, err)
		}
	}
}
