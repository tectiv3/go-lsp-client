package main

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestMarkedString_MarshalUnmarshalJSON(t *testing.T) {
	tests := []struct {
		data []byte
		want MarkedString
	}{{
		data: []byte(`{"language":"go","value":"foo"}`),
		want: MarkedString{Language: "go", Value: "foo", isRawString: false},
	}, {
		data: []byte(`{"language":"","value":"foo"}`),
		want: MarkedString{Language: "", Value: "foo", isRawString: false},
	}, {
		data: []byte(`"foo"`),
		want: MarkedString{Language: "", Value: "foo", isRawString: true},
	}}

	for _, test := range tests {
		var m MarkedString
		if err := json.Unmarshal(test.data, &m); err != nil {
			t.Errorf("json.Unmarshal error: %s", err)
			continue
		}
		if !reflect.DeepEqual(test.want, m) {
			t.Errorf("Unmarshaled %q, expected %+v, but got %+v", string(test.data), test.want, m)
			continue
		}

		marshaled, err := json.Marshal(m)
		if err != nil {
			t.Errorf("json.Marshal error: %s", err)
			continue
		}
		if string(marshaled) != string(test.data) {
			t.Errorf("Marshaled result expected %s, but got %s", string(test.data), string(marshaled))
		}
	}
}

func TestHover(t *testing.T) {
	tests := []struct {
		data          []byte
		want          Hover
		skipUnmarshal bool
		skipMarshal   bool
	}{{
		data: []byte(`{"contents":[{"language":"go","value":"foo"}]}`),
		want: Hover{Contents: []MarkedString{{Language: "go", Value: "foo", isRawString: false}}},
	}, {
		data:          []byte(`{"contents":[]}`),
		want:          Hover{Contents: nil},
		skipUnmarshal: true, // testing we don't marshal nil
	}}

	for _, test := range tests {
		if !test.skipUnmarshal {
			var h Hover
			if err := json.Unmarshal(test.data, &h); err != nil {
				t.Errorf("json.Unmarshal error: %s", err)
				continue
			}
			if !reflect.DeepEqual(test.want, h) {
				t.Errorf("Unmarshaled %q, expected %+v, but got %+v", string(test.data), test.want, h)
				continue
			}
		}

		if !test.skipMarshal {
			marshaled, err := json.Marshal(&test.want)
			if err != nil {
				t.Errorf("json.Marshal error: %s", err)
				continue
			}
			if string(marshaled) != string(test.data) {
				t.Errorf("Marshaled result expected %s, but got %s", string(test.data), string(marshaled))
			}
		}
	}
}
