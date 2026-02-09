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

	// 4 : Prévention des attaques par rejeu
	if wc.isEventProcessed(event.ID) {
		utils.Debug("🔄 Duplicate event %s from IP %s", event.ID, ctx.ClientIP())
		ctx.JSON(http.StatusOK, gin.H{"message": "Event already processed"})
		return
	}

	// 5 : Vérifier l'âge de l'événement (anti-replay)
	eventTime := time.Unix(event.Created, 0)
	if time.Since(eventTime) > 10*time.Minute {
		utils.Debug("🕐 Event %s too old (%v), rejecting", event.ID, time.Since(eventTime))
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Event too old",
		})
		return
	}

	// 6 : Marquer comme traité AVANT le traitement
	wc.markEventProcessed(event.ID)

	// 7 : Traitement asynchrone pour éviter les timeouts Stripe
	go func() {
		defer func() {
			if r := recover(); r != nil {
				utils.Debug("🚨 Webhook processing panic for event %s: %v", event.ID, r)
			}
		}()

		if err := wc.stripeService.ProcessWebhook(payload, signature); err != nil {
			utils.Debug("❌ Webhook processing failed for event %s: %v", event.ID, err)
			// TODO: Dans un futur système, envoyer dans une queue pour retry
		} else {
			utils.Debug("✅ Successfully processed webhook event %s", event.ID)
		}
	}()

	// 8 : Réponse immédiate à Stripe (OBLIGATOIRE)
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

// 🔐 Gestion des événements déjà traités (anti-replay avec database)
// ✅ SECURITY: Database-backed duplicate prevention (survives restarts)
func (wc *webhookController) isEventProcessed(eventID string) bool {
	var count int64
	wc.db.Model(&models.WebhookEvent{}).
		Where("event_id = ? AND expires_at > ?", eventID, time.Now()).
		Count(&count)

	return count > 0
}

func (wc *webhookController) markEventProcessed(eventID string) {
	// Create webhook event record
	webhookEvent := &models.WebhookEvent{
		EventID:     eventID,
		EventType:   "", // Will be populated from Stripe event if needed
		ProcessedAt: time.Now(),
		ExpiresAt:   time.Now().Add(24 * time.Hour), // Keep for 24 hours
	}

	if err := wc.db.Create(webhookEvent).Error; err != nil {
		utils.Debug("⚠️ Failed to mark event %s as processed: %v", eventID, err)
		// Continue anyway - better to process twice than not at all
		// The unique constraint on event_id will prevent duplicates in the database
	}
}

// ✅ SECURITY: Cleanup is now handled by a separate cron job
// See: src/cron/webhookCleanup.go
// This method has been removed - cleanup happens in background job

