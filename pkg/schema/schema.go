package schema

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

type ValidationError struct {
	Expected []string
	Received string
	Property string
	Errors   []*ValidationError
}

func (e *ValidationError) Error() string {
	if len(e.Errors) > 0 {
		var msgs []string
		for _, err := range e.Errors {
			msgs = append(msgs, err.Error())
		}
		return strings.Join(msgs, "\n")
	}
	expectations := fmt.Sprintf("Expected %s, got %s instead.",
		strings.Join(e.Expected, ", "), e.Received)
	if e.Property != "" {
		return fmt.Sprintf("%s: %s", e.Property, expectations)
	}
	return expectations
}

func (e *ValidationError) Prefix(property string) *ValidationError {
	if len(e.Errors) > 0 {
		prefixed := make([]*ValidationError, len(e.Errors))
		for i, err := range e.Errors {
			prefixed[i] = err.Prefix(property)
		}
		return &ValidationError{Errors: prefixed}
	}
	prop := property
	if e.Property != "" {
		prop = property + "." + e.Property
	}
	return &ValidationError{
		Expected: e.Expected,
		Received: e.Received,
		Property: prop,
	}
}

func ToString(err *ValidationError) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

type Schema interface {
	Validate(data interface{}) *ValidationError
}

func typeName(data interface{}) string {
	if data == nil {
		return "nil"
	}
	switch data.(type) {
	case string:
		return "string"
	case bool:
		return "boolean"
	case int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64:
		return "number"
	default:
		r := reflect.ValueOf(data)
		switch r.Kind() {
		case reflect.Slice, reflect.Array:
			return "object"
		case reflect.Map:
			return "object"
		case reflect.Struct:
			return "object"
		default:
			return r.Kind().String()
		}
	}
}

func formatValue(v interface{}) string {
	if v == nil {
		return "nil"
	}
	switch val := v.(type) {
	case string:
		return fmt.Sprintf("\"%s\"", val)
	default:
		return fmt.Sprintf("%v", val)
	}
}

func toSlice(data interface{}) ([]interface{}, bool) {
	if data == nil {
		return nil, false
	}
	v := reflect.ValueOf(data)
	if v.Kind() != reflect.Slice && v.Kind() != reflect.Array {
		return nil, false
	}
	result := make([]interface{}, v.Len())
	for i := 0; i < v.Len(); i++ {
		result[i] = v.Index(i).Interface()
	}
	return result, true
}

func toMap(data interface{}) (map[string]interface{}, bool) {
	if data == nil {
		return nil, false
	}
	v := reflect.ValueOf(data)
	if v.Kind() != reflect.Map {
		return nil, false
	}
	result := make(map[string]interface{})
	iter := v.MapRange()
	for iter.Next() {
		key := iter.Key()
		if key.Kind() == reflect.String {
			result[key.String()] = iter.Value().Interface()
		}
	}
	return result, true
}

func prefixErrors(err *ValidationError, prefix string) []*ValidationError {
	if err == nil {
		return nil
	}
	if len(err.Errors) > 0 {
		var result []*ValidationError
		for _, e := range err.Errors {
			result = append(result, prefixErrors(e, prefix)...)
		}
		return result
	}
	return []*ValidationError{err.Prefix(prefix)}
}

func collectLeafErrors(err *ValidationError) []*ValidationError {
	if err == nil {
		return nil
	}
	if len(err.Errors) > 0 {
		var result []*ValidationError
		for _, e := range err.Errors {
			result = append(result, collectLeafErrors(e)...)
		}
		return result
	}
	return []*ValidationError{err}
}

// StringSchema validates that data is a string.
type StringSchema struct{}

func (s *StringSchema) Validate(data interface{}) *ValidationError {
	if _, ok := data.(string); ok {
		return nil
	}
	return &ValidationError{Expected: []string{"String"}, Received: typeName(data)}
}

func String() *StringSchema { return &StringSchema{} }

// NumberSchema validates that data is a numeric type.
type NumberSchema struct{}

func (s *NumberSchema) Validate(data interface{}) *ValidationError {
	switch data.(type) {
	case int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64:
		return nil
	}
	return &ValidationError{Expected: []string{"Number"}, Received: typeName(data)}
}

func Number() *NumberSchema { return &NumberSchema{} }

// BooleanSchema validates that data is a boolean.
type BooleanSchema struct{}

func (s *BooleanSchema) Validate(data interface{}) *ValidationError {
	if _, ok := data.(bool); ok {
		return nil
	}
	return &ValidationError{Expected: []string{"Boolean"}, Received: typeName(data)}
}

func Boolean() *BooleanSchema { return &BooleanSchema{} }

// AnySchema accepts any data without validation.
type AnySchema struct{}

func (s *AnySchema) Validate(data interface{}) *ValidationError {
	return nil
}

func Any() *AnySchema { return &AnySchema{} }

// OptionalSchema validates data against a sub-schema if present, or accepts nil.
type OptionalSchema struct {
	SubSchema    Schema
	DefaultValue interface{}
}

func (s *OptionalSchema) Validate(data interface{}) *ValidationError {
	if data == nil {
		return nil
	}
	return s.SubSchema.Validate(data)
}

func Optional(sub Schema, defaultVal ...interface{}) *OptionalSchema {
	s := &OptionalSchema{SubSchema: sub}
	if len(defaultVal) > 0 {
		s.DefaultValue = defaultVal[0]
	}
	return s
}

// ConstantSchema validates that data equals a specific constant value.
type ConstantSchema struct {
	Value interface{}
}

func (s *ConstantSchema) Validate(data interface{}) *ValidationError {
	if reflect.DeepEqual(data, s.Value) {
		return nil
	}
	return &ValidationError{
		Expected: []string{formatValue(s.Value)},
		Received: formatValue(data),
	}
}

func Constant(val interface{}) *ConstantSchema { return &ConstantSchema{Value: val} }

// TupleSchema validates an array where each position matches a specific schema.
type TupleSchema struct {
	Schemas []Schema
}

func (s *TupleSchema) Validate(data interface{}) *ValidationError {
	arr, ok := toSlice(data)
	if !ok {
		return &ValidationError{Expected: []string{"Array"}, Received: typeName(data)}
	}
	var errors []*ValidationError
	schemaLen := len(s.Schemas)
	for i := 0; i < schemaLen; i++ {
		if i >= len(arr) {
			if err := s.Schemas[i].Validate(nil); err != nil {
				errors = append(errors, prefixErrors(err, strconv.Itoa(i))...)
			}
		} else if err := s.Schemas[i].Validate(arr[i]); err != nil {
			errors = append(errors, prefixErrors(err, strconv.Itoa(i))...)
		}
	}
	for i := schemaLen; i < len(arr); i++ {
		errors = append(errors, &ValidationError{
			Expected: []string{"Undefined"},
			Received: typeName(arr[i]),
			Property: strconv.Itoa(i),
		})
	}
	if len(errors) == 0 {
		return nil
	}
	return &ValidationError{Errors: errors}
}

func Tuple(schemas ...Schema) *TupleSchema {
	return &TupleSchema{Schemas: schemas}
}

// ArraySchema validates that data is an array with all items matching a sub-schema.
type ArraySchema struct {
	SubSchema Schema
}

func (s *ArraySchema) Validate(data interface{}) *ValidationError {
	arr, ok := toSlice(data)
	if !ok {
		return &ValidationError{Expected: []string{"Array"}, Received: typeName(data)}
	}
	var errors []*ValidationError
	for i, item := range arr {
		if err := s.SubSchema.Validate(item); err != nil {
			errors = append(errors, prefixErrors(err, strconv.Itoa(i))...)
		}
	}
	if len(errors) == 0 {
		return nil
	}
	return &ValidationError{Errors: errors}
}

func Array(sub Schema) *ArraySchema { return &ArraySchema{SubSchema: sub} }

// ObjectSchema validates an object with typed properties.
type ObjectSchema struct {
	Properties map[string]Schema
	Strict     bool
}

func (s *ObjectSchema) Validate(data interface{}) *ValidationError {
	m, ok := toMap(data)
	if !ok {
		return &ValidationError{Expected: []string{"Object"}, Received: typeName(data)}
	}
	var errors []*ValidationError

	requiredKeys := make(map[string]bool)
	for k := range s.Properties {
		requiredKeys[k] = true
	}

	for k, v := range m {
		if propSchema, exists := s.Properties[k]; exists {
			delete(requiredKeys, k)
			if err := propSchema.Validate(v); err != nil {
				errors = append(errors, prefixErrors(err, k)...)
			}
		} else if s.Strict {
			errors = append(errors, &ValidationError{
				Expected: []string{"Undefined"},
				Received: typeName(v),
				Property: k,
			})
		}
	}

	for k := range requiredKeys {
		if err := s.Properties[k].Validate(nil); err != nil {
			errors = append(errors, prefixErrors(err, k)...)
		}
	}

	if len(errors) == 0 {
		return nil
	}
	return &ValidationError{Errors: errors}
}

func Object(props map[string]Schema, strict ...bool) *ObjectSchema {
	s := &ObjectSchema{Properties: props}
	if len(strict) > 0 {
		s.Strict = strict[0]
	}
	return s
}

// UnionSchema validates that data matches at least one sub-schema.
type UnionSchema struct {
	Schemas []Schema
}

func (s *UnionSchema) Validate(data interface{}) *ValidationError {
	var allErrors []*ValidationError
	for _, schema := range s.Schemas {
		if err := schema.Validate(data); err == nil {
			return nil
		} else {
			allErrors = append(allErrors, collectLeafErrors(err)...)
		}
	}
	if len(allErrors) == 0 {
		return &ValidationError{Expected: []string{"any of union"}, Received: typeName(data)}
	}
	return &ValidationError{Errors: allErrors}
}

func Union(schemas ...Schema) *UnionSchema {
	return &UnionSchema{Schemas: schemas}
}

// IntersectionSchema validates that data matches all sub-schemas.
type IntersectionSchema struct {
	Schemas []Schema
}

func (s *IntersectionSchema) Validate(data interface{}) *ValidationError {
	var allErrors []*ValidationError
	for _, schema := range s.Schemas {
		if err := schema.Validate(data); err != nil {
			allErrors = append(allErrors, collectLeafErrors(err)...)
		}
	}
	if len(allErrors) == 0 {
		return nil
	}
	return &ValidationError{Errors: allErrors}
}

func Intersection(schemas ...Schema) *IntersectionSchema {
	return &IntersectionSchema{Schemas: schemas}
}