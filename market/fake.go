package market

type FakeBackend struct {
	BaseQuote Quote
}

func (f *FakeBackend) Quote(symbols []string) ([]Quote, error) {
	var quotes []Quote
	for _, s := range symbols {
		quote := f.BaseQuote
		quote.Symbol = s
		quotes = append(quotes, quote)
	}
	return quotes, nil
}
