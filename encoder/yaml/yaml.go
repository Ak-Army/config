package yaml

import (
	"encoding/json"
	"fmt"
	"reflect"

	"gopkg.in/yaml.v3"

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

// DecodeData parses YAML with yaml.v3 (YAML 1.2: only true/false are booleans,
// so unquoted yes/no/on/off stay strings) and re-encodes each top-level value
// as JSON, matching the json.RawMessage representation Decode expects.
func (y yamlEncoder) DecodeData(data interface{}) (encoder.Data, error) {
	if d, ok := data.([]byte); ok {
		var yamlMap map[string]interface{}
		if err := yaml.Unmarshal(d, &yamlMap); err != nil {
			return nil, err
		}
		return toEncoderData(yamlMap)
	}
	if d, ok := data.(json.RawMessage); ok {
		yamlMap := make(map[string]json.RawMessage)
		if err := json.Unmarshal(d, &yamlMap); err != nil {
			return nil, err
		}
		encoderData := make(encoder.Data, len(yamlMap))
		for k, v := range yamlMap {
			encoderData[k] = v
		}
		return encoderData, nil
	}
	return nil, fmt.Errorf("unknown data type %s", reflect.TypeOf(data))
}

func (y yamlEncoder) DecodeDataList(data interface{}) ([]encoder.Data, error) {
	if d, ok := data.([]byte); ok {
		var yamlMaps []map[string]interface{}
		if err := yaml.Unmarshal(d, &yamlMaps); err != nil {
			return nil, err
		}
		encoderData := make([]encoder.Data, len(yamlMaps))
		for i, yamlMap := range yamlMaps {
			ed, err := toEncoderData(yamlMap)
			if err != nil {
				return nil, err
			}
			encoderData[i] = ed
		}
		return encoderData, nil
	}
	if d, ok := data.(json.RawMessage); ok {
		var yamlMaps []map[string]json.RawMessage
		if err := json.Unmarshal(d, &yamlMaps); err != nil {
			return nil, err
		}
		encoderData := make([]encoder.Data, len(yamlMaps))
		for i, yamlMap := range yamlMaps {
			encoderData[i] = make(encoder.Data, len(yamlMap))
			for k, v := range yamlMap {
				encoderData[i][k] = v
			}
		}
		return encoderData, nil
	}
	return nil, fmt.Errorf("unknown data type %s", reflect.TypeOf(data))
}

func (y yamlEncoder) String() string {
	return "yaml"
}

func toEncoderData(yamlMap map[string]interface{}) (encoder.Data, error) {
	encoderData := make(encoder.Data, len(yamlMap))
	for k, v := range yamlMap {
		raw, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		encoderData[k] = json.RawMessage(raw)
	}
	return encoderData, nil
}
