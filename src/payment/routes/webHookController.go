package paymentController

import (
	"errors"
	"net/http"
	authErrors "soli/formations/src/auth/errors"
	"soli/formations/src/payment/models"
	"soli/formations/src/payment/services"
	"soli/formations/src/utils"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
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
		ctx.JSON(http.StatusUnauthorized, &authErrors.APIError{
			ErrorCode:    http.StatusUnauthorized,
			ErrorMessage: "Invalid webhook signature",
		})
		return
	}

	// 4 : Vérifier l'âge de l'événement (anti-replay)
	eventTime := time.Unix(event.Created, 0)
	if time.Since(eventTime) > 10*time.Minute {
		utils.Debug("🕐 Event %s too old (%v), rejecting", event.ID, time.Since(eventTime))
		ctx.JSON(http.StatusBadRequest, &authErrors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Event too old",
		})
		return
	}

	// 5 : Reserve the event atomically BEFORE processing.
	// Uses a status-aware reservation:
	// - no row              -> INSERT(status=reserved), reserved=true
	// - row status=reserved -> reserved=false (another pod owns it)
	// - row status=processed-> reserved=false (idempotent skip)
	// - row status=failed   -> UPDATE(status=reserved), reserved=true (re-claim)
	//
	// This replaces the former check-then-act flow (SELECT then INSERT) which
	// let two concurrent deliveries both pass the check and both run the
	// handler, causing duplicate side effects. It also fixes the "stuck
	// reservation" bug (#261): a transient DB error during cleanup no longer
	// wedges the row in `reserved` forever — failures land in `failed` and
	// are re-reservable on the next retry.
	reserved, err := wc.reserveEvent(event.ID, string(event.Type), payload)
	if err != nil {
		utils.Debug("⚠️ Failed to reserve event %s: %v", event.ID, err)
		ctx.JSON(http.StatusInternalServerError, &authErrors.APIError{
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
	// On failure, mark the reservation as failed so Stripe's retry can re-claim it.
	if err := wc.stripeService.ProcessWebhook(payload, signature); err != nil {
		utils.Debug("❌ Webhook processing failed for event %s: %v", event.ID, err)
		wc.markFailed(event.ID)
		ctx.JSON(http.StatusInternalServerError, &authErrors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Webhook processing failed",
		})
		return
	}

	// Mark the row as processed (terminal state) so future deliveries short-circuit.
	wc.markProcessed(event.ID)
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
		ctx.JSON(http.StatusForbidden, &authErrors.APIError{
			ErrorCode:    http.StatusForbidden,
			ErrorMessage: "Invalid request source",
		})
		return false
	}

	// Vérification Content-Type
	contentType := ctx.GetHeader("Content-Type")
	if !strings.Contains(contentType, "application/json") {
		utils.Debug("🚨 Invalid Content-Type from IP %s: %s", ctx.ClientIP(), contentType)
		ctx.JSON(http.StatusBadRequest, &authErrors.APIError{
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
		ctx.JSON(http.StatusBadRequest, &authErrors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Failed to read request body",
		})
		return nil, "", false
	}

	// Vérifier la taille (protection contre les gros payloads)
	if len(payload) > 1024*1024 { // 1MB max
		utils.Debug("🚨 Payload too large from IP %s: %d bytes", ctx.ClientIP(), len(payload))
		ctx.JSON(http.StatusRequestEntityTooLarge, &authErrors.APIError{
			ErrorCode:    http.StatusRequestEntityTooLarge,
			ErrorMessage: "Payload too large",
		})
		return nil, "", false
	}

	// Récupérer la signature
	signature := ctx.GetHeader("Stripe-Signature")
	if signature == "" {
		ctx.JSON(http.StatusBadRequest, &authErrors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Missing Stripe signature",
		})
		return nil, "", false
	}

	return payload, signature, true
}

// reserveEvent attempts to claim processing ownership of a webhook event.
//
// Status-aware semantics:
//   - no existing row          -> INSERT(status=reserved); returns (true, nil)
//   - existing status=reserved -> another worker owns it; returns (false, nil)
//   - existing status=processed-> idempotent terminal state; returns (false, nil)
//   - existing status=failed   -> UPDATE(status=reserved); returns (true, nil)
//
// Implemented as a transactional SELECT-then-INSERT/UPDATE for portability
// across SQLite (tests) and PostgreSQL (production). The unique index on
// event_id resolves any racing concurrent INSERTs (the loser hits the
// constraint and is treated as "another pod won the reservation").
func (wc *webhookController) reserveEvent(eventID, eventType string, payload []byte) (bool, error) {
	now := time.Now()

	var reserved bool
	err := wc.db.Transaction(func(tx *gorm.DB) error {
		var existing models.WebhookEvent
		err := tx.Where("event_id = ?", eventID).First(&existing).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// No row exists — INSERT a fresh reservation.
			row := &models.WebhookEvent{
				EventID:     eventID,
				EventType:   eventType,
				ProcessedAt: now,
				ExpiresAt:   now.Add(24 * time.Hour),
				Payload:     string(payload),
				Status:      models.WebhookEventStatusReserved,
			}
			if createErr := tx.Create(row).Error; createErr != nil {
				// A concurrent INSERT (unique index on event_id) likely won the
				// race. Treat as "not reserved by us" rather than a hard error.
				utils.Debug("reserveEvent: concurrent insert for %s: %v", eventID, createErr)
				reserved = false
				return nil
			}
			reserved = true
			return nil
		}
		if err != nil {
			return err
		}

		// Row exists — branch on its current status.
		switch existing.Status {
		case models.WebhookEventStatusFailed:
			// Re-claim a previously failed reservation. Use a WHERE clause
			// keyed on event_id (the natural key) rather than relying on the
			// struct's primary key, which may not be populated by SQLite for
			// rows seeded with a Postgres-side `gen_random_uuid()` default.
			if updateErr := tx.Model(&models.WebhookEvent{}).
				Where("event_id = ?", eventID).
				Updates(map[string]interface{}{
					"status":       models.WebhookEventStatusReserved,
					"processed_at": now,
				}).Error; updateErr != nil {
				return updateErr
			}
			reserved = true
			return nil
		case models.WebhookEventStatusReserved, models.WebhookEventStatusProcessed:
			// Another worker owns it OR we already processed it.
			reserved = false
			return nil
		default:
			// Unknown status — treat as not-reservable (defensive).
			utils.Debug("reserveEvent: unknown status %q for event %s", existing.Status, eventID)
			reserved = false
			return nil
		}
	})
	if err != nil {
		return false, err
	}
	return reserved, nil
}

// markProcessed transitions an existing reservation row to status=processed
// after ProcessWebhook ran to success. This is the terminal state — future
// deliveries for the same event_id short-circuit on the row.
func (wc *webhookController) markProcessed(eventID string) {
	if err := wc.db.Model(&models.WebhookEvent{}).
		Where("event_id = ?", eventID).
		Updates(map[string]interface{}{
			"status":       models.WebhookEventStatusProcessed,
			"processed_at": time.Now(),
		}).Error; err != nil {
		utils.Debug("⚠️ Failed to mark event %s as processed: %v", eventID, err)
	}
}

// markFailed transitions an existing reservation row to status=failed after
// ProcessWebhook returned an error. Replaces the previous hard DELETE: the
// row stays around so a transient DB glitch on cleanup can no longer wedge
// the event in `reserved` forever — `failed` is re-reservable.
func (wc *webhookController) markFailed(eventID string) {
	if err := wc.db.Model(&models.WebhookEvent{}).
		Where("event_id = ?", eventID).
		Update("status", models.WebhookEventStatusFailed).Error; err != nil {
		utils.Debug("⚠️ Failed to mark event %s as failed: %v", eventID, err)
	}
}

// ✅ SECURITY: Cleanup is now handled by a separate cron job
// See: src/cron/webhookCleanup.go
