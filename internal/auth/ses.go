package auth

import (
	"context"
	"fmt"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	"github.com/aws/aws-sdk-go-v2/service/sesv2/types"
)

// SESSender is an EmailSender that delivers magic-link sign-in emails via
// Amazon SES (SESv2). It is the production counterpart to LogSender.
type SESSender struct {
	From   string
	client *sesv2.Client
}

// NewSESSender loads AWS configuration (the SDK default credential/region
// chain, with region optionally overridden) and returns an SESSender ready to
// deliver mail from the given verified From address.
func NewSESSender(ctx context.Context, from, region string) (EmailSender, error) {
	if from == "" {
		return nil, fmt.Errorf("ses: a From address is required")
	}
	var opts []func(*awsconfig.LoadOptions) error
	if region != "" {
		opts = append(opts, awsconfig.WithRegion(region))
	}
	cfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("ses: load AWS config: %w", err)
	}
	return &SESSender{From: from, client: sesv2.NewFromConfig(cfg)}, nil
}

// Send delivers a sign-in email containing the magic link, with both a
// plain-text and an HTML body.
func (s *SESSender) Send(email, link string) error {
	subject := "Your IOU sign-in link"
	text := fmt.Sprintf(
		"Sign in to IOU by opening this link:\n\n%s\n\n"+
			"This link expires in 15 minutes. If you didn't request it, you can ignore this email.",
		link)
	html := fmt.Sprintf(
		"<p>Sign in to IOU by opening this link:</p>"+
			"<p><a href=\"%s\">Sign in to IOU</a></p>"+
			"<p>This link expires in 15 minutes. If you didn't request it, you can ignore this email.</p>",
		link)

	charset := "UTF-8"
	_, err := s.client.SendEmail(context.Background(), &sesv2.SendEmailInput{
		FromEmailAddress: &s.From,
		Destination: &types.Destination{
			ToAddresses: []string{email},
		},
		Content: &types.EmailContent{
			Simple: &types.Message{
				Subject: &types.Content{Data: &subject, Charset: &charset},
				Body: &types.Body{
					Text: &types.Content{Data: &text, Charset: &charset},
					Html: &types.Content{Data: &html, Charset: &charset},
				},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("ses: send email to %s: %w", email, err)
	}
	return nil
}

// NewSender returns the EmailSender selected by provider. For "ses" it builds
// an SESSender; for any other value (including the empty string and "log") it
// returns a LogSender. An "ses" provider with an empty From fails fast.
func NewSender(ctx context.Context, provider, from, region string) (EmailSender, error) {
	if provider == "ses" {
		if from == "" {
			return nil, fmt.Errorf("ses mail provider requires IOU_MAIL_FROM to be set")
		}
		return NewSESSender(ctx, from, region)
	}
	return LogSender{}, nil
}
