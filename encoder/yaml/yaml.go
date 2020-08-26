package yaml

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/ghodss/yaml"

	"github.com/Ak-Army/config/encoder"
)

type yamlEncoder struct{}

func New() encoder.Encoder {
	return yamlEncoder{}
}

func (y yamlEncoder) Encode(v interface{}) ([]byte, error) {
	return yaml.Marshal(v)
}

func (y yamlEncoder) Decode(data interface{}, v interface{}) error {
	if d, ok := data.(json.RawMessage); ok {
		return json.Unmarshal(d, v)
	}
	return fmt.Errorf("unknown data type %s", reflect.TypeOf(data))
}

func (y yamlEncoder) DecodeData(data interface{}) (encoder.Data, error) {
	yamlMap := make(map[string]json.RawMessage)
	encoderData := make(encoder.Data)
	if d, ok := data.([]byte); ok {
		err := yaml.Unmarshal(d, &yamlMap)
		if err != nil {
			return nil, err
		}
		for k, v := range yamlMap {
			encoderData[k] = v
		}
		return encoderData, nil
	}
	if d, ok := data.(json.RawMessage); ok {
		err := json.Unmarshal(d, &yamlMap)
		if err != nil {
			return nil, err
		}
		for k, v := range yamlMap {
			encoderData[k] = v
		}
		return encoderData, nil
	}
	return nil, fmt.Errorf("unknown data type %s", reflect.TypeOf(data))
}

func (y yamlEncoder) String() string {
	return "yaml"
}
