package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOrder_FillClientFields(t *testing.T) {
	ref := "order_123"

	razorpay := &Order{Provider: ProviderRazorpay, ProviderCheckoutID: &ref}
	razorpay.FillClientFields()
	if assert.NotNil(t, razorpay.RazorpayOrderID) {
		assert.Equal(t, "order_123", *razorpay.RazorpayOrderID)
	}
	assert.NotSame(t, razorpay.ProviderCheckoutID, razorpay.RazorpayOrderID, "must be a copy, not an alias")

	dodo := &Order{Provider: ProviderDodo, ProviderCheckoutID: &ref}
	dodo.FillClientFields()
	assert.Nil(t, dodo.RazorpayOrderID, "a dodo session id is not a razorpay order id")

	empty := ""
	noRef := &Order{Provider: ProviderRazorpay, ProviderCheckoutID: &empty, RazorpayOrderID: &ref}
	noRef.FillClientFields()
	assert.Nil(t, noRef.RazorpayOrderID, "stale value is cleared")

	none := &Order{Provider: ProviderRazorpay}
	none.FillClientFields()
	assert.Nil(t, none.RazorpayOrderID)
}
