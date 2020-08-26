package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/Ak-Army/config"
	"github.com/Ak-Army/config/backend/env"
	"github.com/Ak-Army/config/backend/file"
)

type Config struct {
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

type configStore struct {
	sync.Mutex
	config Config
	err    error
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

func (c *configStore) NewSnapshot() interface{} {
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

func (c *configStore) SetSnapshot(confInterface interface{}, err error) {
	c.Lock()
	defer c.Unlock()
	conf := confInterface.(*Config)

	conf.RecallCheckInterval *= time.Second
	conf.CallCheckInterval *= time.Second
	conf.CallConsumer *= time.Millisecond
	conf.CallRemoveInterval *= time.Second
	conf.StatPublishInterval *= time.Second
	conf.AutoRemoveInterval *= time.Second
	conf.DialerProjectCheck *= time.Second
	conf.StatCrawlingInterval *= time.Second
	c.config = *conf
	c.err = err
}

func (c *configStore) Config() (Config, error) {
	c.Lock()
	defer c.Unlock()
	return c.config, c.err
}

func main() {
	loader, err := config.NewLoader(context.Background(),
		env.New(env.WithDefaults("config/default")),
		file.New(file.WithPath("config/config.json")),
	)
	c := &configStore{}
	if err != nil {
		log.Fatal(err)
	}
	err = loader.Load(c)
	if err != nil {
		log.Fatal(err)
	}
	conf, err := c.Config()
	fmt.Printf("%+v, err: %s\n", conf, err)
}
