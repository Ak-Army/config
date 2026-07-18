package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/Ak-Army/config"
	"github.com/Ak-Army/config/backend/env"
	"github.com/Ak-Army/config/backend/file"
	"github.com/Ak-Army/config/crypto"
	"github.com/Ak-Army/config/crypto/aesgcm"
)

type Config struct {
	// APIKey is stored as an ENC(...) value in config.json and decrypted on
	// load; produce such values with cmd/configcrypt.
	APIKey               string        `config:"api-key,encrypted"`
	RecallCheckInterval  time.Duration `config:"recall-check-interval"`
	QueueThreshold       int           `config:"queue-threshold"`
	CallCheckInterval    time.Duration `config:"call-check-interval"`
	CallConsumer         time.Duration `config:"call-consumer"`
	CallRemoveInterval   time.Duration `config:"call-remove-interval"`
	FailedCallThreshold  int64         `config:"failed-call-threshold"`
	StatPublishInterval  time.Duration `config:"stat-publish-interval"`
	AutoRemoveInterval   time.Duration `config:"auto-remove-interval"`
	DialerProjectCheck   time.Duration `config:"dialer-project-check"`
	StatCrawlingInterval time.Duration `config:"stat-crawling-interval"`
	Amd2Config           *Amd2Config   `config:"amd2"`
}

type Amd2Config struct {
	Active              bool           `config:"active"`
	PhoneNumberPrefixes []string       `config:"phone-number-prefixes"`
	AppParams           *Amd2AppParams `config:"app-params"`
}

type Amd2AppParams struct {
	Record         int    `config:"record"`
	AnalyzedLength int64  `config:"analyzed_length"`
	Filepath       string `config:"filepath"`
}

// configHandler supplies the defaults and post-processing for Config; it
// implements config.Handler[Config].
type configHandler struct{}

// Default returns a freshly initialised Config holding the default values.
func (*Config) Default() *Config {
	return &Config{
		RecallCheckInterval: 30,
		QueueThreshold:      400,
		CallCheckInterval:   10,
		CallConsumer:        10,
		FailedCallThreshold: 10,
		CallRemoveInterval:  1,
		StatPublishInterval: 1,
		AutoRemoveInterval:  30,
		DialerProjectCheck:  60,
		Amd2Config: &Amd2Config{
			AppParams: &Amd2AppParams{},
		},
	}
}

// Set scales the raw numeric values into the durations the app expects.
func (*Config) Set(conf *Config) {
	conf.RecallCheckInterval *= time.Second
	conf.CallCheckInterval *= time.Second
	conf.CallConsumer *= time.Millisecond
	conf.CallRemoveInterval *= time.Second
	conf.StatPublishInterval *= time.Second
	conf.AutoRemoveInterval *= time.Second
	conf.DialerProjectCheck *= time.Second
	conf.StatCrawlingInterval *= time.Second
}

func main() {
	loader, err := config.NewLoader(context.Background(),
		env.New(env.WithDefaults("config/default")),
		file.New(file.WithPath("config/config.json")),
	)
	if err != nil {
		log.Fatal(err)
	}
	cr, err := crypto.New("config/config.keyring", func(key []byte) (crypto.Decrypter, error) {
		return aesgcm.New(key)
	})
	if err != nil {
		log.Fatal(err)
	}
	loader.SetCrypto(cr)
	c := config.NewStore[Config](&Config{})
	if err := config.Load(loader, c); err != nil {
		log.Fatal(err)
	}
	conf, err := c.Config()
	fmt.Printf("%+v, err: %s\n", conf, err)
}
