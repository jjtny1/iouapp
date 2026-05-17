package config

import "os"

type Config struct {
	Port         string
	DBPath       string
	BaseURL      string
	AnthropicKey string
	OpenAIKey    string
	DevMode      bool
	MailProvider string
	MailFrom     string
	AWSRegion    string
	// AppleAppID is the iOS app's "<TeamID>.<bundleID>" identifier. When set,
	// the server publishes an Apple App Site Association file so the native
	// app can handle iouapp.ai Universal Links (magic-link sign-in).
	AppleAppID string
}

func Load() Config {
	return Config{
		Port:         env("PORT", "8080"),
		DBPath:       env("IOU_DB", "iou.db"),
		BaseURL:      env("IOU_BASE_URL", "http://localhost:8080"),
		AnthropicKey: os.Getenv("ANTHROPIC_API_KEY"),
		OpenAIKey:    os.Getenv("OPENAI_API_KEY"),
		DevMode:      boolEnv("IOU_DEV"),
		MailProvider: env("IOU_MAIL_PROVIDER", "log"),
		MailFrom:     os.Getenv("IOU_MAIL_FROM"),
		AWSRegion:    os.Getenv("AWS_REGION"),
		AppleAppID:   os.Getenv("IOU_APPLE_APP_ID"),
	}
}

func boolEnv(key string) bool {
	v := os.Getenv(key)
	return v == "1" || v == "true"
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
