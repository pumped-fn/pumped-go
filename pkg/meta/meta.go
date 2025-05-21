package meta

import (
	"errors"
	"reflect"
)

// Schema defines validation rules for metadata
type Schema interface {
	// Validate validates a value against the schema
	Validate(value any) (any, error)
}

// Meta represents a metadata entry
type Meta struct {
	Key    string
	Schema Schema
	Value  any
}

// MetaOption is a function that configures a Meta
type MetaOption func(*Meta)

// WithSchema sets the schema for a Meta
func WithSchema(schema Schema) MetaOption {
	return func(m *Meta) {
		m.Schema = schema
	}
}

// New creates a new Meta
func New(key string, value any, options ...MetaOption) *Meta {
	m := &Meta{
		Key:   key,
		Value: value,
	}

	for _, option := range options {
		option(m)
	}

	return m
}

// Get retrieves a metadata value from a source
func Get[T any](source map[string]any, key string) (T, error) {
	if source == nil {
		var zero T
		return zero, errors.New("metadata source is nil")
	}

	value, ok := source[key]
	if !ok {
		var zero T
		return zero, errors.New("metadata key not found")
	}

	// Try to convert the value to the requested type
	if result, ok := value.(T); ok {
		return result, nil
	}

	// Try to use reflection to convert the value
	sourceValue := reflect.ValueOf(value)
	targetType := reflect.TypeOf((*T)(nil)).Elem()

	if sourceValue.Type().ConvertibleTo(targetType) {
		convertedValue := sourceValue.Convert(targetType)
		return convertedValue.Interface().(T), nil
	}

	var zero T
	return zero, errors.New("metadata value cannot be converted to requested type")
}

// Set sets a metadata value in a source
func Set(source map[string]any, key string, value any) {
	if source == nil {
		return
	}

	source[key] = value
}

// Find finds all metadata entries with a given key
func Find(source map[string]any, key string) []any {
	if source == nil {
		return nil
	}

	value, ok := source[key]
	if !ok {
		return nil
	}

	return []any{value}
}

// CustomSchema is a schema that accepts any value
type CustomSchema struct{}

// Validate validates a value against the schema
func (s *CustomSchema) Validate(value any) (any, error) {
	return value, nil
}

// Custom creates a new custom schema
func Custom[T any]() Schema {
	return &CustomSchema{}
}

