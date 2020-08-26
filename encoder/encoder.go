package encoder

type Encoder interface {
	Encode(interface{}) ([]byte, error)
	Decode(interface{}, interface{}) error
	DecodeData(interface{}) (Data, error)
	String() string
}

type Data map[string]interface{}
