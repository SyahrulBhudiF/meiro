package youtube

import (
	"bytes"
	"encoding/json"
	"encoding/json/jsontext"
)

// YouTube's responses are order-sensitive: of several competing keys, IDs, or
// thumbnails, the first one a walk finds is the one the app uses. Decoding an
// object into a map loses that member order, so the parsers used to sort keys
// and walk them alphabetically, which both costs time and can pick a member the
// response never intended. jsonObject keeps its members in document order, and
// the parsers walk them the way the response listed them.

// jsonMember is one member of a JSON object, kept in document order.
type jsonMember struct {
	key   string
	value any
}

// jsonObject is a JSON object whose members keep their document order.
type jsonObject struct {
	members []jsonMember
}

// get returns the value stored under key, or nil when the object has none.
func (o *jsonObject) get(key string) any {
	if o == nil {
		return nil
	}
	for index := range o.members {
		if o.members[index].key == key {
			return o.members[index].value
		}
	}
	return nil
}

// has reports whether the object stores a value under key.
func (o *jsonObject) has(key string) bool {
	if o == nil {
		return false
	}
	for index := range o.members {
		if o.members[index].key == key {
			return true
		}
	}
	return false
}

// set stores value under key. A repeated name replaces the earlier member, as
// decoding into a map would keep the last one.
func (o *jsonObject) set(key string, value any) {
	for index := range o.members {
		if o.members[index].key == key {
			o.members[index].value = value
			return
		}
	}
	o.members = append(o.members, jsonMember{key: key, value: value})
}

// MarshalJSON encodes the object with its members in document order, so a
// parsed subtree can be sent back as a request payload.
func (o *jsonObject) MarshalJSON() ([]byte, error) {
	var buffer bytes.Buffer
	buffer.WriteByte('{')
	for index, member := range o.members {
		if index > 0 {
			buffer.WriteByte(',')
		}
		key, err := json.Marshal(member.key)
		if err != nil {
			return nil, err
		}
		buffer.Write(key)
		buffer.WriteByte(':')
		value, err := json.Marshal(member.value)
		if err != nil {
			return nil, err
		}
		buffer.Write(value)
	}
	buffer.WriteByte('}')
	return buffer.Bytes(), nil
}

// decodeResponse parses a response into the ordered form the extractors walk.
// It gives nil, which they read as empty, for what is not JSON.
func decodeResponse(raw json.RawMessage) any {
	decoder := jsontext.NewDecoder(bytes.NewReader(raw), jsontext.AllowDuplicateNames(true))
	value, err := readJSON(decoder)
	if err != nil {
		return nil
	}
	return value
}

// readJSON reads one JSON value: an object, an array, or a scalar.
func readJSON(decoder *jsontext.Decoder) (any, error) {
	switch decoder.PeekKind() {
	case jsontext.KindBeginObject:
		return readJSONObject(decoder)
	case jsontext.KindBeginArray:
		return readJSONArray(decoder)
	default:
		token, err := decoder.ReadToken()
		if err != nil {
			return nil, err
		}
		switch token.Kind() {
		case jsontext.KindNull:
			return nil, nil
		case jsontext.KindTrue:
			return true, nil
		case jsontext.KindFalse:
			return false, nil
		case jsontext.KindString:
			return token.String(), nil
		default:
			number, err := token.Float()
			if err != nil {
				return nil, err
			}
			return number, nil
		}
	}
}

func readJSONObject(decoder *jsontext.Decoder) (*jsonObject, error) {
	if _, err := decoder.ReadToken(); err != nil { // '{'
		return nil, err
	}
	object := &jsonObject{}
	for decoder.PeekKind() != jsontext.KindEndObject {
		name, err := decoder.ReadToken()
		if err != nil {
			return nil, err
		}
		key := name.String()
		value, err := readJSON(decoder)
		if err != nil {
			return nil, err
		}
		object.set(key, value)
	}
	if _, err := decoder.ReadToken(); err != nil { // '}'
		return nil, err
	}
	return object, nil
}

func readJSONArray(decoder *jsontext.Decoder) ([]any, error) {
	if _, err := decoder.ReadToken(); err != nil { // '['
		return nil, err
	}
	array := make([]any, 0)
	for decoder.PeekKind() != jsontext.KindEndArray {
		value, err := readJSON(decoder)
		if err != nil {
			return nil, err
		}
		array = append(array, value)
	}
	if _, err := decoder.ReadToken(); err != nil { // ']'
		return nil, err
	}
	return array, nil
}
