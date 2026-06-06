package gotipp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client is a simple HTTP client for the GoTipp API.
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

func NewClient(baseURL, token string) *Client {
	return &Client{
		baseURL: baseURL,
		token:   token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Match represents a match from the API.
type Match struct {
	ID          int    `json:"id"`
	TeamA       string `json:"team_a"`
	TeamB       string `json:"team_b"`
	Start       string `json:"start"`
	MatchType   string `json:"match_type"`
	EventPhase  int    `json:"event_phase"`
	Finished    bool   `json:"finished"`
	AcceptsTipps bool  `json:"accepts_tipps"`
}

// Tipp represents an existing tipp from the API.
type Tipp struct {
	MatchID int    `json:"match_id"`
	TippA   int    `json:"tipp_a"`
	TippB   int    `json:"tipp_b"`
	Created string `json:"created"`
	Changed string `json:"changed"`
}

// TippRequest is the payload for a single tipp within a batch.
type TippRequest struct {
	MatchID int `json:"match_id"`
	TippA   int `json:"tipp_a"`
	TippB   int `json:"tipp_b"`
}

// tippsPayload is the request body for POST /api/v1/tipps.
type tippsPayload struct {
	Tipps []TippRequest `json:"tipps"`
}

// TippsResponse is the response from POST /api/v1/tipps.
type TippsResponse struct {
	Count   int `json:"count"`
	Results []struct {
		MatchID int    `json:"match_id"`
		Status  string `json:"status"`
	} `json:"results"`
}

// GetMatches fetches all matches for the active event.
func (c *Client) GetMatches(ctx context.Context) ([]Match, error) {
	var matches []Match
	if err := c.get(ctx, "/api/v1/matches", &matches); err != nil {
		return nil, err
	}
	return matches, nil
}

// GetTipps fetches the user's existing tipps.
func (c *Client) GetTipps(ctx context.Context) ([]Tipp, error) {
	var tipps []Tipp
	if err := c.get(ctx, "/api/v1/tipps", &tipps); err != nil {
		return nil, err
	}
	return tipps, nil
}

// PostTipps submits one or more tipps in a single batch request (max 200).
func (c *Client) PostTipps(ctx context.Context, tipps []TippRequest) (*TippsResponse, error) {
	body, err := json.Marshal(tippsPayload{Tipps: tipps})
	if err != nil {
		return nil, fmt.Errorf("marshal tipps: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/tipps", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("post tipps: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("post tipps: status %d, body: %s", resp.StatusCode, respBody)
	}

	var result TippsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode tipps response: %w", err)
	}

	return &result, nil
}

func (c *Client) get(ctx context.Context, path string, dest any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("GET %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GET %s: status %d, body: %s", path, resp.StatusCode, respBody)
	}

	return json.NewDecoder(resp.Body).Decode(dest)
}
