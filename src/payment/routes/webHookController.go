package paymentController

import (
	"net/http"
	"soli/formations/src/auth/errors"
	"soli/formations/src/payment/models"
	"soli/formations/src/payment/services"
	"soli/formations/src/utils"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Webhook Controller pour traiter les événements Stripe
type WebhookController interface {
	HandleStripeWebhook(ctx *gin.Context)
}

type webhookController struct {
	stripeService services.StripeService
	db            *gorm.DB // ✅ SECURITY: Use database instead of in-memory map
}

func NewWebhookController(db *gorm.DB) WebhookController {
	return &webhookController{
		stripeService: services.NewStripeService(db),
		db:            db,
	}
	// ✅ SECURITY: Cleanup is now handled by a separate cron job
	// This prevents memory leaks and persists across restarts
}

// NewWebhookControllerWithService is a test-only constructor that allows
// injecting a custom StripeService implementation (typically a mock).
// Production code should always use NewWebhookController.
func NewWebhookControllerWithService(db *gorm.DB, stripeService services.StripeService) WebhookController {
	return &webhookController{
		stripeService: stripeService,
		db:            db,
	}
}

// Handle Stripe Webhook godoc
//
//	@Summary		Traiter les webhooks Stripe
//	@Description	Endpoint pour recevoir et traiter les événements de Stripe
//	@Tags			webhooks
//	@Accept			json
//	@Produce		json
//	@Success		200	{object}	string
//	@Failure		400	{object}	errors.APIError	"Invalid webhook"
//	@Router			/webhooks/stripe [post]
func (wc *webhookController) HandleStripeWebhook(ctx *gin.Context) {
	// 1 : Vérifications de sécurité de base
	if !wc.basicSecurityChecks(ctx) {
		return // La réponse d'erreur est déjà envoyée
	}

	// 2 : Récupérer et valider le payload
	payload, signature, valid := wc.validatePayloadAndSignature(ctx)
	if !valid {
		return // La réponse d'erreur est déjà envoyée
	}

	// 3 : Validation de la signature Stripe
	event, err := wc.stripeService.ValidateWebhookSignature(payload, signature)
	if err != nil {
		utils.Debug("🚨 Webhook signature validation failed from IP %s: %v", ctx.ClientIP(), err)
		ctx.JSON(http.StatusUnauthorized, &errors.APIError{
			ErrorCode:    http.StatusUnauthorized,
			ErrorMessage: "Invalid webhook signature",
		})
		return
	}

	// 4 : Vérifier l'âge de l'événement (anti-replay)
	eventTime := time.Unix(event.Created, 0)
	if time.Since(eventTime) > 10*time.Minute {
		utils.Debug("🕐 Event %s too old (%v), rejecting", event.ID, time.Since(eventTime))
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Event too old",
		})
		return
	}

	// 5 : Reserve the event atomically BEFORE processing.
	// Uses INSERT ... ON CONFLICT DO NOTHING on the unique event_id.
	// - RowsAffected == 1 -> we own processing for this event.
	// - RowsAffected == 0 -> another pod (or an earlier delivery) already
	//   reserved/processed this event; return 200 so Stripe stops retrying.
	//
	// This replaces the former check-then-act flow (SELECT then INSERT) which
	// let two concurrent deliveries both pass the check and both run the
	// handler, causing duplicate side effects.
	reserved, err := wc.reserveEvent(event.ID, string(event.Type), payload)
	if err != nil {
		utils.Debug("⚠️ Failed to reserve event %s: %v", event.ID, err)
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to reserve event",
		})
		return
	}
	if !reserved {
		utils.Debug("🔄 Event %s already reserved/processed (IP %s)", event.ID, ctx.ClientIP())
		ctx.JSON(http.StatusOK, gin.H{"message": "Event already reserved or processed"})
		return
	}

	// 6 : Traitement synchrone — Stripe accorde 20 secondes pour répondre.
	// On failure, release the reservation so Stripe's retry can succeed.
	if err := wc.stripeService.ProcessWebhook(payload, signature); err != nil {
		utils.Debug("❌ Webhook processing failed for event %s: %v", event.ID, err)
		wc.releaseReservation(event.ID)
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Webhook processing failed",
		})
		return
	}

	utils.Debug("✅ Successfully processed webhook event %s", event.ID)

	// 7 : Réponse de succès à Stripe — reservation row remains as processed marker.
	ctx.JSON(http.StatusOK, gin.H{
		"received":  true,
		"event_id":  event.ID,
		"timestamp": time.Now().Unix(),
	})
}

// 🔐 Vérifications de sécurité de base
func (wc *webhookController) basicSecurityChecks(ctx *gin.Context) bool {
	// Vérification User-Agent
	userAgent := ctx.GetHeader("User-Agent")
	if !strings.Contains(userAgent, "Stripe") {
		utils.Debug("🚨 Invalid User-Agent from IP %s: %s", ctx.ClientIP(), userAgent)
		ctx.JSON(http.StatusForbidden, &errors.APIError{
			ErrorCode:    http.StatusForbidden,
			ErrorMessage: "Invalid request source",
		})
		return false
	}

	// Vérification Content-Type
	contentType := ctx.GetHeader("Content-Type")
	if !strings.Contains(contentType, "application/json") {
		utils.Debug("🚨 Invalid Content-Type from IP %s: %s", ctx.ClientIP(), contentType)
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Invalid content type",
		})
		return false
	}

	return true
}

func (wc *webhookController) validatePayloadAndSignature(ctx *gin.Context) ([]byte, string, bool) {
	// Récupérer le payload brut
	payload, err := ctx.GetRawData()
	if err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Failed to read request body",
		})
		return nil, "", false
	}

	// Vérifier la taille (protection contre les gros payloads)
	if len(payload) > 1024*1024 { // 1MB max
		utils.Debug("🚨 Payload too large from IP %s: %d bytes", ctx.ClientIP(), len(payload))
		ctx.JSON(http.StatusRequestEntityTooLarge, &errors.APIError{
			ErrorCode:    http.StatusRequestEntityTooLarge,
			ErrorMessage: "Payload too large",
		})
		return nil, "", false
	}

	// Récupérer la signature
	signature := ctx.GetHeader("Stripe-Signature")
	if signature == "" {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Missing Stripe signature",
		})
		return nil, "", false
	}

	return payload, signature, true
}

// reserveEvent atomically attempts to claim processing ownership of an event
// using INSERT ... ON CONFLICT (event_id) DO NOTHING on webhook_events.
//
// Returns (true, nil) if this caller successfully reserved the event and
// should proceed to process it. Returns (false, nil) if another caller has
// already reserved/processed this event (idempotent short-circuit).
// Returns (false, err) on unexpected DB errors.
func (wc *webhookController) reserveEvent(eventID, eventType string, payload []byte) (bool, error) {
	now := time.Now()
	row := &models.WebhookEvent{
		EventID:     eventID,
		EventType:   eventType,
		ProcessedAt: now,
		ExpiresAt:   now.Add(24 * time.Hour),
		Payload:     string(payload),
	}
	tx := wc.db.Clauses(clause.OnConflict{DoNothing: true}).Create(row)
	if tx.Error != nil {
		return false, tx.Error
	}
	// RowsAffected == 1 -> we inserted (reserved); 0 -> conflict (already reserved).
	return tx.RowsAffected == 1, nil
}

// releaseReservation hard-deletes the webhook_events row for the given event
// so Stripe's retry can pass reservation again after a processing failure.
// Unscoped is used defensively even though WebhookEvent has no soft-delete.
func (wc *webhookController) releaseReservation(eventID string) {
	if err := wc.db.Unscoped().
		Where("event_id = ?", eventID).
		Delete(&models.WebhookEvent{}).Error; err != nil {
		utils.Debug("⚠️ Failed to release reservation for event %s: %v", eventID, err)
	}
}

// ✅ SECURITY: Cleanup is now handled by a separate cron job
// See: src/cron/webhookCleanup.go
