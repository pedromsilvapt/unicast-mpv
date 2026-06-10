package cases

import (
	"testing"
)

func TestConvertCamelCase(t *testing.T) {
	input := map[string]interface{}{
		"media-title":   "test",
		"sub-fix-timing": true,
	}
	expected := map[string]interface{}{
		"mediaTitle":   "test",
		"subFixTiming": true,
	}
	result := Convert(input, Camel)
	assertMapEqual(t, expected, result)
}

func TestConvertKebabCase(t *testing.T) {
	input := map[string]interface{}{
		"mediaTitle":   "test",
		"subFixTiming": true,
	}
	expected := map[string]interface{}{
		"media-title":    "test",
		"sub-fix-timing": true,
	}
	result := Convert(input, Kebab)
	assertMapEqual(t, expected, result)
}

func TestConvertSnakeCase(t *testing.T) {
	input := map[string]interface{}{
		"media-title": "test",
	}
	expected := map[string]interface{}{
		"media_title": "test",
	}
	result := Convert(input, Snake)
	assertMapEqual(t, expected, result)
}

func TestConvertPascalCase(t *testing.T) {
	input := map[string]interface{}{
		"media-title": "test",
	}
	expected := map[string]interface{}{
		"MediaTitle": "test",
	}
	result := Convert(input, Pascal)
	assertMapEqual(t, expected, result)
}

func TestConvertConstantCase(t *testing.T) {
	input := map[string]interface{}{
		"media-title": "test",
	}
	expected := map[string]interface{}{
		"MEDIA_TITLE": "test",
	}
	result := Convert(input, Constant)
	assertMapEqual(t, expected, result)
}

func TestConvertNestedMaps(t *testing.T) {
	input := map[string]interface{}{
		"outer-key": map[string]interface{}{
			"inner-key": "value",
		},
	}
	expected := map[string]interface{}{
		"outerKey": map[string]interface{}{
			"innerKey": "value",
		},
	}
	result := Convert(input, Camel)
	assertMapEqual(t, expected, result)
}

func TestConvertArraysWithMaps(t *testing.T) {
	input := map[string]interface{}{
		"track-list": []interface{}{
			map[string]interface{}{
				"codec-name": "h264",
			},
		},
	}
	expected := map[string]interface{}{
		"trackList": []interface{}{
			map[string]interface{}{
				"codecName": "h264",
			},
		},
	}
	result := Convert(input, Camel)
	assertMapEqual(t, expected, result)
}

func TestConvertPreservesNonStringValues(t *testing.T) {
	input := map[string]interface{}{
		"some-key":  42,
		"other-key": 3.14,
		"bool-key":  true,
		"nil-key":   nil,
	}
	expected := map[string]interface{}{
		"someKey":  42,
		"otherKey": 3.14,
		"boolKey":  true,
		"nilKey":   nil,
	}
	result := Convert(input, Camel)
	assertMapEqual(t, expected, result)
}

func TestConvertEmptyMap(t *testing.T) {
	input := map[string]interface{}{}
	result := Convert(input, Camel)
	if len(result) != 0 {
		t.Errorf("expected empty map, got %v", result)
	}
}

func TestConvertAlreadyKebabStaysKebab(t *testing.T) {
	input := map[string]interface{}{
		"sub-fix-timing": true,
	}
	expected := map[string]interface{}{
		"sub-fix-timing": true,
	}
	result := Convert(input, Kebab)
	assertMapEqual(t, expected, result)
}

func TestConvertCamelToSnake(t *testing.T) {
	input := map[string]interface{}{
		"mediaTitle": "test",
	}
	expected := map[string]interface{}{
		"media_title": "test",
	}
	result := Convert(input, Snake)
	assertMapEqual(t, expected, result)
}

func assertMapEqual(t *testing.T, expected, actual map[string]interface{}) {
	t.Helper()
	if len(expected) != len(actual) {
		t.Errorf("map length mismatch: expected %d, got %d", len(expected), len(actual))
		return
	}
	for k, ev := range expected {
		av, ok := actual[k]
		if !ok {
			t.Errorf("missing key %q", k)
			continue
		}
		switch ev := ev.(type) {
		case map[string]interface{}:
			avMap, ok := av.(map[string]interface{})
			if !ok {
				t.Errorf("key %q: expected map, got %T", k, av)
				continue
			}
			assertMapEqual(t, ev, avMap)
		case []interface{}:
			avSlice, ok := av.([]interface{})
			if !ok {
				t.Errorf("key %q: expected slice, got %T", k, av)
				continue
			}
			if len(ev) != len(avSlice) {
				t.Errorf("key %q: slice length mismatch", k)
				continue
			}
			for i := range ev {
				evItem, ok1 := ev[i].(map[string]interface{})
				avItem, ok2 := avSlice[i].(map[string]interface{})
				if ok1 && ok2 {
					assertMapEqual(t, evItem, avItem)
				} else if ev[i] != avSlice[i] {
					t.Errorf("key %q[%d]: expected %v, got %v", k, i, ev[i], avSlice[i])
				}
			}
		default:
			if ev != av {
				t.Errorf("key %q: expected %v (%T), got %v (%T)", k, ev, ev, av, av)
			}
		}
	}
}