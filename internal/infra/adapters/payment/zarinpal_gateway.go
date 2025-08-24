// File: internal/infra/adapters/payment/zarinpal_gateway.go
package payment

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"telegram-ai-subscription/internal/domain/ports/adapter"
)

var _ adapter.PaymentGateway = (*ZarinPalGateway)(nil)

// ZarinPalGateway implements adapter.PaymentGateway using REST v4 for request/verify
// and GraphQL v4 for refunds.
type ZarinPalGateway struct {
	merchantID      string
	callback        string
	sandbox         bool
	client          *http.Client
	accessToken     string // OAuth2 access token (GraphQL)
	graphqlEndpoint string // e.g. https://api.zarinpal.com/api/v4/graphql
}

// NewZarinPalGateway matches existing callsites/signature.
func NewZarinPalGateway(merchantID, callbackURL string, sandbox bool) (*ZarinPalGateway, error) {
	if merchantID == "" {
		return nil, errors.New("merchant id empty")
	}
	if _, err := url.Parse(callbackURL); err != nil {
		return nil, fmt.Errorf("invalid callback url: %w", err)
	}
	gp := &ZarinPalGateway{
		merchantID:      merchantID,
		callback:        callbackURL,
		sandbox:         sandbox,
		client:          &http.Client{Timeout: 15 * time.Second},
		graphqlEndpoint: "https://api.zarinpal.com/api/v4/graphql",
	}
	return gp, nil
}

// SetRefundAuth optionally configures OAuth and GraphQL endpoint for refunds.
func (z *ZarinPalGateway) SetRefundAuth(accessToken, graphqlEndpoint string) {
	z.accessToken = accessToken
	if graphqlEndpoint != "" {
		z.graphqlEndpoint = graphqlEndpoint
	}
}

func (z *ZarinPalGateway) Name() string { return "zarinpal" }

func (z *ZarinPalGateway) endpoint(path string) string {
	base := "https://api.zarinpal.com/pg/v4"
	if z.sandbox {
		base = "https://sandbox.zarinpal.com/pg/v4"
	}
	return base + path
}

// RequestPayment calls ZarinPal /payment/request.json and returns (authority, payURL).
func (z *ZarinPalGateway) RequestPayment(ctx context.Context, amountIRR int64, description, callbackURL string, meta map[string]interface{}) (string, string, error) {
	if callbackURL == "" {
		callbackURL = z.callback
	}
	payload := map[string]any{
		"merchant_id":  z.merchantID,
		"amount":       amountIRR,
		"description":  description,
		"callback_url": callbackURL,
	}
	if meta != nil {
		payload["metadata"] = meta
	}
	b, _ := json.Marshal(payload)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, z.endpoint("/payment/request.json"), bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := z.client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	var out struct {
		Data struct {
			Authority string `json:"authority"`
			Code      int    `json:"code"`
		} `json:"data"`
		Errors any `json:"errors"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", "", err
	}
	if out.Data.Code != 100 || out.Data.Authority == "" {
		return "", "", errors.New("zarinpal request failed")
	}
	payURL := fmt.Sprintf("https://www.zarinpal.com/pg/StartPay/%s", out.Data.Authority)
	if z.sandbox {
		payURL = fmt.Sprintf("https://sandbox.zarinpal.com/pg/StartPay/%s", out.Data.Authority)
	}
	return out.Data.Authority, payURL, nil
}

// VerifyPayment calls /payment/verify.json and returns provider refID on success.
func (z *ZarinPalGateway) VerifyPayment(ctx context.Context, authority string, expectedAmount int64) (string, error) {
	payload := map[string]any{
		"merchant_id": z.merchantID,
		"amount":      expectedAmount,
		"authority":   authority,
	}
	b, _ := json.Marshal(payload)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, z.endpoint("/payment/verify.json"), bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := z.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var out struct {
		Data struct {
			Code  int   `json:"code"`
			RefID int64 `json:"ref_id"`
		} `json:"data"`
		Errors any `json:"errors"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	// success code is 100 (101 means already verified). Treat both as ok if ref_id present.
	if (out.Data.Code != 100 && out.Data.Code != 101) || out.Data.RefID == 0 {
		return "", errors.New("zarinpal verify failed")
	}
	return fmt.Sprintf("%d", out.Data.RefID), nil
}

// RefundPayment issues a refund via GraphQL AddRefund mutation.
func (z *ZarinPalGateway) RefundPayment(ctx context.Context, sessionID string, amount int64, description string, method adapter.RefundMethod, reason adapter.RefundReason) (adapter.RefundResult, error) {
	if z.accessToken == "" {
		return adapter.RefundResult{}, errors.New("zarinpal refund requires access token: configure payment.zarinpal.access_token")
	}
	type gqlReq struct {
		Query     string                 `json:"query"`
		Variables map[string]interface{} `json:"variables"`
	}
	reqBody := gqlReq{
		Query: `mutation AddRefund($session_id: ID!, $amount: BigInteger!, $description: String, $method: InstantPayoutActionTypeEnum, $reason: RefundReasonEnum) {
  resource: AddRefund(session_id: $session_id, amount: $amount, description: $description, method: $method, reason: $reason) {
    id
    amount
    timeline { refund_amount refund_time refund_status }
  }
}`,
		Variables: map[string]interface{}{
			"session_id":  sessionID,
			"amount":      amount,
			"description": description,
			"method":      string(method),
			"reason":      string(reason),
		},
	}
	b, _ := json.Marshal(reqBody)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, z.graphqlEndpoint, bytes.NewReader(b))
	if err != nil {
		return adapter.RefundResult{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+z.accessToken)

	resp, err := z.client.Do(httpReq)
	if err != nil {
		return adapter.RefundResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return adapter.RefundResult{}, fmt.Errorf("refund http %d", resp.StatusCode)
	}
	var out struct {
		Data struct {
			Resource struct {
				ID       string `json:"id"`
				Amount   int64  `json:"amount"`
				Timeline struct {
					RefundAmount int64  `json:"refund_amount"`
					RefundTime   string `json:"refund_time"`
					RefundStatus string `json:"refund_status"`
				} `json:"timeline"`
			} `json:"resource"`
		} `json:"data"`
		Errors any `json:"errors"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return adapter.RefundResult{}, err
	}
	if out.Errors != nil {
		return adapter.RefundResult{}, fmt.Errorf("refund gql error: %v", out.Errors)
	}
	var rt time.Time
	if t := out.Data.Resource.Timeline.RefundTime; t != "" {
		if parsed, err := time.Parse(time.RFC3339, t); err == nil {
			rt = parsed
		}
	}
	return adapter.RefundResult{
		ID:           out.Data.Resource.ID,
		Status:       out.Data.Resource.Timeline.RefundStatus,
		RefundAmount: out.Data.Resource.Timeline.RefundAmount,
		RefundTime:   rt,
	}, nil
}
