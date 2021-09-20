package market

import (
	"github.com/piquette/finance-go"
)

// Quote represents a market quote for a symbol.
// TODO Since yahoo finance via piquette/finance-go is the only supported backend and it contains the fields we actually care about, just alias the type for now.
type Quote = finance.Quote

type Backend interface {
	Quote(symbols []string) ([]Quote, error)
}
