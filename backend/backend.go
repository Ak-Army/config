package backend

import (
	"time"

	"github.com/Ak-Army/config/encoder"
)

type Backend interface {
	Read() (*Content, error)
	Watcher() (Watcher, error)
	String() string
}

type Content struct {
	Data      encoder.Data
	Encoder   encoder.Encoder
	Source    string
	Timestamp time.Time
}

type Watcher interface {
	Watch() <-chan *Content
	Stop()
}
