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
	value     reflect.Value
	origValue reflect.Value
	required  bool
	isList    bool
	source    string
	subFields []*field
	found     bool
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
	fields := l.parseStruct(ref)
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

func (l *Loader) parseStruct(ref reflect.Value) []*field {
	var list []*field
	t := ref.Type()
	numFields := ref.NumField()

	for i := 0; i < numFields; i++ {
		structField := t.Field(i)
		originalValue := ref.Field(i)
		typ := originalValue.Type()
		if structField.PkgPath != "" {
			continue
		}

		tag := structField.Tag.Get("config")
		value := reflect.New(typ).Elem()
		value.Set(originalValue)
		f := field{
			name:      structField.Name,
			key:       tag,
			value:     value,
			origValue: originalValue,
			found:     false,
		}
		tagCheck := func() {
			if tag == "-" {
				list = append(list, l.parseStruct(value)...)
			} else {
				f.subFields = l.parseStruct(value)
				l.parseTag(tag, &f)
				list = append(list, &f)
			}
		}
		switch typ.Kind() {
		case reflect.Struct:
			tagCheck()
			continue
		case reflect.Slice:
			if value.Type().Elem().Kind() == reflect.Struct {
				value = reflect.New(value.Type().Elem()).Elem()
				if tag == "-" {
					continue
				}
				f.isList = true
				f.value = f.origValue
				tagCheck()
				continue
			}
		case reflect.Ptr:
			if originalValue.Type().Elem().Kind() == reflect.Struct {
				if originalValue.IsNil() {
					value = reflect.New(originalValue.Type().Elem())
				}
				value = value.Elem()
				tagCheck()
				continue
			}
		default:
			if tag == "-" {
				continue
			}
		}
		l.parseTag(tag, &f)
		list = append(list, &f)

	}

	return list
}

func (l *Loader) parseTag(tag string, f *field) {
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
			break
		}
		if f.found {
			f.origValue.Set(f.value)
		}
		if f.source != "" && !backendFound {
			return fmt.Errorf("the backend: '%s' is not supported", f.source)
		}
		if f.required && !f.found {
			return fmt.Errorf("required key '%s' for field '%s' not found", f.key, f.name)
		}
		if len(f.subFields) != 0 {
			for _, subF := range f.subFields {
				if subF.required && !subF.found {
					return fmt.Errorf("required key '%s' for field '%s' not found", subF.key, subF.name)
				}
			}
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
		if f.isList {
			newDatas, err := c.Encoder.DecodeDataList(v)
			if err != nil {
				return err
			}
			val := reflect.MakeSlice(f.value.Type(), len(newDatas), len(newDatas))
			f.value.Set(val)
			for i, newData := range newDatas {
				for a, subF := range f.subFields {
					subF.value = reflect.New(subF.value.Type()).Elem()
					f.subFields[a].value = subF.value
					if err := l.getFieldData(subF, c, newData); err != nil {
						continue
					}
					f.value.Index(i).Field(a).Set(subF.value)
				}
			}
			f.found = true
			return nil
		}
		newData, err := c.Encoder.DecodeData(v)
		if err != nil {
			return err
		}
		for a, subF := range f.subFields {
			origValue := f.value
			kind := f.value.Type().Kind()
			if kind == reflect.Ptr && f.value.IsNil() {
				f.value = reflect.New(f.value.Type().Elem())
			}
			if err := l.getFieldData(subF, c, newData); err != nil {
				f.value = origValue
				continue
			}
			if kind == reflect.Struct {
				f.value.Field(a).Set(subF.value)
			} else {
				f.value.Elem().Field(a).Set(subF.value)
			}

		}
		f.found = true
		return nil
	}
	var to interface{}
	if f.value.CanAddr() {
		to = f.value.Addr().Interface()
	} else {
		to = f.value.Interface()
	}
	f.found = true
	return c.Encoder.Decode(v, to)
}
