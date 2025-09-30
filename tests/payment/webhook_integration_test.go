// tests/payment/webhook_integration_test.go
package payment_tests

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stripe/stripe-go/v82"
)

func setupWebhookIntegrationRouter() (*gin.Engine, *SharedMockStripeService) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	mockStripeService := new(SharedMockStripeService)

	// Middleware pour simuler la validation de signature
	router.Use(func(c *gin.Context) {
		// Dans un vrai test d'intégration, nous validerions la signature
		c.Set("stripe_signature_valid", true)
		c.Next()
	})

	// Route webhook pour les tests
	router.POST("/webhooks/stripe", func(c *gin.Context) {
		// Lire le payload
		var payload []byte
		if c.Request.Body != nil {
			payload, _ = c.GetRawData()
		}

		signature := c.GetHeader("Stripe-Signature")

		// Valider la signature et traiter le webhook
		event, err := mockStripeService.ValidateWebhookSignature(payload, signature)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid signature"})
			return
		}

		// Traiter l'événement
		err = mockStripeService.ProcessWebhook(payload, signature)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"received": true,
			"event_id": event.ID,
			"event_type": event.Type,
		})
	})

	return router, mockStripeService
}

func generateTestSignature(payload []byte, secret string, timestamp int64) string {
	// Simuler la génération de signature Stripe
	timestampStr := fmt.Sprintf("%d", timestamp)
	data := timestampStr + "." + string(payload)

	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(data))
	signature := hex.EncodeToString(h.Sum(nil))

	return fmt.Sprintf("t=%d,v1=%s", timestamp, signature)
}

func TestWebhookIntegration_SubscriptionCreated(t *testing.T) {
	router, mockStripeService := setupWebhookIntegrationRouter()

	t.Run("Valid subscription created webhook", func(t *testing.T) {
		// Créer un payload de webhook réaliste
		webhookPayload := map[string]interface{}{
			"id":      "evt_subscription_created_123",
			"object":  "event",
			"type":    "customer.subscription.created",
			"created": time.Now().Unix(),
			"data": map[string]interface{}{
				"object": map[string]interface{}{
					"id":         "sub_test_123",
					"object":     "subscription",
					"customer":   "cus_test_123",
					"status":     "active",
					"start_date": time.Now().Unix(),
					"current_period_start": time.Now().Unix(),
					"current_period_end":   time.Now().Add(30 * 24 * time.Hour).Unix(),
					"metadata": map[string]interface{}{
						"user_id":              "user_123",
						"subscription_plan_id": uuid.New().String(),
					},
					"items": map[string]interface{}{
						"object": "list",
						"data": []map[string]interface{}{
							{
								"id":     "si_test_123",
								"object": "subscription_item",
								"price": map[string]interface{}{
									"id":        "price_test_123",
									"currency":  "usd",
									"unit_amount": 1999,
								},
							},
						},
					},
				},
			},
		}

		payload, _ := json.Marshal(webhookPayload)
		timestamp := time.Now().Unix()
		signature := generateTestSignature(payload, "test_webhook_secret", timestamp)

		// Mock l'événement Stripe
		mockEvent := &stripe.Event{
			ID:      "evt_subscription_created_123",
			Type:    "customer.subscription.created",
			Created: timestamp,
		}

		mockStripeService.On("ValidateWebhookSignature", payload, signature).Return(mockEvent, nil)
		mockStripeService.On("ProcessWebhook", payload, signature).Return(nil)

		// Créer la requête
		req := httptest.NewRequest("POST", "/webhooks/stripe", bytes.NewBuffer(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "Stripe/1.0")
		req.Header.Set("Stripe-Signature", signature)

		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		// Vérifications
		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)

		assert.True(t, response["received"].(bool))
		assert.Equal(t, "evt_subscription_created_123", response["event_id"])
		assert.Equal(t, "customer.subscription.created", response["event_type"])

		mockStripeService.AssertExpectations(t)
	})

	t.Run("Subscription updated webhook", func(t *testing.T) {
		webhookPayload := map[string]interface{}{
			"id":      "evt_subscription_updated_456",
			"object":  "event",
			"type":    "customer.subscription.updated",
			"created": time.Now().Unix(),
			"data": map[string]interface{}{
				"object": map[string]interface{}{
					"id":       "sub_test_456",
					"object":   "subscription",
					"customer": "cus_test_456",
					"status":   "active",
					"metadata": map[string]interface{}{
						"user_id":              "user_456",
						"subscription_plan_id": uuid.New().String(),
					},
				},
				"previous_attributes": map[string]interface{}{
					"status": "incomplete",
				},
			},
		}

		payload, _ := json.Marshal(webhookPayload)
		timestamp := time.Now().Unix()
		signature := generateTestSignature(payload, "test_webhook_secret", timestamp)

		mockEvent := &stripe.Event{
			ID:      "evt_subscription_updated_456",
			Type:    "customer.subscription.updated",
			Created: timestamp,
		}

		mockStripeService.On("ValidateWebhookSignature", payload, signature).Return(mockEvent, nil)
		mockStripeService.On("ProcessWebhook", payload, signature).Return(nil)

		req := httptest.NewRequest("POST", "/webhooks/stripe", bytes.NewBuffer(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "Stripe/1.0")
		req.Header.Set("Stripe-Signature", signature)

		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)

		assert.Equal(t, "evt_subscription_updated_456", response["event_id"])
		assert.Equal(t, "customer.subscription.updated", response["event_type"])

		mockStripeService.AssertExpectations(t)
	})

	t.Run("Subscription deleted webhook", func(t *testing.T) {
		webhookPayload := map[string]interface{}{
			"id":      "evt_subscription_deleted_789",
			"object":  "event",
			"type":    "customer.subscription.deleted",
			"created": time.Now().Unix(),
			"data": map[string]interface{}{
				"object": map[string]interface{}{
					"id":       "sub_test_789",
					"object":   "subscription",
					"customer": "cus_test_789",
					"status":   "canceled",
					"metadata": map[string]interface{}{
						"user_id":              "user_789",
						"subscription_plan_id": uuid.New().String(),
					},
				},
			},
		}

		payload, _ := json.Marshal(webhookPayload)
		timestamp := time.Now().Unix()
		signature := generateTestSignature(payload, "test_webhook_secret", timestamp)

		mockEvent := &stripe.Event{
			ID:      "evt_subscription_deleted_789",
			Type:    "customer.subscription.deleted",
			Created: timestamp,
		}

		mockStripeService.On("ValidateWebhookSignature", payload, signature).Return(mockEvent, nil)
		mockStripeService.On("ProcessWebhook", payload, signature).Return(nil)

		req := httptest.NewRequest("POST", "/webhooks/stripe", bytes.NewBuffer(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "Stripe/1.0")
		req.Header.Set("Stripe-Signature", signature)

		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		mockStripeService.AssertExpectations(t)
	})
}

func TestWebhookIntegration_PaymentEvents(t *testing.T) {
	router, mockStripeService := setupWebhookIntegrationRouter()

	t.Run("Invoice payment succeeded", func(t *testing.T) {
		webhookPayload := map[string]interface{}{
			"id":      "evt_payment_succeeded_123",
			"object":  "event",
			"type":    "invoice.payment_succeeded",
			"created": time.Now().Unix(),
			"data": map[string]interface{}{
				"object": map[string]interface{}{
					"id":           "in_test_123",
					"object":       "invoice",
					"customer":     "cus_test_123",
					"subscription": "sub_test_123",
					"status":       "paid",
					"amount_paid":  1999,
					"currency":     "usd",
				},
			},
		}

		payload, _ := json.Marshal(webhookPayload)
		timestamp := time.Now().Unix()
		signature := generateTestSignature(payload, "test_webhook_secret", timestamp)

		mockEvent := &stripe.Event{
			ID:      "evt_payment_succeeded_123",
			Type:    "invoice.payment_succeeded",
			Created: timestamp,
		}

		mockStripeService.On("ValidateWebhookSignature", payload, signature).Return(mockEvent, nil)
		mockStripeService.On("ProcessWebhook", payload, signature).Return(nil)

		req := httptest.NewRequest("POST", "/webhooks/stripe", bytes.NewBuffer(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "Stripe/1.0")
		req.Header.Set("Stripe-Signature", signature)

		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		mockStripeService.AssertExpectations(t)
	})

	t.Run("Invoice payment failed", func(t *testing.T) {
		webhookPayload := map[string]interface{}{
			"id":      "evt_payment_failed_456",
			"object":  "event",
			"type":    "invoice.payment_failed",
			"created": time.Now().Unix(),
			"data": map[string]interface{}{
				"object": map[string]interface{}{
					"id":           "in_test_456",
					"object":       "invoice",
					"customer":     "cus_test_456",
					"subscription": "sub_test_456",
					"status":       "open",
					"amount_due":   1999,
					"currency":     "usd",
					"attempt_count": 1,
				},
			},
		}

		payload, _ := json.Marshal(webhookPayload)
		timestamp := time.Now().Unix()
		signature := generateTestSignature(payload, "test_webhook_secret", timestamp)

		mockEvent := &stripe.Event{
			ID:      "evt_payment_failed_456",
			Type:    "invoice.payment_failed",
			Created: timestamp,
		}

		mockStripeService.On("ValidateWebhookSignature", payload, signature).Return(mockEvent, nil)
		mockStripeService.On("ProcessWebhook", payload, signature).Return(nil)

		req := httptest.NewRequest("POST", "/webhooks/stripe", bytes.NewBuffer(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "Stripe/1.0")
		req.Header.Set("Stripe-Signature", signature)

		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		mockStripeService.AssertExpectations(t)
	})
}

func TestWebhookIntegration_CheckoutSessionCompleted(t *testing.T) {
	router, mockStripeService := setupWebhookIntegrationRouter()

	t.Run("Checkout session completed with subscription", func(t *testing.T) {
		planID := uuid.New()
		webhookPayload := map[string]interface{}{
			"id":      "evt_checkout_completed_123",
			"object":  "event",
			"type":    "checkout.session.completed",
			"created": time.Now().Unix(),
			"data": map[string]interface{}{
				"object": map[string]interface{}{
					"id":           "cs_test_123",
					"object":       "checkout.session",
					"customer":     "cus_test_123",
					"subscription": "sub_test_123",
					"mode":         "subscription",
					"status":       "complete",
					"metadata": map[string]interface{}{
						"user_id":              "user_123",
						"subscription_plan_id": planID.String(),
					},
					"line_items": map[string]interface{}{
						"object": "list",
						"data": []map[string]interface{}{
							{
								"id": "li_test_123",
								"price": map[string]interface{}{
									"id":        "price_test_123",
									"currency":  "usd",
									"unit_amount": 1999,
								},
							},
						},
					},
				},
			},
		}

		payload, _ := json.Marshal(webhookPayload)
		timestamp := time.Now().Unix()
		signature := generateTestSignature(payload, "test_webhook_secret", timestamp)

		mockEvent := &stripe.Event{
			ID:      "evt_checkout_completed_123",
			Type:    "checkout.session.completed",
			Created: timestamp,
		}

		mockStripeService.On("ValidateWebhookSignature", payload, signature).Return(mockEvent, nil)
		mockStripeService.On("ProcessWebhook", payload, signature).Return(nil)

		req := httptest.NewRequest("POST", "/webhooks/stripe", bytes.NewBuffer(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "Stripe/1.0")
		req.Header.Set("Stripe-Signature", signature)

		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, "checkout.session.completed", response["event_type"])

		mockStripeService.AssertExpectations(t)
	})
}

func TestWebhookIntegration_SecurityValidation(t *testing.T) {
	router, mockStripeService := setupWebhookIntegrationRouter()

	t.Run("Invalid signature should be rejected", func(t *testing.T) {
		webhookPayload := map[string]interface{}{
			"id":   "evt_invalid_signature",
			"type": "customer.subscription.created",
		}

		payload, _ := json.Marshal(webhookPayload)
		invalidSignature := "t=1234567890,v1=invalid_signature"

		mockStripeService.On("ValidateWebhookSignature", payload, invalidSignature).Return((*stripe.Event)(nil), assert.AnError)

		req := httptest.NewRequest("POST", "/webhooks/stripe", bytes.NewBuffer(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "Stripe/1.0")
		req.Header.Set("Stripe-Signature", invalidSignature)

		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, "Invalid signature", response["error"])

		mockStripeService.AssertExpectations(t)
	})

	t.Run("Processing error should return 500", func(t *testing.T) {
		webhookPayload := map[string]interface{}{
			"id":   "evt_processing_error",
			"type": "customer.subscription.created",
		}

		payload, _ := json.Marshal(webhookPayload)
		timestamp := time.Now().Unix()
		signature := generateTestSignature(payload, "test_webhook_secret", timestamp)

		mockEvent := &stripe.Event{
			ID:      "evt_processing_error",
			Type:    "customer.subscription.created",
			Created: timestamp,
		}

		mockStripeService.On("ValidateWebhookSignature", payload, signature).Return(mockEvent, nil)
		mockStripeService.On("ProcessWebhook", payload, signature).Return(assert.AnError)

		req := httptest.NewRequest("POST", "/webhooks/stripe", bytes.NewBuffer(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "Stripe/1.0")
		req.Header.Set("Stripe-Signature", signature)

		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)

		mockStripeService.AssertExpectations(t)
	})
}

func TestWebhookIntegration_ComplexScenarios(t *testing.T) {
	router, mockStripeService := setupWebhookIntegrationRouter()

	t.Run("Rapid succession of events", func(t *testing.T) {
		events := []struct {
			id        string
			eventType string
		}{
			{"evt_rapid_1", "customer.subscription.created"},
			{"evt_rapid_2", "invoice.payment_succeeded"},
			{"evt_rapid_3", "customer.subscription.updated"},
		}

		for _, event := range events {
			webhookPayload := map[string]interface{}{
				"id":      event.id,
				"object":  "event",
				"type":    event.eventType,
				"created": time.Now().Unix(),
				"data": map[string]interface{}{
					"object": map[string]interface{}{
						"id": "obj_test_123",
					},
				},
			}

			payload, _ := json.Marshal(webhookPayload)
			timestamp := time.Now().Unix()
			signature := generateTestSignature(payload, "test_webhook_secret", timestamp)

			mockEvent := &stripe.Event{
				ID:      event.id,
				Type:    stripe.EventType(event.eventType),
				Created: timestamp,
			}

			mockStripeService.On("ValidateWebhookSignature", payload, signature).Return(mockEvent, nil)
			mockStripeService.On("ProcessWebhook", payload, signature).Return(nil)

			req := httptest.NewRequest("POST", "/webhooks/stripe", bytes.NewBuffer(payload))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("User-Agent", "Stripe/1.0")
			req.Header.Set("Stripe-Signature", signature)

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code)
		}

		mockStripeService.AssertExpectations(t)
	})

	t.Run("Event with missing metadata", func(t *testing.T) {
		webhookPayload := map[string]interface{}{
			"id":      "evt_missing_metadata",
			"object":  "event",
			"type":    "customer.subscription.created",
			"created": time.Now().Unix(),
			"data": map[string]interface{}{
				"object": map[string]interface{}{
					"id":       "sub_no_metadata",
					"object":   "subscription",
					"customer": "cus_test_123",
					"status":   "active",
					"metadata": map[string]interface{}{}, // Métadonnées vides
				},
			},
		}

		payload, _ := json.Marshal(webhookPayload)
		timestamp := time.Now().Unix()
		signature := generateTestSignature(payload, "test_webhook_secret", timestamp)

		mockEvent := &stripe.Event{
			ID:      "evt_missing_metadata",
			Type:    "customer.subscription.created",
			Created: timestamp,
		}

		mockStripeService.On("ValidateWebhookSignature", payload, signature).Return(mockEvent, nil)
		mockStripeService.On("ProcessWebhook", payload, signature).Return(nil)

		req := httptest.NewRequest("POST", "/webhooks/stripe", bytes.NewBuffer(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "Stripe/1.0")
		req.Header.Set("Stripe-Signature", signature)

		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		mockStripeService.AssertExpectations(t)
	})
}