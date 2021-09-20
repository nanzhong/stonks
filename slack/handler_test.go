package slack

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nanzhong/stonks/market"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slacktest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEventHandler_request_validation(t *testing.T) {
	handler := NewEventHandler(nil, "", nil)

	t.Run("invalid method", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)

		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})
}

func TestEventHandler_url_verification(t *testing.T) {
	handler := NewEventHandler(nil, "", nil)

	r := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{
    "token": "Jhj5dZrVaK7ZwHHjRyZWjbDl",
    "challenge": "3eZbrw1aBm2rZgRNFdxV2595E9CY3gmdALWMmHkvFXO7tYXAYM8P",
    "type": "url_verification"
}`))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, `{"Challenge":"3eZbrw1aBm2rZgRNFdxV2595E9CY3gmdALWMmHkvFXO7tYXAYM8P"}`, strings.TrimSpace(w.Body.String()))
}

func TestEventHandler_app_mention(t *testing.T) {
	s := slacktest.NewTestServer()
	go s.Start()
	t.Cleanup(func() {
		s.Stop()
	})

	handler := NewEventHandler(
		slack.New("token", slack.OptionAPIURL(s.GetAPIURL())),
		"",
		&market.FakeBackend{
			BaseQuote: market.Quote{
				RegularMarketPrice:         1.23,
				RegularMarketChange:        1.23,
				RegularMarketChangePercent: 1.23,
			},
		},
	)

	r := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{
    "token": "ZZZZZZWSxiZZZ2yIvs3peJ",
    "team_id": "T061EG9R6",
    "api_app_id": "A0MDYCDME",
    "event": {
        "type": "app_mention",
        "user": "W021FGA1Z",
        "text": "<@U0LAN0Z89> quote DOCN AAPL GOOG. :pray:",
        "ts": "1515449483.000108",
        "channel": "C0LAN2Q65",
        "event_ts": "1515449483000108"
    },
    "type": "event_callback",
    "event_id": "Ev0MDYHUEL",
    "event_time": 1515449483000108,
    "authed_users": [
        "U0LAN0Z89"
    ]
}`))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "{}", strings.TrimSpace(w.Body.String()))

	// TODO not sure what the best way of doing a simple comparison of the message
	// contents would be. Perhaps we actually should verify the entire message.
	require.Len(t, s.GetSeenOutboundMessages(), 1)
	assert.Contains(t, s.GetSeenOutboundMessages()[0], "DOCN")
	assert.Contains(t, s.GetSeenOutboundMessages()[0], "AAPL")
	assert.Contains(t, s.GetSeenOutboundMessages()[0], "GOOG")
}

func Test_filterPossibleSymbols(t *testing.T) {
	tests := []struct {
		name    string
		tokens  []string
		symbols []string
	}{
		{
			name:    "negative",
			tokens:  []string{"s", "toolong", "non-alpha", "numeric", ".prefix"},
			symbols: nil,
		},
		{
			name:    "positive - general 3 or 4 characters",
			tokens:  []string{"aaa", "bbbb"},
			symbols: []string{"aaa", "bbbb"},
		},
		{
			name:    "positive - trailing special characters",
			tokens:  []string{"aaa...", "bbbb."},
			symbols: []string{"aaa", "bbbb"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			symbols := filterPossibleSymbols(test.tokens)
			assert.Equal(t, test.symbols, symbols)
		})
	}
}
