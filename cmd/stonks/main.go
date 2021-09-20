package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nanzhong/stonks/market"
	"github.com/nanzhong/stonks/slack"
	slackgo "github.com/slack-go/slack"
)

var (
	addr               string
	slackBotToken      string
	slackSigningSecret string
)

func main() {
	flag.StringVar(&addr, "addr", envOrString("ADDR", "0.0.0.0:8080"), "Address to listen on.")
	flag.StringVar(&slackBotToken, "slack-bot-token", envOrString("SLACK_BOT_TOKEN", ""), "Slack token to use.")
	flag.StringVar(&slackSigningSecret, "slack-signing-secret", envOrString("SLACK_SIGNING_SECRET", ""), "Slack signing secret for requests events.")

	slackEventHandler := slack.NewEventHandler(
		slackgo.New(slackBotToken, slackgo.OptionDebug(true)),
		slackSigningSecret,
		market.NewYahooBackend(),
	)
	mux := http.NewServeMux()
	mux.Handle("/slack/event", slackEventHandler)

	server := http.Server{
		Addr:    addr,
		Handler: mux,
	}

	go func() {
		log.Println("Staring slack http server...")
		if err := server.ListenAndServe(); err != nil {
			log.Printf("Slack http server listen failed: %s", err.Error())
		}
	}()

	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM, syscall.SIGSTOP)

	<-done
	log.Printf("Got signal: %s", done)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Printf("Slack http server shutdown failed: %s", err.Error())
	}
}

func envOrString(envKey, defaultValue string) string {
	value, defined := os.LookupEnv(envKey)
	if defined {
		return value
	}
	return defaultValue
}
