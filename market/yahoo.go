package market

import (
	"github.com/piquette/finance-go/quote"
)

type YahooBackend struct{}

func NewYahooBackend() *YahooBackend {
	return &YahooBackend{}
}

func (y *YahooBackend) Quote(symbols []string) ([]Quote, error) {
	var quotes []Quote

	iter := quote.List(symbols)
	for iter.Next() {
		quotes = append(quotes, *iter.Quote())
	}
	if err := iter.Err(); err != nil {
		return nil, err
	}

	return quotes, nil
}
