package payment

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/dumu-tech/destination-cocktails/internal/config"
	"github.com/dumu-tech/destination-cocktails/internal/core"
)

// stkPayload represents a queued STK Push request
type stkPayload struct {
	orderID string
	phone   string
	amount  float64
}

// Client handles Kopo Kopo payment operations with rate limiting
type Client struct {
	baseURL         string
	webhookSecret   string
	tillNumber      string
	callbackURL     string
	httpClient      *http.Client
	// OAuth: used when KOPOKOPO_ACCESS_TOKEN is not set
	clientID        string
	clientSecret    string
	accessToken     string
	tokenExpiry     time.Time
	tokenMu         sync.Mutex
	// Rate limiting: queue + worker
	requestQueue    chan stkPayload
}

// tokenResponse is the OAuth client_credentials token response
type tokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"` // seconds
	TokenType   string `json:"token_type"`
}

// NewClient creates a new Kopo Kopo payment client with rate-limited worker.
// Use KOPOKOPO_CLIENT_ID + KOPOKOPO_CLIENT_SECRET (OAuth) or KOPOKOPO_ACCESS_TOKEN (manual token).
// The worker ensures we never exceed 10 requests per 20 seconds (using 2.1s interval = safe margin).
func NewClient() (*Client, error) {
	cfg := config.Get()
	c := &Client{
		baseURL:       cfg.KopoKopoBaseURL,
		webhookSecret: cfg.KopoKopoWebhookSecret,
		tillNumber:    cfg.KopoKopoTillNumber,
		callbackURL:   cfg.KopoKopoCallbackURL,
		clientID:      cfg.KopoKopoClientID,
		clientSecret:  cfg.KopoKopoClientSecret,
		accessToken:   cfg.KopoKopoAccessToken,
		requestQueue:  make(chan stkPayload, 100), // Buffer 100 requests
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
	
	// Start background worker
	go c.processQueue()
	
	return c, nil
}

// getAccessToken returns a valid Bearer token, fetching via OAuth if needed (client credentials).
func (c *Client) getAccessToken(ctx context.Context) (string, error) {
	// If we have a static token (e.g. from env), use it
	c.tokenMu.Lock()
	staticToken := c.accessToken
	c.tokenMu.Unlock()
	if staticToken != "" {
		return staticToken, nil
	}
	// OAuth: fetch/cache token
	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()
	// Refresh if expired or within 5 minutes of expiry
	if time.Now().Add(5 * time.Minute).Before(c.tokenExpiry) && c.accessToken != "" {
		return c.accessToken, nil
	}
	token, expiresIn, err := c.fetchOAuthToken(ctx)
	if err != nil {
		return "", err
	}
	c.accessToken = token
	c.tokenExpiry = time.Now().Add(time.Duration(expiresIn) * time.Second)
	return c.accessToken, nil
}

func (c *Client) fetchOAuthToken(ctx context.Context) (accessToken string, expiresIn int, err error) {
	authURL := strings.TrimSuffix(c.baseURL, "/") + "/oauth/token"
	form := url.Values{}
	form.Set("client_id", c.clientID)
	form.Set("client_secret", c.clientSecret)
	form.Set("grant_type", "client_credentials")
	req, err := http.NewRequestWithContext(ctx, "POST", authURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", 0, fmt.Errorf("create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "destination-cocktails/1.0")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", 0, fmt.Errorf("read token response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", 0, fmt.Errorf("oauth token error: status %d, body: %s", resp.StatusCode, string(body))
	}
	var tr tokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return "", 0, fmt.Errorf("parse token response: %w", err)
	}
	if tr.ExpiresIn <= 0 {
		tr.ExpiresIn = 3600
	}
	return tr.AccessToken, tr.ExpiresIn, nil
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

// InitiateSTKPush queues an M-Pesa STK Push request for async processing.
// Returns nil if successfully queued, error if queue is full.
func (c *Client) InitiateSTKPush(ctx context.Context, orderID string, phone string, amount float64) error {
	payload := stkPayload{
		orderID: orderID,
		phone:   phone,
		amount:  amount,
	}
	
	// Non-blocking send: return error if queue is full
	select {
	case c.requestQueue <- payload:
		return nil
	default:
		return errors.New("payment system busy, please try again")
	}
}

// processQueue is the background worker that processes queued STK push requests
// with rate limiting (2.1 seconds per request = ~28/min, well below 10/20s limit).
func (c *Client) processQueue() {
	ticker := time.NewTicker(2100 * time.Millisecond) // 2.1 seconds
	defer ticker.Stop()
	
	for {
		<-ticker.C // Wait for tick
		
		// Try to get next item from queue (non-blocking)
		select {
		case payload := <-c.requestQueue:
			// Process this STK push request
			ctx := context.Background()
			if err := c.sendSTKPush(ctx, payload.orderID, payload.phone, payload.amount); err != nil {
				slog.Error("STK push failed in worker",
					"order_id", payload.orderID,
					"error", err.Error())
			} else {
				slog.Info("STK push sent successfully",
					"order_id", payload.orderID)
			}
		default:
			// Queue is empty, continue waiting
		}
	}
}

// sendSTKPush sends an M-Pesa STK Push request to Kopo Kopo API (internal worker method).
func (c *Client) sendSTKPush(ctx context.Context, orderID string, phone string, amount float64) error {
	// Sanitize phone number: remove leading 0, ensure +254 format
	phone = sanitizePhone(phone)

	// Format amount as string (Kopo Kopo expects string)
	amountStr := fmt.Sprintf("%.0f", amount)

	// Build request payload (Kopo Kopo incoming_payments format)
	payload := STKPushRequest{
		PaymentChannel: "M-PESA STK Push",
		TillNumber:     c.tillNumber,
	}
	payload.Subscriber.PhoneNumber = phone
	payload.Amount.Currency = "KES"
	payload.Amount.Value = amountStr
	payload.Metadata.OrderID = orderID
	payload.Links.CallbackURL = c.callbackURL

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal STK push request: %w", err)
	}

	token, err := c.getAccessToken(ctx)
	if err != nil {
		return fmt.Errorf("get access token: %w", err)
	}

	// Make API request (correct Kopo Kopo endpoint)
	apiURL := fmt.Sprintf("%s/api/v1/incoming_payments", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send STK push request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("kopokopo API error: status %d, body: %s", resp.StatusCode, string(body))
	}

	var stkResponse STKPushResponse
	if err := json.Unmarshal(body, &stkResponse); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	slog.Info("Kopo Kopo STK response", "reference", stkResponse.Reference, "status", stkResponse.Status)
	return nil
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

	mac := hmac.New(sha256.New, []byte(c.webhookSecret))
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
	phone = strings.TrimPrefix(phone, "+")

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
