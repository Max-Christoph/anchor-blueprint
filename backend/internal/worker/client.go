// Package worker ist der HTTP-Client zum Python-Worker. Der Worker ist ein
// optionaler, aber gekapselter Abhaengigkeitspunkt: Fehler werden als
// domaenenspezifischer Fehler zurueckgegeben, nicht als roher Transportfehler.
package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"
)

// ErrUnavailable signalisiert, dass der Worker nicht erreichbar ist.
var ErrUnavailable = errors.New("worker unavailable")

// CalcRequest ist der Payload fuer POST /process/calc.
type CalcRequest struct {
	Values    []float64 `json:"values"`
	Operation string    `json:"operation,omitempty"`
}

// CalcResult ist die Antwort des Workers.
type CalcResult struct {
	Operation  string  `json:"operation"`
	Count      int     `json:"count"`
	Sum        float64 `json:"sum"`
	Mean       float64 `json:"mean"`
	Median     float64 `json:"median"`
	Min        float64 `json:"min"`
	Max        float64 `json:"max"`
	StdDev     float64 `json:"stddev"`
	P90        float64 `json:"p90"`
	DurationMS float64 `json:"duration_ms"`
}

// Client ruft den Worker ueber internes HTTP auf.
type Client struct {
	baseURL string
	http    *http.Client
}

// New erzeugt einen Client mit eigenem Timeout und begrenzten Verbindungen.
func New(baseURL string, timeout time.Duration) *Client {
	transport := &http.Transport{
		DialContext:           (&net.Dialer{Timeout: 3 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		MaxIdleConns:          20,
		MaxIdleConnsPerHost:   10,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	return &Client{
		baseURL: baseURL,
		http: &http.Client{
			Timeout:   timeout,
			Transport: transport,
		},
	}
}

// Calc schickt die Werte an den Worker und dekodiert das Ergebnis.
func (c *Client) Calc(ctx context.Context, req CalcRequest) (CalcResult, error) {
	var result CalcResult

	if len(req.Values) == 0 {
		return result, errors.New("values must not be empty")
	}

	body, err := json.Marshal(req)
	if err != nil {
		return result, fmt.Errorf("marshal worker request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/process/calc", bytes.NewReader(body))
	if err != nil {
		return result, fmt.Errorf("build worker request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return result, fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return result, fmt.Errorf("%w: unexpected status %d", ErrUnavailable, resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return result, fmt.Errorf("decode worker response: %w", err)
	}
	return result, nil
}
