package toml

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/BurntSushi/toml"

	"github.com/Ak-Army/config/encoder"
)

type tomlEncoder struct{}

func New() encoder.Encoder {
	return tomlEncoder{}
}

type innerToml struct {
	InnerToml []byte
}

func (d *innerToml) UnmarshalTOML(text interface{}) error {
	var err error
	d.InnerToml, err = json.Marshal(text)
	return err
}

func (t tomlEncoder) Encode(v interface{}) ([]byte, error) {
	b := bytes.NewBuffer(nil)
	defer b.Reset()

	err := toml.NewEncoder(b).Encode(v)
	if err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func (t tomlEncoder) Decode(data interface{}, v interface{}) error {
	if d, ok := data.(innerToml); ok {
		return json.Unmarshal(d.InnerToml, v)
	}
	if d, ok := data.(json.RawMessage); ok {
		return json.Unmarshal(d, v)
	}
	return fmt.Errorf("unknown data type %s", reflect.TypeOf(data))
}

func (t tomlEncoder) DecodeData(data interface{}) (encoder.Data, error) {
	encoderData := make(encoder.Data)
	if d, ok := data.([]byte); ok {
		ret := make(map[string]innerToml)
		err := toml.Unmarshal(d, &ret)
		if err != nil {
			return nil, err
		}
		for k, v := range ret {
			encoderData[k] = v
		}
		return encoderData, nil
	}
	if d, ok := data.(innerToml); ok {
		ret := make(map[string]json.RawMessage)
		err := json.Unmarshal(d.InnerToml, &ret)
		if err != nil {
			return nil, err
		}
		for k, v := range ret {
			encoderData[k] = v
		}
		return encoderData, nil
	}
	return nil, fmt.Errorf("unknown data type %s", reflect.TypeOf(data))
}

func (t tomlEncoder) String() string {
	return "toml"
}
