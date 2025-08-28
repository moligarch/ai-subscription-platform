// File: .\internal\infra\adapters\ai\openai_adapter.go
package ai

import (
	"bufio"
	"context"
	"errors"
	"strings"

	openai "github.com/openai/openai-go/v2"
	"github.com/openai/openai-go/v2/option"
	"github.com/openai/openai-go/v2/packages/param"

	"github.com/pkoukk/tiktoken-go"

	"telegram-ai-subscription/internal/domain/ports/adapter"
)

var _ adapter.AIServiceAdapter = (*OpenAIAdapter)(nil)

type OpenAIAdapter struct {
	client       *openai.Client
	defaultModel string
	maxOut       int
}

func NewOpenAIAdapter(apiKey, baseURL, defaultModel string, maxOut int) (*OpenAIAdapter, error) {
	if apiKey == "" {
		return nil, errors.New("openai: empty api key")
	}
	opts := []option.RequestOption{option.WithAPIKey(apiKey)}
	if strings.TrimSpace(baseURL) != "" {
		opts = append(opts, option.WithBaseURL(strings.TrimRight(baseURL, "/")))
	}

	cl := openai.NewClient(opts...)
	return &OpenAIAdapter{
		client:       &cl,
		defaultModel: defaultModel,
		maxOut:       maxOut,
	}, nil
}

func (o *OpenAIAdapter) ListModels(ctx context.Context) ([]string, error) {
	// Keep minimal & resilient: return just the default if listing isn’t needed.
	if o.defaultModel != "" {
		return []string{o.defaultModel}, nil
	}
	return []string{"gpt-4o-mini"}, nil
}

func (o *OpenAIAdapter) GetModelInfo(model string) (adapter.ModelInfo, error) {
	return adapter.ModelInfo{Name: modelOrDefault(model, o.defaultModel)}, nil
}

// CountTokens best-effort using tiktoken-go.
// NOTE: OpenAI can change tokenization; this is only for pre-checks.
func (o *OpenAIAdapter) CountTokens(ctx context.Context, model string, messages []adapter.Message) (int, error) {
	enc, err := tiktoken.EncodingForModel(modelOrDefault(model, o.defaultModel))
	if err != nil {
		enc, err = tiktoken.GetEncoding("cl100k_base")
		if err != nil {
			return 0, err
		}
	}
	total := 0
	for _, m := range messages {
		total += len(enc.Encode(m.Content, nil, nil))
		// Optional: small overhead; comment out if you prefer raw-contents only.
		// total += len(enc.Encode(m.Role, nil, nil))
	}
	return total, nil
}

func (o *OpenAIAdapter) Chat(ctx context.Context, model string, messages []adapter.Message) (string, error) {
	reply, _, err := o.ChatWithUsage(ctx, model, messages)
	return reply, err
}

func (o *OpenAIAdapter) ChatWithUsage(ctx context.Context, model string, messages []adapter.Message) (string, adapter.Usage, error) {
	if len(messages) == 0 {
		return "", adapter.Usage{}, errors.New("openai: no messages")
	}
	msgs := toOpenAIMessages(messages)
	maxtkn := param.Opt[int64]{}
	maxtkn.Value = int64(o.maxOut)
	resp, err := o.client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model:               modelOrDefault(model, o.defaultModel),
		Messages:            msgs,
		MaxCompletionTokens: maxtkn,
	})
	if err != nil {
		return "", adapter.Usage{}, err
	}
	text := ""
	if len(resp.Choices) > 0 {
		text = resp.Choices[0].Message.Content
	}
	u := adapter.Usage{}
	if resp.Usage.JSON.TotalTokens.Valid() {
		u.TotalTokens = int(resp.Usage.TotalTokens)
		u.PromptTokens = int(resp.Usage.PromptTokens)
		u.CompletionTokens = int(resp.Usage.CompletionTokens)
	}
	return text, u, nil
}

// --- helpers ---

func toOpenAIMessages(msgs []adapter.Message) []openai.ChatCompletionMessageParamUnion {
	out := make([]openai.ChatCompletionMessageParamUnion, 0, len(msgs))
	for _, m := range msgs {
		switch strings.ToLower(m.Role) {
		case "assistant":
			out = append(out, openai.AssistantMessage(m.Content))
		case "system":
			out = append(out, openai.SystemMessage(m.Content))
		default:
			out = append(out, openai.UserMessage(m.Content))
		}
	}
	return out
}

// (Optional) generic word counter if you want a pure-length precheck:
// keeps here as a utility others can reuse.
func countWords(s string) int {
	sc := bufio.NewScanner(strings.NewReader(s))
	sc.Split(bufio.ScanWords)
	n := 0
	for sc.Scan() {
		n++
	}
	return n
}
