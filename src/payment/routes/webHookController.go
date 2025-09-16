package paymentController

import (
	"fmt"
	"net/http"
	"soli/formations/src/auth/errors"
	"soli/formations/src/payment/services"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Webhook Controller pour traiter les événements Stripe
type WebhookController interface {
	HandleStripeWebhook(ctx *gin.Context)
}

type webhookController struct {
	stripeService   services.StripeService
	processedEvents map[string]time.Time
	eventMutex      sync.RWMutex
}

func NewWebhookController(db *gorm.DB) WebhookController {
	controller := &webhookController{
		stripeService:   services.NewStripeService(db),
		processedEvents: make(map[string]time.Time),
	}

	go controller.cleanupProcessedEvents()

	return controller
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
		fmt.Printf("🚨 Webhook signature validation failed from IP %s: %v\n", ctx.ClientIP(), err)
		ctx.JSON(http.StatusUnauthorized, &errors.APIError{
			ErrorCode:    http.StatusUnauthorized,
			ErrorMessage: "Invalid webhook signature",
		})
		return
	}

	// 4 : Prévention des attaques par rejeu
	if wc.isEventProcessed(event.ID) {
		fmt.Printf("🔄 Duplicate event %s from IP %s\n", event.ID, ctx.ClientIP())
		ctx.JSON(http.StatusOK, gin.H{"message": "Event already processed"})
		return
	}

	// 5 : Vérifier l'âge de l'événement (anti-replay)
	eventTime := time.Unix(event.Created, 0)
	if time.Since(eventTime) > 5*time.Minute {
		fmt.Printf("🕐 Event %s too old (%v), rejecting\n", event.ID, time.Since(eventTime))
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
				fmt.Printf("🚨 Webhook processing panic for event %s: %v\n", event.ID, r)
			}
		}()

		if err := wc.stripeService.ProcessWebhook(payload, signature); err != nil {
			fmt.Printf("❌ Webhook processing failed for event %s: %v\n", event.ID, err)
			// TODO: Dans un futur système, envoyer dans une queue pour retry
		} else {
			fmt.Printf("✅ Successfully processed webhook event %s\n", event.ID)
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
	if !contains(userAgent, "Stripe") {
		fmt.Printf("🚨 Invalid User-Agent from IP %s: %s\n", ctx.ClientIP(), userAgent)
		ctx.JSON(http.StatusForbidden, &errors.APIError{
			ErrorCode:    http.StatusForbidden,
			ErrorMessage: "Invalid request source",
		})
		return false
	}

	// Vérification Content-Type
	contentType := ctx.GetHeader("Content-Type")
	if !contains(contentType, "application/json") {
		fmt.Printf("🚨 Invalid Content-Type from IP %s: %s\n", ctx.ClientIP(), contentType)
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
		fmt.Printf("🚨 Payload too large from IP %s: %d bytes\n", ctx.ClientIP(), len(payload))
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

// 🔐 Gestion des événements déjà traités (anti-replay simple)
func (wc *webhookController) isEventProcessed(eventID string) bool {
	wc.eventMutex.RLock()
	defer wc.eventMutex.RUnlock()

	_, exists := wc.processedEvents[eventID]
	return exists
}

func (wc *webhookController) markEventProcessed(eventID string) {
	wc.eventMutex.Lock()
	defer wc.eventMutex.Unlock()

	wc.processedEvents[eventID] = time.Now()
}

// Nettoyage périodique des événements traités
func (wc *webhookController) cleanupProcessedEvents() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		wc.eventMutex.Lock()
		cutoff := time.Now().Add(-24 * time.Hour)

		for eventID, processedAt := range wc.processedEvents {
			if processedAt.Before(cutoff) {
				delete(wc.processedEvents, eventID)
			}
		}
		wc.eventMutex.Unlock()

		fmt.Printf("🧹 Cleaned up old processed events, current count: %d\n", len(wc.processedEvents))
	}
}

// Utilitaire
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr ||
		(len(s) > len(substr) &&
			(s[:len(substr)] == substr ||
				s[len(s)-len(substr):] == substr ||
				strings.Contains(s, substr))))
}
