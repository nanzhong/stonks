package slack

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"

	"github.com/nanzhong/stonks/market"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
)

type httpErrorResponse struct {
	StatusCode int    `json:"status_code"`
	Error      string `json:"error"`
}

type httpError struct {
	err        error
	statusCode int
}

func newHTTPError(err error, status int) error {
	return &httpError{err: err, statusCode: status}
}

func newHTTPErrorWithMessage(err error, message string, status int) error {
	return &httpError{err: fmt.Errorf("%s: %w", message, err), statusCode: status}
}

func (e *httpError) Error() string {
	return fmt.Sprintf("%s: %d", e.err.Error(), e.statusCode)
}

func (e *httpError) Unwrap() error {
	return e.err
}

func (e *httpError) WriteResponse(w http.ResponseWriter) error {
	return json.NewEncoder(w).Encode(&httpErrorResponse{
		StatusCode: e.statusCode,
		Error:      e.err.Error(),
	})
}

var symbolRegexp = regexp.MustCompile("(?i)^([a-z]{3,4})[^a-z]*$")

type eventHandler struct {
	signingSecret string

	log           *log.Logger
	slackClient   *slack.Client
	marketBackend market.Backend
}

func NewEventHandler(slackClient *slack.Client, signingSecret string, marketBackend market.Backend) http.Handler {
	return &eventHandler{
		signingSecret: signingSecret,

		log:           log.New(os.Stderr, "", log.LstdFlags),
		slackClient:   slackClient,
		marketBackend: marketBackend,
	}
}

func (h *eventHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	event, err := h.validateRequest(r)
	if err != nil {
		switch err {
		case slack.ErrMissingHeaders, slack.ErrExpiredTimestamp:
			h.respondWithErr(w, r, newHTTPError(err, http.StatusBadRequest))
		default:
			h.respondWithErr(w, r, fmt.Errorf("validating request : %w", err))
		}
		return
	}

	switch event.Type {
	case slackevents.URLVerification:
		verificationEvent := event.Data.(*slackevents.EventsAPIURLVerificationEvent)
		_ = json.NewEncoder(w).Encode(&slackevents.ChallengeResponse{Challenge: verificationEvent.Challenge})
	case slackevents.CallbackEvent:
		if event.InnerEvent.Type != slackevents.AppMention {
			h.respondWithErr(w, r, newHTTPError(errors.New("unhandled event"), http.StatusNotImplemented))
			return
		}

		appMentionEvent := event.InnerEvent.Data.(*slackevents.AppMentionEvent)
		possibleSymbols := filterPossibleSymbols(strings.Fields(appMentionEvent.Text))
		var quotes []market.Quote
		if len(possibleSymbols) != 0 {
			quotes, err = h.marketBackend.Quote(possibleSymbols)
			if err != nil {
				h.respondWithErr(w, r, newHTTPErrorWithMessage(err, "getting quotes", http.StatusInternalServerError))
				return
			}
		}

		if len(quotes) == 0 {
			_, _, err = h.slackClient.PostMessageContext(
				r.Context(),
				appMentionEvent.Channel,
				slack.MsgOptionText("Sorry, I didn't find any valid market symbols in your message. :cry:", false),
				slack.MsgOptionTS(appMentionEvent.TimeStamp),
				slack.MsgOptionBroadcast(),
			)
		} else {
			messageBlocks := []slack.Block{
				slack.NewSectionBlock(
					slack.NewTextBlockObject(slack.MarkdownType, "Found the following quotes :chart_with_upwards_trend:", false, false),
					nil,
					nil,
				),
			}
			for i, quote := range quotes {
				messageBlocks = append(messageBlocks,
					slack.NewHeaderBlock(
						slack.NewTextBlockObject(slack.PlainTextType, fmt.Sprintf("%s (%s)", quote.ShortName, quote.Symbol), false, false)),
					slack.NewSectionBlock(
						slack.NewTextBlockObject(slack.MarkdownType, url.QueryEscape(fmt.Sprintf("*%.2f %+.2f (%+.2f%%)*", quote.RegularMarketPrice, quote.RegularMarketChange, quote.RegularMarketChangePercent)), false, true),
						nil,
						nil,
					),
				)
				if i != len(quotes)-1 {
					messageBlocks = append(messageBlocks, slack.NewDividerBlock())
				}
			}

			_, _, err = h.slackClient.PostMessageContext(
				r.Context(),
				appMentionEvent.Channel,
				slack.MsgOptionBlocks(messageBlocks...),
				slack.MsgOptionTS(appMentionEvent.TimeStamp),
				slack.MsgOptionBroadcast(),
			)
		}
		if err != nil {
			// Best effort attempt to send a message indicating failure
			_, _, _ = h.slackClient.PostMessageContext(
				r.Context(),
				appMentionEvent.Channel, slack.MsgOptionText("Sorry, I messed something up... Try again later :poop:", true),
				slack.MsgOptionTS(appMentionEvent.TimeStamp),
				slack.MsgOptionBroadcast(),
			)
			h.respondWithErr(w, r, newHTTPErrorWithMessage(err, "responding to mention", http.StatusInternalServerError))
			return
		}
		w.Write([]byte(`{}`))
	default:
		h.respondWithErr(w, r, newHTTPError(errors.New("unhandled event"), http.StatusNotImplemented))
	}
}

func (h *eventHandler) validateRequest(r *http.Request) (slackevents.EventsAPIEvent, error) {
	if r.Method != http.MethodPost {
		return slackevents.EventsAPIEvent{}, newHTTPError(fmt.Errorf("invalid method: %s", r.Method), http.StatusMethodNotAllowed)
	}

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return slackevents.EventsAPIEvent{}, fmt.Errorf("reading request body: %w", err)

	}
	defer r.Body.Close()

	if h.signingSecret == "" {
		// TODO log warning here.
	} else {
		sv, err := slack.NewSecretsVerifier(r.Header, h.signingSecret)
		if err != nil {
			return slackevents.EventsAPIEvent{}, fmt.Errorf("building slack secrets verifier: %w", err)
		}

		if _, err := sv.Write(body); err != nil {
			return slackevents.EventsAPIEvent{}, newHTTPErrorWithMessage(err, "verifying signature", http.StatusInternalServerError)
		}
		if err := sv.Ensure(); err != nil {
			return slackevents.EventsAPIEvent{}, newHTTPErrorWithMessage(err, "verifying signature", http.StatusUnauthorized)
		}
	}

	// NOTE prefer verifying signature over verification token.
	return slackevents.ParseEvent(json.RawMessage(body), slackevents.OptionNoVerifyToken())
}

func (h *eventHandler) respondWithErr(w http.ResponseWriter, r *http.Request, err error) error {
	h.log.Printf("%s %s - responding with error: %s", r.Method, r.URL.String(), err.Error())

	var he *httpError
	if errors.As(err, &he) {
		w.WriteHeader(he.statusCode)
		return he.WriteResponse(w)
	}

	w.WriteHeader(http.StatusInternalServerError)
	return json.NewEncoder(w).Encode(&httpErrorResponse{
		StatusCode: http.StatusInternalServerError,
		Error:      err.Error(),
	})
}

func filterPossibleSymbols(tokens []string) []string {
	var possible []string
	for _, token := range tokens {
		matches := symbolRegexp.FindStringSubmatch(token)
		if matches == nil {
			continue
		}
		possible = append(possible, matches[1])
	}
	return possible
}
