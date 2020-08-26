package file

import (
	"errors"
	"io/ioutil"
	"os"
	"time"

	"github.com/Ak-Army/config/backend"
)

type file struct {
	opts          backend.Options
	watchInterval time.Duration
	path          string
}

func New(opts ...Option) backend.Backend {
	f := &file{
		opts:          backend.NewOptions(),
		watchInterval: 5 * time.Second,
	}
	f.opts.Name = "file"
	for _, o := range opts {
		o(f)
	}
	return f
}

func (f *file) Read() (*backend.Content, error) {
	if f.path == "" {
		return nil, errors.New("path not set")
	}
	fh, err := os.Open(f.path)
	if err != nil {
		return nil, err
	}
	defer fh.Close()

	b, err := ioutil.ReadAll(fh)
	if err != nil {
		return nil, err
	}
	info, err := fh.Stat()
	if err != nil {
		return nil, err
	}
	s := &backend.Content{
		Encoder:   f.opts.Encoder,
		Source:    f.String(),
		Timestamp: info.ModTime(),
		Data:      make(map[string]interface{}),
	}
	s.Data, err = f.opts.Encoder.DecodeData(b)
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (f *file) String() string {
	return f.opts.Name
}

func (f *file) Watcher() (backend.Watcher, error) {
	if !f.opts.Watcher {
		return nil, nil
	}
	if _, err := os.Stat(f.path); err != nil {
		return nil, err
	}
	return newWatcher(f)
}
