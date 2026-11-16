// Package weatherapi is a client for WeatherAPI.com
// (https://www.weatherapi.com), whose free plan gives the current weather
// at a latitude and longitude.
package weatherapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"

	"go.uber.org/zap"
)

const (
	// DefaultURL is WeatherAPI's API base, used when Config.URL is empty.
	DefaultURL = "https://api.weatherapi.com/v1"

	// DefaultTimeout bounds a request when Config.HTTPClient is unset.
	DefaultTimeout = 10 * time.Second

	currentEndpoint = "/current.json"

	// maxErrorBody caps how much of an unrecognised error response is
	// quoted in the error.
	maxErrorBody = 512
)

// Config configures a Client. Only a key (Key or KeyFunc) is required. The
// mapstructure tags let a host decode it straight from viper or similar.
type Config struct {
	// URL is the API base. Empty means DefaultURL.
	URL string `json:"url" mapstructure:"url"`

	// Key is the API key.
	Key string `json:"key" mapstructure:"key"`

	// KeyFunc, when set, is called for every request in place of Key, so
	// a host can change the key (from a settings page, say) without
	// building a new client.
	KeyFunc func() string `json:"-" mapstructure:"-"`

	// HTTPClient sends the requests. Nil means a client with
	// DefaultTimeout.
	HTTPClient *http.Client `json:"-" mapstructure:"-"`
}

// APIError is an error WeatherAPI reported, with its HTTP status and its
// own error code (1006 is "no matching location", 2006 an invalid key;
// see https://www.weatherapi.com/docs/#intro-error-codes).
type APIError struct {
	StatusCode int    `json:"-"`
	Code       int    `json:"code"`
	Message    string `json:"message"`
}

// Error implements error.
func (e *APIError) Error() string {
	return fmt.Sprintf("weatherapi: status %d, code %d: %s", e.StatusCode, e.Code, e.Message)
}

// Client is a client for WeatherAPI. It is safe for concurrent use.
type Client struct {
	logger     *zap.Logger
	httpClient *http.Client
	url        string
	key        func() string
}

// NewClient creates a WeatherAPI client. A nil logger logs nothing.
func NewClient(logger *zap.Logger, cfg Config) (*Client, error) {
	if logger == nil {
		logger = zap.NewNop()
	}

	base := strings.TrimRight(cfg.URL, "/")
	if base == "" {
		base = DefaultURL
	}
	if _, err := url.Parse(base); err != nil {
		return nil, fmt.Errorf("weatherapi: invalid URL: %w", err)
	}

	key := cfg.KeyFunc
	if key == nil {
		static := cfg.Key
		key = func() string { return static }
	}

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: DefaultTimeout}
	}

	return &Client{
		logger:     logger.Named("weatherapi"),
		httpClient: httpClient,
		url:        base,
		key:        key,
	}, nil
}

// GetCurrentForecast returns the current weather at a latitude and
// longitude. An error WeatherAPI reports comes back as an *APIError.
func (c *Client) GetCurrentForecast(ctx context.Context, latitude, longitude float64) (*Payload, error) {
	if !validCoordinate(latitude, 90) || !validCoordinate(longitude, 180) {
		return nil, fmt.Errorf("weatherapi: invalid coordinates %v,%v", latitude, longitude)
	}

	key := c.key()
	if key == "" {
		return nil, errors.New("weatherapi: no API key set")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url+currentEndpoint, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("weatherapi: unable to build request: %w", err)
	}

	query := req.URL.Query()
	query.Set("q", fmt.Sprintf("%.8f,%.8f", latitude, longitude))
	query.Set("key", key)
	req.URL.RawQuery = query.Encode()

	res, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("weatherapi: request failed: %w", err)
	}
	defer func() { _ = res.Body.Close() }()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("weatherapi: unable to read response: %w", err)
	}

	// Without this check an error (a bad key, say) decoded into an empty
	// Payload and came back as current weather.
	if res.StatusCode != http.StatusOK {
		return nil, apiError(res.StatusCode, body)
	}

	var payload Payload
	if err := json.Unmarshal(body, &payload); err != nil {
		c.logger.Debug("unable to parse response", zap.ByteString("body", body))
		return nil, fmt.Errorf("weatherapi: unable to parse response: %w", err)
	}

	return &payload, nil
}

// apiError reads WeatherAPI's {"error": {"code", "message"}} body, falling
// back to the start of the body when it isn't in that shape.
func apiError(status int, body []byte) *APIError {
	var envelope struct {
		Error *APIError `json:"error"`
	}
	if json.Unmarshal(body, &envelope) == nil && envelope.Error != nil {
		envelope.Error.StatusCode = status
		return envelope.Error
	}

	msg := strings.TrimSpace(string(body))
	if len(msg) > maxErrorBody {
		msg = msg[:maxErrorBody] + "..."
	}
	return &APIError{StatusCode: status, Message: msg}
}

// validCoordinate reports whether v is a finite number within ±limit.
func validCoordinate(v, limit float64) bool {
	return !math.IsNaN(v) && v >= -limit && v <= limit
}
