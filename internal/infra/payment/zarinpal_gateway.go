package payment

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
)

// ZarinPalDirectGateway implements PaymentGateway using direct HTTP calls.
type ZarinPalDirectGateway struct {
	merchantID string
	sandbox    bool
	baseURL    string
	client     *http.Client
}

// NewZarinPalDirectGateway creates a new direct ZarinPal gateway.
func NewZarinPalDirectGateway(merchantID string, sandbox bool) *ZarinPalDirectGateway {
	var baseURL string
	switch sandbox {
	case true:
		baseURL = "https://sandbox.zarinpal.com/pg/v4/payment"
	case false:
		baseURL = "https://payment.zarinpal.com/pg/v4/payment"
	}

	return &ZarinPalDirectGateway{
		merchantID: merchantID,
		sandbox:    sandbox,
		baseURL:    baseURL,
		client:     &http.Client{},
	}
}

// ZarinPalRequestResponse represents the response from the payment request API
type ZarinPalRequestResponse struct {
	Data struct {
		Code      int    `json:"code"`
		Message   string `json:"message"`
		Authority string `json:"authority"`
		FeeType   string `json:"fee_type"`
		Fee       int    `json:"fee"`
	} `json:"data"`
	Errors []interface{} `json:"errors"`
}

// ZarinPalVerifyResponse represents the response from the payment verification API
type ZarinPalVerifyResponse struct {
	Data struct {
		Code     int    `json:"code"`
		RefID    int64  `json:"ref_id"`
		CardPan  string `json:"card_pan"`
		CardHash string `json:"card_hash"`
		FeeType  string `json:"fee_type"`
		Fee      int    `json:"fee"`
	} `json:"data"`
	Errors []interface{} `json:"errors"`
}

// Request implements PaymentGateway.Request using direct HTTP calls.
func (g *ZarinPalDirectGateway) Request(ctx context.Context, amountIRR int64, callbackURL, description string, meta map[string]interface{}) (authority string, payURL string, err error) {
	requestData := map[string]interface{}{
		"merchant_id":  g.merchantID,
		"amount":       amountIRR,
		"callback_url": callbackURL,
		"description":  description,
	}

	if meta != nil {
		requestData["metadata"] = meta
	}

	jsonData, err := json.Marshal(requestData)
	if err != nil {
		return "", "", fmt.Errorf("failed to marshal request data: %w", err)
	}

	url := g.baseURL + "/request.json"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := g.client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", fmt.Errorf("failed to read response body: %w", err)
	}

	var response ZarinPalRequestResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return "", "", fmt.Errorf("failed to unmarshal response: %w, body: %s", err, string(body))
	}

	if response.Data.Code != 100 {
		return "", "", fmt.Errorf("zarinpal error: code %d, message: %s", response.Data.Code, response.Data.Message)
	}

	if len(response.Errors) > 0 {
		errorBytes, _ := json.Marshal(response.Errors)
		return "", "", fmt.Errorf("zarinpal errors: %s", string(errorBytes))
	}

	payURL = fmt.Sprintf("https://sandbox.zarinpal.com/pg/StartPay/%s", response.Data.Authority)
	if !g.sandbox {
		payURL = fmt.Sprintf("https://payment.zarinpal.com/pg/StartPay/%s", response.Data.Authority)
	}

	return response.Data.Authority, payURL, nil
}

// Verify implements PaymentGateway.Verify using direct HTTP calls.
func (g *ZarinPalDirectGateway) Verify(ctx context.Context, amountIRR int64, authority string) (refID string, ok bool, err error) {
	requestData := map[string]interface{}{
		"merchant_id": g.merchantID,
		"amount":      amountIRR,
		"authority":   authority,
	}

	jsonData, err := json.Marshal(requestData)
	if err != nil {
		return "", false, fmt.Errorf("failed to marshal request data: %w", err)
	}

	url := g.baseURL + "/verify.json"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", false, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := g.client.Do(req)
	if err != nil {
		return "", false, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", false, fmt.Errorf("failed to read response body: %w", err)
	}

	var response ZarinPalVerifyResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return "", false, fmt.Errorf("failed to unmarshal response: %w, body: %s", err, string(body))
	}

	if response.Data.Code != 100 && response.Data.Code != 101 {
		return "", false, fmt.Errorf("zarinpal error: code %d", response.Data.Code)
	}

	if len(response.Errors) > 0 {
		errorBytes, _ := json.Marshal(response.Errors)
		return "", false, fmt.Errorf("zarinpal errors: %s", string(errorBytes))
	}

	return strconv.FormatInt(response.Data.RefID, 10), true, nil
}
