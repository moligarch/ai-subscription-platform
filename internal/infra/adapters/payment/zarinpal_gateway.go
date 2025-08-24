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

// ZarinPalGateway implements adapter.PaymentGateway using REST v4.
type ZarinPalGateway struct {
	merchantID string
	callback   string
	sandbox    bool
	client     *http.Client
}

func NewZarinPalGateway(merchantID, callbackURL string, sandbox bool) (*ZarinPalGateway, error) {
	if merchantID == "" {
		return nil, errors.New("merchant id empty")
	}
	if _, err := url.Parse(callbackURL); err != nil {
		return nil, fmt.Errorf("invalid callback url: %w", err)
	}
	return &ZarinPalGateway{
		merchantID: merchantID,
		callback:   callbackURL,
		sandbox:    sandbox,
		client:     &http.Client{Timeout: 15 * time.Second},
	}, nil
}

func (z *ZarinPalGateway) Name() string { return "zarinpal" }

func (z *ZarinPalGateway) endpoint(path string) string {
	base := "https://api.zarinpal.com/pg/v4"
	if z.sandbox {
		base = "https://sandbox.zarinpal.com/pg/v4"
	}
	return base + path
}

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
	if out.Data.Code != 100 || out.Data.RefID == 0 {
		return "", errors.New("zarinpal verify failed")
	}
	return fmt.Sprintf("%d", out.Data.RefID), nil
}
