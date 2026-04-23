package email

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuildVerificationEmail(t *testing.T) {
	msg := BuildVerificationEmail("test@example.com", "Saransh", "123456", "http://localhost:4200")

	assert.Equal(t, []string{"test@example.com"}, msg.To)
	assert.Equal(t, "Verify your Blueprint account", msg.Subject)
	assert.Contains(t, msg.Text, "123456")
	assert.Contains(t, msg.Text, "http://localhost:4200/verify-email")
	assert.Contains(t, msg.HTML, "BLUEPRINT")
	assert.Contains(t, msg.HTML, "Confirm your email and step into the studio.")
	assert.Contains(t, msg.HTML, "123456")
	assert.Contains(t, msg.HTML, "Verify Email")
	assert.Contains(t, msg.HTML, "http://localhost:4200/verify-email?code=123456&amp;email=test%40example.com")
}

func TestBuildPasswordResetEmail(t *testing.T) {
	msg := BuildPasswordResetEmail("test@example.com", "", "654321", "http://localhost:4200")

	assert.Equal(t, []string{"test@example.com"}, msg.To)
	assert.Equal(t, "Reset your Blueprint password", msg.Subject)
	assert.Contains(t, msg.Text, "Hi there,")
	assert.Contains(t, msg.Text, "654321")
	assert.Contains(t, msg.HTML, "Reset your password without missing a beat.")
	assert.Contains(t, msg.HTML, "Wasn&#39;t you?")
	assert.Contains(t, msg.HTML, "http://localhost:4200/reset-password?code=654321&amp;email=test%40example.com")
}

func TestBuildPaymentReceiptEmail(t *testing.T) {
	msg := BuildPaymentReceiptEmail(ReceiptData{
		BuyerName:     "Buyer",
		BuyerEmail:    "buyer@example.com",
		SpecTitle:     "Midnight Drive",
		LicenseType:   "Premium",
		AmountDisplay: "INR 1,999.00",
		OrderID:       "ord_123",
		PaymentID:     "pay_123",
		LicenseID:     "lic_123",
		SupportEmail:  "support@example.com",
	}, "http://localhost:4200")

	assert.Equal(t, []string{"buyer@example.com"}, msg.To)
	assert.Equal(t, "Your Blueprint purchase receipt", msg.Subject)
	assert.Contains(t, msg.Text, "Midnight Drive")
	assert.Contains(t, msg.Text, "support@example.com")
	assert.Contains(t, msg.HTML, "Purchase confirmed")
	assert.Contains(t, msg.HTML, "Open My Licenses")
	assert.Contains(t, msg.HTML, "support@example.com")
	assert.Contains(t, msg.HTML, "Midnight Drive")
	assert.Contains(t, msg.HTML, "ord_123")
	assert.Contains(t, msg.HTML, "http://localhost:4200/licenses")
}

func TestRenderTemplate_OmitsSupportBlockWhenEmailMissing(t *testing.T) {
	html, err := renderTemplate("payment-receipt.html", emailTemplateData{
		BrandName:  "BLUEPRINT",
		Title:      "Your new license is ready.",
		Greeting:   "Hi there,",
		NoticeBody: "Body",
	})

	assert.NoError(t, err)
	assert.NotContains(t, html, "Need help? Reach us at")
	assert.True(t, strings.Contains(html, "Blueprint transactional email"))
}
