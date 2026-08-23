package gen

import "encoding/json"

// JSONCodec keeps the checked-in fallback stubs usable when protoc/buf are unavailable.
// It is deliberately deterministic and only used by this learning project.
type JSONCodec struct{}

func (JSONCodec) Name() string                       { return "json" }
func (JSONCodec) Marshal(v any) ([]byte, error)      { return json.Marshal(v) }
func (JSONCodec) Unmarshal(data []byte, v any) error { return json.Unmarshal(data, v) }
