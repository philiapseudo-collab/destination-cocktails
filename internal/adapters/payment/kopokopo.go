package payment

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/dumu-tech/destination-cocktails/internal/config"
	"github.com/dumu-tech/destination-cocktails/internal/core"
)

// Client handles Kopo Kopo payment operations
type Client struct {
	baseURL     string
	accessToken string
	secret      string
	tillNumber  string
	webhookURL  string
	httpClient  *http.Client
}

// NewClient creates a new Kopo Kopo payment client
func NewClient() (*Client, error) {
	cfg := config.Get()
	return &Client{
		baseURL:     cfg.KopoKopoBaseURL,
		accessToken: cfg.KopoKopoAccessToken,
		secret:      cfg.KopoKopoSecret,
		tillNumber:  cfg.KopoKopoTillNumber,
		webhookURL:  cfg.WebhookBaseURL + "/api/webhooks/payment",
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

// STKPushRequest represents the Kopo Kopo STK Push request payload
type STKPushRequest struct {
	PaymentChannel string `json:"payment_channel"`
	TillNumber     string `json:"till_number"`
	Subscriber     struct {
		PhoneNumber string `json:"phone_number"`
	} `json:"subscriber"`
	Amount struct {
		Currency string `json:"currency"`
		Value    string `json:"value"`
	} `json:"amount"`
	Metadata struct {
		OrderID string `json:"order_id"`
	} `json:"metadata"`
	Links struct {
		CallbackURL string `json:"callback_url"`
	} `json:"_links"`
}

// STKPushResponse represents the Kopo Kopo STK Push response
type STKPushResponse struct {
	ID          string `json:"id"`
	Status      string `json:"status"`
	Reference   string `json:"reference,omitempty"`
	Description string `json:"description,omitempty"`
}

// InitiateSTKPush initiates an M-Pesa STK Push request
func (c *Client) InitiateSTKPush(ctx context.Context, phone string, amount float64, orderID string) (string, error) {
	// Sanitize phone number: remove leading 0, ensure +254 format
	phone = sanitizePhone(phone)

	// Format amount as string (Kopo Kopo expects string)
	amountStr := fmt.Sprintf("%.0f", amount)

	// Build request payload
	payload := STKPushRequest{
		PaymentChannel: "M-PESA",
		TillNumber:     c.tillNumber,
	}
	payload.Subscriber.PhoneNumber = phone
	payload.Amount.Currency = "KES"
	payload.Amount.Value = amountStr
	payload.Metadata.OrderID = orderID
	payload.Links.CallbackURL = c.webhookURL

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal STK push request: %w", err)
	}

	// Make API request
	url := fmt.Sprintf("%s/api/v1/stk_push_requests", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.accessToken))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send STK push request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("kopokopo API error: status %d, body: %s", resp.StatusCode, string(body))
	}

	var stkResponse STKPushResponse
	if err := json.Unmarshal(body, &stkResponse); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	return stkResponse.Reference, nil
}

// VerifyWebhook verifies the X-KopoKopo-Signature header
func (c *Client) VerifyWebhook(ctx context.Context, signature string, payload []byte) bool {
	// Signature format: sha256=<hex_string>
	parts := strings.Split(signature, "=")
	if len(parts) != 2 || parts[0] != "sha256" {
		return false
	}

	expectedSig, err := hex.DecodeString(parts[1])
	if err != nil {
		return false
	}

	mac := hmac.New(sha256.New, []byte(c.secret))
	mac.Write(payload)
	computedSig := mac.Sum(nil)

	return hmac.Equal(expectedSig, computedSig)
}

// PaymentWebhookPayload represents the Kopo Kopo webhook payload
type PaymentWebhookPayload struct {
	EventType string `json:"event_type"`
	Resource  struct {
		ID          string `json:"id"`
		Status      string `json:"status"`
		Reference   string `json:"reference"`
		Amount      string `json:"amount"`
		Metadata    struct {
			OrderID string `json:"order_id"`
		} `json:"metadata"`
	} `json:"resource"`
}

// ProcessWebhook processes the payment webhook and extracts order information
func (c *Client) ProcessWebhook(ctx context.Context, payload []byte) (*core.PaymentWebhook, error) {
	var webhook PaymentWebhookPayload
	if err := json.Unmarshal(payload, &webhook); err != nil {
		return nil, fmt.Errorf("failed to parse webhook payload: %w", err)
	}

	// Check if this is a successful transaction
	// Kopo Kopo uses event_type like "buygoods_transaction_received" or similar
	// and status like "Success" or "Received"
	isSuccess := (webhook.EventType == "buygoods_transaction_received" || 
	              strings.Contains(strings.ToLower(webhook.EventType), "transaction")) &&
	             (webhook.Resource.Status == "Success" || 
	              webhook.Resource.Status == "Received" ||
	              strings.ToLower(webhook.Resource.Status) == "success")

	result := &core.PaymentWebhook{
		OrderID:   webhook.Resource.Metadata.OrderID,
		Status:    webhook.Resource.Status,
		Reference: webhook.Resource.Reference,
		Success:   isSuccess,
	}

	// Parse amount if available
	if webhook.Resource.Amount != "" {
		var amount float64
		if _, err := fmt.Sscanf(webhook.Resource.Amount, "%f", &amount); err == nil {
			result.Amount = amount
		}
	}

	return result, nil
}

// sanitizePhone converts phone number to E.164 format (+254...)
func sanitizePhone(phone string) string {
	// Remove all spaces and dashes
	phone = strings.ReplaceAll(phone, " ", "")
	phone = strings.ReplaceAll(phone, "-", "")

	// Remove leading + if present
	if strings.HasPrefix(phone, "+") {
		phone = phone[1:]
	}

	// If starts with 0, replace with 254
	if strings.HasPrefix(phone, "0") {
		phone = "254" + phone[1:]
	}

	// If doesn't start with 254, add it
	if !strings.HasPrefix(phone, "254") {
		phone = "254" + phone
	}

	// Add + prefix
	return "+" + phone
}
