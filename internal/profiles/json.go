package profiles

import "winone/internal/configjson"

// Archive and committed settings share the same strict JSON boundary.
func strictJSON(data []byte, target any) error { return configjson.DecodeObject(data, target) }
func objectJSON(data []byte) error             { return configjson.ValidateObject(data) }
