package json

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"

	jsoniter "github.com/json-iterator/go"
	"github.com/json-iterator/go/extra"

	"github.com/Ak-Army/config/encoder"
)

// bytesOf normalises the raw representations DecodeData/DecodeDataList accept.
func bytesOf(data interface{}) ([]byte, bool) {
	switch d := data.(type) {
	case json.RawMessage:
		return d, true
	case []byte:
		return d, true
	default:
		return nil, false
	}
}

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

// DecodeData splits a JSON object into its top-level members, keeping each
// value as a raw message for a later typed Decode. It streams the object with a
// single iterator so only one map is built and no intermediate
// map[string]json.RawMessage is boxed into interface values.
func (j jsonEncoder) DecodeData(data interface{}) (encoder.Data, error) {
	d, ok := bytesOf(data)
	if !ok {
		return nil, fmt.Errorf("unknown data type %s", reflect.TypeOf(data))
	}
	iter := jsoniter.ConfigDefault.BorrowIterator(d)
	defer jsoniter.ConfigDefault.ReturnIterator(iter)

	encoderData := make(encoder.Data)
	// A JSON null decodes to an empty object, mirroring Unmarshal into a map.
	if iter.WhatIsNext() == jsoniter.NilValue {
		iter.ReadNil()
		return encoderData, iterErr(iter)
	}
	// ReadMapCB instead of a ReadObject loop: ReadObject returns "" both for
	// end-of-object and for a genuine empty-string key, which would silently
	// drop the rest of the object.
	iter.ReadMapCB(func(it *jsoniter.Iterator, field string) bool {
		encoderData[field] = json.RawMessage(it.SkipAndReturnBytes())
		return true
	})
	if err := iterErr(iter); err != nil {
		return nil, err
	}
	return encoderData, nil
}

// DecodeDataList splits a JSON array of objects the same way DecodeData splits a
// single object.
func (j jsonEncoder) DecodeDataList(data interface{}) ([]encoder.Data, error) {
	d, ok := bytesOf(data)
	if !ok {
		return nil, fmt.Errorf("unknown data type %s", reflect.TypeOf(data))
	}
	iter := jsoniter.ConfigDefault.BorrowIterator(d)
	defer jsoniter.ConfigDefault.ReturnIterator(iter)

	var list []encoder.Data
	for iter.ReadArray() {
		item := make(encoder.Data)
		iter.ReadMapCB(func(it *jsoniter.Iterator, field string) bool {
			item[field] = json.RawMessage(it.SkipAndReturnBytes())
			return true
		})
		list = append(list, item)
	}
	if err := iterErr(iter); err != nil {
		return nil, err
	}
	return list, nil
}

// DecodeValue parses a single JSON value (object, array or scalar) into a plain
// Go value. Numbers are kept as json.Number so they survive a later re-marshal
// (e.g. by the consul backend) without float64 precision loss.
func (j jsonEncoder) DecodeValue(data interface{}) (interface{}, error) {
	d, ok := bytesOf(data)
	if !ok {
		return nil, fmt.Errorf("unknown data type %s", reflect.TypeOf(data))
	}
	dec := json.NewDecoder(bytes.NewReader(d))
	dec.UseNumber()
	var v interface{}
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	return v, nil
}

// iterErr returns the iterator error, treating a clean end of input as success.
func iterErr(iter *jsoniter.Iterator) error {
	if iter.Error != nil && iter.Error != io.EOF {
		return iter.Error
	}
	return nil
}

func (j jsonEncoder) String() string {
	return "json"
}
