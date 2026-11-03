package application

import (
	"fmt"
	"strings"

	"github.com/saransh1220/blueprint-audio/internal/modules/payment/domain"
)

// GatewayResolver is what the payment service needs from the registry.
type GatewayResolver interface {
	// ForCurrency picks the gateway that charges in the given currency.
	ForCurrency(currency string) (domain.Gateway, error)
	// ByName returns the gateway recorded on an existing order.
	ByName(name string) (domain.Gateway, error)
}

// GatewayRegistry is the factory: it owns which gateway serves which currency.
type GatewayRegistry struct {
	byName     map[string]domain.Gateway
	byCurrency map[string]string
}

// NewGatewayRegistry registers the gateways and routes each currency to one
// of them, e.g. {"INR": "razorpay", "USD": "dodo"}.
func NewGatewayRegistry(currencyRoutes map[string]string, gateways ...domain.Gateway) (*GatewayRegistry, error) {
	r := &GatewayRegistry{
		byName:     make(map[string]domain.Gateway, len(gateways)),
		byCurrency: make(map[string]string, len(currencyRoutes)),
	}
	for _, g := range gateways {
		if _, dup := r.byName[g.Name()]; dup {
			return nil, fmt.Errorf("payment gateway %q registered twice", g.Name())
		}
		r.byName[g.Name()] = g
	}
	for currency, name := range currencyRoutes {
		if _, ok := r.byName[name]; !ok {
			return nil, fmt.Errorf("currency %s routes to unknown payment gateway %q", currency, name)
		}
		r.byCurrency[strings.ToUpper(currency)] = name
	}
	return r, nil
}

func (r *GatewayRegistry) ForCurrency(currency string) (domain.Gateway, error) {
	name, ok := r.byCurrency[strings.ToUpper(strings.TrimSpace(currency))]
	if !ok {
		return nil, fmt.Errorf("no payment gateway configured for currency %q", currency)
	}
	return r.byName[name], nil
}

func (r *GatewayRegistry) ByName(name string) (domain.Gateway, error) {
	g, ok := r.byName[name]
	if !ok {
		return nil, fmt.Errorf("unknown payment provider %q", name)
	}
	return g, nil
}
