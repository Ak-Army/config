package encoder

type Encoder interface {
	Encode(any) ([]byte, error)
	Decode(any, any) error
	DecodeData(any) (Data, error)
	DecodeDataList(any) ([]Data, error)
	// DecodeValue parses a single value written in the encoder's native format
	// into a plain Go value (map[string]interface{}, []interface{} or a scalar)
	// following the JSON data model, so the result marshals losslessly into the
	// backend-neutral json.RawMessage leaf representation the loader consumes.
	//
	// Unlike DecodeData it accepts any node — scalar, table or list — so a
	// backend that stores one document per entry (e.g. consul) can parse each
	// value in the configured format and still merge them into a path-built
	// tree. Formats without a bare-scalar document (TOML) return an error for
	// scalar input; use YAML or JSON for scalar leaves.
	DecodeValue(any) (any, error)
	String() string
}

type Data map[string]any
