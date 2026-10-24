package gallery

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
)

type artistChoice struct {
	Type       string   `json:"type"`
	Choice     string   `json:"choice"`
	Confidence *float64 `json:"confidence"`
}

func (f exampleFinder) artistChoices(ctx context.Context, payload any) (map[string]artistChoice, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, f.aiURL, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+f.apiKey)
	req.Header.Set("Content-Type", "application/json")
	var result struct {
		Answers map[string]artistChoice `json:"answers"`
	}
	err = f.decode(req, &result)
	return result.Answers, err
}
