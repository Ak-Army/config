package config

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"sync"

	"github.com/juju/errors"

	"github.com/Ak-Army/config/backend"
	"github.com/Ak-Army/config/encoder"
)

type Loader struct {
	sync.Mutex
	ctx            context.Context
	backend        []backend.Backend
	backendWatcher []Config
	maps           map[backend.Backend]*backend.Content
}

type field struct {
	name      string
	key       string
	value     *reflect.Value
	required  bool
	source    string
	subFields []*field
}

func NewLoader(ctx context.Context, sources ...backend.Backend) (*Loader, error) {
	l := &Loader{
		backend: sources,
		ctx:     ctx,
		maps:    make(map[backend.Backend]*backend.Content),
	}
	for _, s := range l.backend {
		if err := l.syncSource(s); err != nil {
			return nil, err
		}
	}
	return l, nil
}

func (l *Loader) AddSource(sources ...backend.Backend) error {
	var gerr []string
	for _, s := range sources {
		if err := l.syncSource(s); err != nil {
			gerr = append(gerr, err.Error())
			continue
		}
		l.backend = append(l.backend, s)
	}
	if len(gerr) > 0 {
		return fmt.Errorf("source loading errors: %s", strings.Join(gerr, "\n"))
	}
	return nil
}

func (l *Loader) Load(c Config) error {
	l.backendWatcher = append(l.backendWatcher, c)
	to := c.NewSnapshot()
	ref := reflect.ValueOf(to)

	if !ref.IsValid() || ref.Kind() != reflect.Ptr || ref.Elem().Kind() != reflect.Struct {
		return errors.New("provided target must be a pointer to struct")
	}
	l.load(c)
	return nil
}

func (l *Loader) load(c Config) {
	to := c.NewSnapshot()
	ref := reflect.ValueOf(to).Elem()
	fields := l.parseStruct(&ref)

	err := l.resolve(fields)
	c.SetSnapshot(to, err)
}

func (l *Loader) syncSource(s backend.Backend) error {
	c, err := s.Read()
	if err != nil {
		return err
	}
	l.Lock()
	defer l.Unlock()
	l.maps[s] = c
	return l.watch(s)
}

func (l *Loader) watch(s backend.Backend) error {
	w, err := s.Watcher()
	if err != nil {
		return err
	}
	if w == nil {
		return nil
	}
	ch := w.Watch()
	go func() {
		for {
			select {
			case <-l.ctx.Done():
				return
			case content := <-ch:
				l.Lock()
				l.maps[s] = content
				for _, config := range l.backendWatcher {
					l.load(config)
				}
				l.Unlock()
			}
		}
	}()
	return nil
}

func (l *Loader) parseStruct(ref *reflect.Value) []*field {
	var list []*field
	t := ref.Type()
	numFields := ref.NumField()
	for i := 0; i < numFields; i++ {
		structField := t.Field(i)
		value := ref.Field(i)
		typ := value.Type()
		if structField.PkgPath != "" {
			continue
		}
		tag := structField.Tag.Get("config")
		f := field{
			name:  structField.Name,
			key:   tag,
			value: &value,
		}
		tagCheck := func() {
			if tag == "-" || tag == "" {
				list = append(list, l.parseStruct(&value)...)
			} else {
				f.subFields = l.parseStruct(&value)
				list = append(list, &f)
			}
		}
		switch typ.Kind() {
		case reflect.Struct:
			tagCheck()
			continue
		case reflect.Ptr:
			if value.Type().Elem().Kind() == reflect.Struct && !value.IsNil() {
				value = value.Elem()
				tagCheck()
				continue
			}
		default:
			if tag == "-" || tag == "" {
				continue
			}
		}
		l.parseTag(tag, f)
		list = append(list, &f)
	}
	return list
}

func (l *Loader) parseTag(tag string, f field) {
	if idx := strings.Index(tag, ","); idx != -1 {
		f.key = tag[:idx]
		opts := strings.Split(tag[idx+1:], ",")

		for _, opt := range opts {
			if opt == "required" {
				f.required = true
			}
			if strings.HasPrefix(opt, "backend=") {
				f.source = opt[len("backend="):]
			}
		}
	}
}

func (l *Loader) resolve(fields []*field) error {
	var gerr []string
	for _, f := range fields {
		var found bool
		var backendFound bool
		for s, data := range l.maps {
			if f.source != "" && f.source != s.String() {
				continue
			}
			backendFound = true
			if err := l.getFieldData(f, data, data.Data); err != nil {
				if !errors.IsNotFound(err) {
					gerr = append(gerr, err.Error())
				}
				continue
			}
			found = true
			break
		}

		if f.source != "" && !backendFound {
			return fmt.Errorf("the backend: '%s' is not supported", f.source)
		}
		if f.required && !found {
			return fmt.Errorf("required key '%s' for field '%s' not found", f.key, f.name)
		}
	}
	if len(gerr) > 0 {
		return fmt.Errorf("data loading errors: %s", strings.Join(gerr, "\n"))
	}
	return nil
}

func (l *Loader) getFieldData(f *field, c *backend.Content, data encoder.Data) error {
	v, found := data[f.key]
	if !found {
		return errors.NotFoundf("data %s", f.key)
	}
	if len(f.subFields) != 0 {
		newData, err := c.Encoder.DecodeData(v)
		if err != nil {
			return err
		}
		for _, subF := range f.subFields {
			if err := l.getFieldData(subF, c, newData); err != nil {
				return err
			}
		}
		return nil
	}
	to := f.value.Addr().Interface()
	return c.Encoder.Decode(v, to)
}
