package schema

import (
	"fmt"
	"strings"
	"testing"
)

func assertNoError(t *testing.T, err *ValidationError, msg ...string) {
	t.Helper()
	if err != nil {
		detail := strings.Join(msg, " ")
		t.Fatalf("expected no error, got: %s (%s)", err.Error(), detail)
	}
}

func assertError(t *testing.T, err *ValidationError, msg ...string) {
	t.Helper()
	if err == nil {
		detail := strings.Join(msg, " ")
		t.Fatalf("expected error, got nil (%s)", detail)
	}
}

func assertErrorContains(t *testing.T, err *ValidationError, substr string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error containing %q, got nil", substr)
	}
	if !strings.Contains(err.Error(), substr) {
		t.Fatalf("expected error containing %q, got: %s", substr, err.Error())
	}
}

func TestStringSchema_Valid(t *testing.T) {
	err := String().Validate("hello")
	assertNoError(t, err)
}

func TestStringSchema_Invalid(t *testing.T) {
	err := String().Validate(42)
	assertError(t, err)
	assertErrorContains(t, err, "String")
}

func TestStringSchema_InvalidBool(t *testing.T) {
	err := String().Validate(true)
	assertError(t, err)
	assertErrorContains(t, err, "String")
}

func TestStringSchema_Nil(t *testing.T) {
	err := String().Validate(nil)
	assertError(t, err)
}

func TestNumberSchema_Valid(t *testing.T) {
	tests := []interface{}{int(42), int8(1), int16(1), int32(1), int64(1),
		uint(1), uint8(1), uint16(1), uint32(1), uint64(1),
		float32(1.5), float64(2.5)}
	for _, v := range tests {
		err := Number().Validate(v)
		assertNoError(t, err, fmt.Sprintf("for value %v", v))
	}
}

func TestNumberSchema_Invalid(t *testing.T) {
	err := Number().Validate("not a number")
	assertError(t, err)
	assertErrorContains(t, err, "Number")
}

func TestNumberSchema_InvalidBool(t *testing.T) {
	err := Number().Validate(true)
	assertError(t, err)
}

func TestBooleanSchema_Valid(t *testing.T) {
	err := Boolean().Validate(true)
	assertNoError(t, err)
	err = Boolean().Validate(false)
	assertNoError(t, err)
}

func TestBooleanSchema_Invalid(t *testing.T) {
	err := Boolean().Validate("not bool")
	assertError(t, err)
	assertErrorContains(t, err, "Boolean")
}

func TestBooleanSchema_InvalidNumber(t *testing.T) {
	err := Boolean().Validate(1)
	assertError(t, err)
}

func TestAnySchema_AlwaysPasses(t *testing.T) {
	tests := []interface{}{nil, "hello", 42, true, []int{1, 2, 3}}
	for _, v := range tests {
		err := Any().Validate(v)
		assertNoError(t, err, fmt.Sprintf("for value %v", v))
	}
}

func TestOptionalSchema_NilPasses(t *testing.T) {
	err := Optional(String()).Validate(nil)
	assertNoError(t, err)
}

func TestOptionalSchema_ValidValuePasses(t *testing.T) {
	err := Optional(String()).Validate("hello")
	assertNoError(t, err)
}

func TestOptionalSchema_InvalidValueFails(t *testing.T) {
	err := Optional(String()).Validate(42)
	assertError(t, err)
}

func TestOptionalSchema_WithDefaultValue(t *testing.T) {
	s := Optional(String(), "default")
	assertNoError(t, s.Validate(nil))
	assertNoError(t, s.Validate("valid"))
	err := s.Validate(42)
	assertError(t, err)
}

func TestConstantSchema_Valid(t *testing.T) {
	err := Constant("hello").Validate("hello")
	assertNoError(t, err)
	err = Constant(42).Validate(42)
	assertNoError(t, err)
	err = Constant(true).Validate(true)
	assertNoError(t, err)
}

func TestConstantSchema_Invalid(t *testing.T) {
	err := Constant("hello").Validate("world")
	assertError(t, err)
	assertErrorContains(t, err, `"hello"`)
}

func TestConstantSchema_InvalidWrongType(t *testing.T) {
	err := Constant(42).Validate("42")
	assertError(t, err)
}

func TestTupleSchema_Valid(t *testing.T) {
	s := Tuple(String(), Number(), Boolean())
	err := s.Validate([]interface{}{"hello", 42, true})
	assertNoError(t, err)
}

func TestTupleSchema_WrongType(t *testing.T) {
	s := Tuple(String(), Number())
	err := s.Validate("not an array")
	assertError(t, err)
	assertErrorContains(t, err, "Array")
}

func TestTupleSchema_InvalidElements(t *testing.T) {
	s := Tuple(String(), Number())
	err := s.Validate([]interface{}{42, "hello"})
	assertError(t, err)
}

func TestTupleSchema_ShortArray(t *testing.T) {
	s := Tuple(String(), Number())
	err := s.Validate([]interface{}{"hello"})
	assertError(t, err)
}

func TestTupleSchema_LongArray(t *testing.T) {
	s := Tuple(String())
	err := s.Validate([]interface{}{"hello", 42})
	assertError(t, err)
}

func TestTupleSchema_EmptyTuple(t *testing.T) {
	s := Tuple()
	err := s.Validate([]interface{}{})
	assertNoError(t, err)
}

func TestTupleSchema_EmptyTupleWithElements(t *testing.T) {
	s := Tuple()
	err := s.Validate([]interface{}{"extra"})
	assertError(t, err)
}

func TestArraySchema_Valid(t *testing.T) {
	s := Array(Number())
	err := s.Validate([]interface{}{1, 2, 3})
	assertNoError(t, err)
}

func TestArraySchema_EmptyArray(t *testing.T) {
	s := Array(String())
	err := s.Validate([]interface{}{})
	assertNoError(t, err)
}

func TestArraySchema_WrongType(t *testing.T) {
	s := Array(Number())
	err := s.Validate("not an array")
	assertError(t, err)
	assertErrorContains(t, err, "Array")
}

func TestArraySchema_InvalidElements(t *testing.T) {
	s := Array(Number())
	err := s.Validate([]interface{}{1, "two", 3})
	assertError(t, err)
}

func TestArraySchema_NestedSchemas(t *testing.T) {
	s := Array(Tuple(String(), Number()))
	data := []interface{}{
		[]interface{}{"hello", 1},
		[]interface{}{"world", 2},
	}
	err := s.Validate(data)
	assertNoError(t, err)
}

func TestObjectSchema_Valid(t *testing.T) {
	s := Object(map[string]Schema{
		"name": String(),
		"age":  Number(),
	})
	err := s.Validate(map[string]interface{}{"name": "Alice", "age": 30})
	assertNoError(t, err)
}

func TestObjectSchema_WrongType(t *testing.T) {
	s := Object(map[string]Schema{})
	err := s.Validate("not an object")
	assertError(t, err)
	assertErrorContains(t, err, "Object")
}

func TestObjectSchema_NilFails(t *testing.T) {
	s := Object(map[string]Schema{"name": String()})
	err := s.Validate(nil)
	assertError(t, err)
}

func TestObjectSchema_MissingRequiredField(t *testing.T) {
	s := Object(map[string]Schema{
		"name": String(),
		"age":  Number(),
	})
	err := s.Validate(map[string]interface{}{"name": "Alice"})
	assertError(t, err)
}

func TestObjectSchema_ExtraFieldNonStrict(t *testing.T) {
	s := Object(map[string]Schema{
		"name": String(),
	})
	err := s.Validate(map[string]interface{}{"name": "Alice", "extra": "field"})
	assertNoError(t, err)
}

func TestObjectSchema_ExtraFieldStrict(t *testing.T) {
	s := Object(map[string]Schema{
		"name": String(),
	}, true)
	err := s.Validate(map[string]interface{}{"name": "Alice", "extra": "field"})
	assertError(t, err)
	assertErrorContains(t, err, "Undefined")
}

func TestObjectSchema_OptionalField(t *testing.T) {
	s := Object(map[string]Schema{
		"name": String(),
		"age":  Optional(Number()),
	})
	err := s.Validate(map[string]interface{}{"name": "Alice"})
	assertNoError(t, err)
}

func TestObjectSchema_NestedObjectValidation(t *testing.T) {
	inner := Object(map[string]Schema{
		"x": Number(),
	})
	s := Object(map[string]Schema{
		"inner": inner,
	})
	err := s.Validate(map[string]interface{}{
		"inner": map[string]interface{}{"x": "not a number"},
	})
	assertError(t, err)
	assertErrorContains(t, err, "inner")
}

func TestUnionSchema_ValidFirstOption(t *testing.T) {
	s := Union(String(), Number())
	err := s.Validate("hello")
	assertNoError(t, err)
}

func TestUnionSchema_ValidSecondOption(t *testing.T) {
	s := Union(String(), Number())
	err := s.Validate(42)
	assertNoError(t, err)
}

func TestUnionSchema_NoMatch(t *testing.T) {
	s := Union(String(), Number())
	err := s.Validate(true)
	assertError(t, err)
}

func TestUnionSchema_WithAny(t *testing.T) {
	s := Union(String(), Any())
	err := s.Validate(42)
	assertNoError(t, err)
}

func TestIntersectionSchema_AllMatch(t *testing.T) {
	s := Intersection(Object(map[string]Schema{
		"name": String(),
	}), Object(map[string]Schema{
		"age": Number(),
	}))
	err := s.Validate(map[string]interface{}{"name": "Alice", "age": 30})
	assertNoError(t, err)
}

func TestIntersectionSchema_PartialMatch(t *testing.T) {
	s := Intersection(Object(map[string]Schema{
		"name": String(),
	}), Object(map[string]Schema{
		"age": Number(),
	}))
	err := s.Validate(map[string]interface{}{"name": "Alice"})
	assertError(t, err)
}

func TestIntersectionSchema_NoneMatch(t *testing.T) {
	s := Intersection(String(), Number())
	err := s.Validate(true)
	assertError(t, err)
}

func TestNestedValidation_ArrayOfObjects(t *testing.T) {
	s := Array(Object(map[string]Schema{
		"name": String(),
		"age":  Number(),
	}))
	data := []interface{}{
		map[string]interface{}{"name": "Alice", "age": 30},
		map[string]interface{}{"name": "Bob", "age": 25},
	}
	err := s.Validate(data)
	assertNoError(t, err)
}

func TestNestedValidation_ArrayOfObjectsFailure(t *testing.T) {
	s := Array(Object(map[string]Schema{
		"name": String(),
		"age":  Number(),
	}))
	data := []interface{}{
		map[string]interface{}{"name": "Alice", "age": 30},
		map[string]interface{}{"name": 123, "age": "25"},
	}
	err := s.Validate(data)
	assertError(t, err)
	assertErrorContains(t, err, "1")
}

func TestNestedValidation_ObjectWithTuple(t *testing.T) {
	s := Object(map[string]Schema{
		"point": Tuple(Number(), Number()),
	})
	data := map[string]interface{}{
		"point": []interface{}{1.0, 2.0},
	}
	err := s.Validate(data)
	assertNoError(t, err)
}

func TestValidationError_Error(t *testing.T) {
	err := &ValidationError{
		Expected: []string{"String"},
		Received: "number",
	}
	msg := err.Error()
	if !strings.Contains(msg, "Expected String") {
		t.Fatalf("expected error message to contain 'Expected String', got: %s", msg)
	}
	if !strings.Contains(msg, "got number") {
		t.Fatalf("expected error message to contain 'got number', got: %s", msg)
	}
}

func TestValidationError_ErrorWithProperty(t *testing.T) {
	err := &ValidationError{
		Expected: []string{"String"},
		Received: "number",
		Property: "name",
	}
	msg := err.Error()
	if !strings.Contains(msg, "name:") {
		t.Fatalf("expected error message to contain property, got: %s", msg)
	}
}

func TestValidationError_Prefix(t *testing.T) {
	err := &ValidationError{
		Expected: []string{"String"},
		Received: "number",
		Property: "name",
	}
	prefixed := err.Prefix("user")
	if prefixed.Property != "user.name" {
		t.Fatalf("expected property 'user.name', got: %s", prefixed.Property)
	}
}

func TestValidationError_PrefixEmpty(t *testing.T) {
	err := &ValidationError{
		Expected: []string{"String"},
		Received: "number",
		Property: "",
	}
	prefixed := err.Prefix("root")
	if prefixed.Property != "root" {
		t.Fatalf("expected property 'root', got: %s", prefixed.Property)
	}
}

func TestValidationError_NestedErrors(t *testing.T) {
	err := &ValidationError{
		Errors: []*ValidationError{
			{Expected: []string{"String"}, Received: "number", Property: "0"},
			{Expected: []string{"Number"}, Received: "string", Property: "1"},
		},
	}
	msg := err.Error()
	if !strings.Contains(msg, "0:") {
		t.Fatalf("expected nested error with '0:', got: %s", msg)
	}
	if !strings.Contains(msg, "1:") {
		t.Fatalf("expected nested error with '1:', got: %s", msg)
	}
}

func TestToString_Nil(t *testing.T) {
	result := ToString(nil)
	if result != "" {
		t.Fatalf("expected empty string for nil, got: %s", result)
	}
}

func TestToString_WithErrors(t *testing.T) {
	err := &ValidationError{Expected: []string{"String"}, Received: "number"}
	result := ToString(err)
	if result == "" {
		t.Fatalf("expected non-empty string, got empty")
	}
}

func TestObjectSchema_WithOptionalAndRequired(t *testing.T) {
	s := Object(map[string]Schema{
		"name":  String(),
		"email": Optional(String()),
	})
	err := s.Validate(map[string]interface{}{"name": "Alice"})
	assertNoError(t, err)
}

func TestTupleSchema_PropertyPathInErrors(t *testing.T) {
	s := Tuple(String(), Number())
	err := s.Validate([]interface{}{42, "hello"})
	assertError(t, err)
	assertErrorContains(t, err, "0")
	assertErrorContains(t, err, "1")
}

func TestArraySchema_PropertyPathInErrors(t *testing.T) {
	s := Array(String())
	err := s.Validate([]interface{}{"ok", 42, "also ok"})
	assertError(t, err)
	assertErrorContains(t, err, "1")
}

func TestUnionSchema_WithObject(t *testing.T) {
	s := Union(String(), Object(map[string]Schema{
		"key": String(),
	}))
	err := s.Validate(map[string]interface{}{"key": "value"})
	assertNoError(t, err)
	err = s.Validate("string value")
	assertNoError(t, err)
}

func TestIntersectionSchema_Empty(t *testing.T) {
	s := Intersection()
	assertNoError(t, s.Validate("anything"))
}

func TestUnionSchema_Empty(t *testing.T) {
	s := Union()
	err := s.Validate("anything")
	assertError(t, err)
}

func TestConstantSchema_Nil(t *testing.T) {
	err := Constant(nil).Validate(nil)
	assertNoError(t, err)
}

func TestObjectSchema_EmptyObject(t *testing.T) {
	s := Object(map[string]Schema{})
	err := s.Validate(map[string]interface{}{})
	assertNoError(t, err)
}

func TestNestedValidation_DeeplyNested(t *testing.T) {
	s := Object(map[string]Schema{
		"user": Object(map[string]Schema{
			"profile": Object(map[string]Schema{
				"age": Number(),
			}),
		}),
	})
	err := s.Validate(map[string]interface{}{
		"user": map[string]interface{}{
			"profile": map[string]interface{}{
				"age": "not a number",
			},
		},
	})
	assertError(t, err)
}

func TestOptionalSchema_WrappedOptional(t *testing.T) {
	s := Optional(Optional(String()))
	assertNoError(t, s.Validate(nil))
	assertNoError(t, s.Validate("hello"))
	assertError(t, s.Validate(42))
}

func TestTupleSchema_TypedSlice(t *testing.T) {
	s := Tuple(String(), Number())
	err := s.Validate([]string{"hello", "world"})
	assertError(t, err)
}

func TestArraySchema_TypedSlice(t *testing.T) {
	s := Array(Number())
	err := s.Validate([]int{1, 2, 3})
	assertNoError(t, err)
}