package json

import (
	"encoding/json"
	"fmt"
	"reflect"

	jsoniter "github.com/json-iterator/go"
	"github.com/json-iterator/go/extra"

	"github.com/Ak-Army/config/encoder"
)

type jsonEncoder struct{}

func init() {
	extra.RegisterFuzzyDecoders()
}
func New() encoder.Encoder {
	return jsonEncoder{}
}

func (j jsonEncoder) Encode(v interface{}) ([]byte, error) {
	return jsoniter.Marshal(v)
}

func (j jsonEncoder) Decode(data interface{}, v interface{}) error {
	if d, ok := data.(json.RawMessage); ok {
		return jsoniter.Unmarshal(d, v)
	}
	if d, ok := data.([]byte); ok {
		return jsoniter.Unmarshal(d, v)
	}
	return fmt.Errorf("unknown data type %s", reflect.TypeOf(data))
}

func (j jsonEncoder) DecodeData(data interface{}) (encoder.Data, error) {
	ret := make(map[string]json.RawMessage)
	encoderData := make(encoder.Data)
	if d, ok := data.(json.RawMessage); ok {
		err := jsoniter.Unmarshal(d, &ret)
		if err != nil {
			return nil, err
		}
		for k, v := range ret {
			encoderData[k] = v
		}
		return encoderData, nil
	}
	if d, ok := data.([]byte); ok {
		err := jsoniter.Unmarshal(d, &ret)
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

func (j jsonEncoder) String() string {
	return "json"
}
