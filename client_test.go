package weatherapi

import (
	"context"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClient(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		c, err := NewClient(nil, Config{Key: "k"})
		require.NoError(t, err)
		assert.Equal(t, DefaultURL, c.url)
		assert.Equal(t, DefaultTimeout, c.httpClient.Timeout)
		assert.Equal(t, "k", c.key())
	})

	t.Run("trailing slash trimmed", func(t *testing.T) {
		c, err := NewClient(nil, Config{URL: "https://api.weatherapi.com/v1/", Key: "k"})
		require.NoError(t, err)
		assert.Equal(t, "https://api.weatherapi.com/v1", c.url)
	})

	t.Run("invalid URL", func(t *testing.T) {
		_, err := NewClient(nil, Config{URL: "http://bad\x7f", Key: "k"})
		require.Error(t, err)
	})

	t.Run("own HTTP client", func(t *testing.T) {
		hc := &http.Client{}
		c, err := NewClient(nil, Config{Key: "k", HTTPClient: hc})
		require.NoError(t, err)
		assert.Same(t, hc, c.httpClient)
	})
}

func TestGetCurrentForecast(t *testing.T) {
	tests := []struct {
		name     string
		lat, lon float64
		status   int
		body     string
		wantAPI  *APIError // an *APIError with these fields
		errText  string    // a substring of any other error
		wantTemp float64
	}{
		{
			name: "success", lat: -27.3842, lon: 153.1175, status: 200,
			body:     `{"location":{"name":"Brisbane"},"current":{"temp_c":25.5,"condition":{"text":"Sunny"}}}`,
			wantTemp: 25.5,
		},
		{name: "zero coordinates", lat: 0, lon: 0, status: 200, body: `{"current":{"temp_c":1}}`, wantTemp: 1},
		{name: "edges", lat: -90, lon: 180, status: 200, body: `{"current":{"temp_c":-40}}`, wantTemp: -40},
		{
			name: "no location", lat: 1, lon: 1, status: 400,
			body:    `{"error":{"code":1006,"message":"No matching location found."}}`,
			wantAPI: &APIError{StatusCode: 400, Code: 1006, Message: "No matching location found."},
		},
		{
			name: "invalid key", lat: 1, lon: 1, status: 401,
			body:    `{"error":{"code":2006,"message":"API key is invalid."}}`,
			wantAPI: &APIError{StatusCode: 401, Code: 2006, Message: "API key is invalid."},
		},
		{
			name: "unrecognised error body", lat: 1, lon: 1, status: 502, body: "<html>bad gateway</html>",
			wantAPI: &APIError{StatusCode: 502, Message: "<html>bad gateway</html>"},
		},
		{name: "invalid JSON", lat: 1, lon: 1, status: 200, body: "not json", errText: "unable to parse response"},
		{name: "latitude out of range", lat: 91, lon: 0, errText: "invalid coordinates"},
		{name: "longitude out of range", lat: 0, lon: -180.5, errText: "invalid coordinates"},
		{name: "NaN", lat: math.NaN(), lon: 0, errText: "invalid coordinates"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/v1/current.json", r.URL.Path)
				assert.Equal(t, "secret", r.URL.Query().Get("key"))
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer srv.Close()

			c, err := NewClient(nil, Config{URL: srv.URL + "/v1", Key: "secret"})
			require.NoError(t, err)

			payload, err := c.GetCurrentForecast(context.Background(), tt.lat, tt.lon)

			switch {
			case tt.wantAPI != nil:
				var apiErr *APIError
				require.True(t, errors.As(err, &apiErr), "want *APIError, got %v", err)
				assert.Equal(t, tt.wantAPI, apiErr)
				assert.Nil(t, payload)
			case tt.errText != "":
				require.ErrorContains(t, err, tt.errText)
				assert.Nil(t, payload)
			default:
				require.NoError(t, err)
				assert.InDelta(t, tt.wantTemp, payload.Current.TempCelsius, 0.001)
			}
		})
	}
}

func TestGetCurrentForecastQuery(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "-27.38420000,153.11750000", r.URL.Query().Get("q"))
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c, err := NewClient(nil, Config{URL: srv.URL, Key: "k"})
	require.NoError(t, err)

	_, err = c.GetCurrentForecast(context.Background(), -27.3842, 153.1175)
	require.NoError(t, err)
}

func TestKeyFuncReadPerRequest(t *testing.T) {
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.URL.Query().Get("key"))
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	var n atomic.Int32
	keys := []string{"first", "second"}
	c, err := NewClient(nil, Config{
		URL:     srv.URL,
		Key:     "ignored",
		KeyFunc: func() string { return keys[n.Add(1)-1] },
	})
	require.NoError(t, err)

	_, _ = c.GetCurrentForecast(context.Background(), 0, 0)
	_, _ = c.GetCurrentForecast(context.Background(), 0, 0)

	assert.Equal(t, []string{"first", "second"}, seen)
}

func TestGetCurrentForecastNoKey(t *testing.T) {
	c, err := NewClient(nil, Config{URL: "http://unused.test"})
	require.NoError(t, err)

	_, err = c.GetCurrentForecast(context.Background(), 0, 0)
	require.ErrorContains(t, err, "no API key set")
}

func TestGetCurrentForecastCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	c, err := NewClient(nil, Config{URL: srv.URL, Key: "k"})
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err = c.GetCurrentForecast(ctx, 0, 0)
	require.ErrorIs(t, err, context.DeadlineExceeded)
}
