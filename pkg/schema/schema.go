package schema

import (
	"errors"
	"fmt"
	"reflect"
)

// ValidationError represents a validation error
type ValidationError struct {
	Message string
	Path    []string
}

// Error returns the error message
func (e *ValidationError) Error() string {
	if len(e.Path) > 0 {
		return fmt.Sprintf("%s at path %v", e.Message, e.Path)
	}
	return e.Message
}

// Schema defines validation rules
type Schema interface {
	// Validate validates a value against the schema
	Validate(value any) (any, error)
}

// StringSchema validates strings
type StringSchema struct {
	MinLength int
	MaxLength int
	Pattern   string
}

// Validate validates a string
func (s *StringSchema) Validate(value any) (any, error) {
	str, ok := value.(string)
	if !ok {
		return nil, &ValidationError{
			Message: "value is not a string",
		}
	}

	if s.MinLength > 0 && len(str) < s.MinLength {
		return nil, &ValidationError{
			Message: fmt.Sprintf("string length %d is less than minimum length %d", len(str), s.MinLength),
		}
	}

	if s.MaxLength > 0 && len(str) > s.MaxLength {
		return nil, &ValidationError{
			Message: fmt.Sprintf("string length %d is greater than maximum length %d", len(str), s.MaxLength),
		}
	}

	// TODO: Implement pattern validation

	return str, nil
}

// NumberSchema validates numbers
type NumberSchema struct {
	Min      float64
	Max      float64
	Positive bool
	Negative bool
	Integer  bool
}

// Validate validates a number
func (s *NumberSchema) Validate(value any) (any, error) {
	var num float64

	switch v := value.(type) {
	case int:
		num = float64(v)
	case int8:
		num = float64(v)
	case int16:
		num = float64(v)
	case int32:
		num = float64(v)
	case int64:
		num = float64(v)
	case uint:
		num = float64(v)
	case uint8:
		num = float64(v)
	case uint16:
		num = float64(v)
	case uint32:
		num = float64(v)
	case uint64:
		num = float64(v)
	case float32:
		num = float64(v)
	case float64:
		num = v
	default:
		return nil, &ValidationError{
			Message: "value is not a number",
		}
	}

	if s.Min != 0 && num < s.Min {
		return nil, &ValidationError{
			Message: fmt.Sprintf("number %f is less than minimum %f", num, s.Min),
		}
	}

	if s.Max != 0 && num > s.Max {
		return nil, &ValidationError{
			Message: fmt.Sprintf("number %f is greater than maximum %f", num, s.Max),
		}
	}

	if s.Positive && num <= 0 {
		return nil, &ValidationError{
			Message: "number must be positive",
		}
	}

	if s.Negative && num >= 0 {
		return nil, &ValidationError{
			Message: "number must be negative",
		}
	}

	if s.Integer && float64(int(num)) != num {
		return nil, &ValidationError{
			Message: "number must be an integer",
		}
	}

	return num, nil
}

// BooleanSchema validates booleans
type BooleanSchema struct{}

// Validate validates a boolean
func (s *BooleanSchema) Validate(value any) (any, error) {
	b, ok := value.(bool)
	if !ok {
		return nil, &ValidationError{
			Message: "value is not a boolean",
		}
	}

	return b, nil
}

// ArraySchema validates arrays
type ArraySchema struct {
	ItemSchema Schema
	MinItems   int
	MaxItems   int
}

// Validate validates an array
func (s *ArraySchema) Validate(value any) (any, error) {
	val := reflect.ValueOf(value)
	if val.Kind() != reflect.Slice && val.Kind() != reflect.Array {
		return nil, &ValidationError{
			Message: "value is not an array",
		}
	}

	length := val.Len()

	if s.MinItems > 0 && length < s.MinItems {
		return nil, &ValidationError{
			Message: fmt.Sprintf("array length %d is less than minimum length %d", length, s.MinItems),
		}
	}

	if s.MaxItems > 0 && length > s.MaxItems {
		return nil, &ValidationError{
			Message: fmt.Sprintf("array length %d is greater than maximum length %d", length, s.MaxItems),
		}
	}

	if s.ItemSchema != nil {
		result := reflect.MakeSlice(val.Type(), 0, length)

		for i := 0; i < length; i++ {
			item := val.Index(i).Interface()
			validatedItem, err := s.ItemSchema.Validate(item)
			if err != nil {
				if valErr, ok := err.(*ValidationError); ok {
					valErr.Path = append([]string{fmt.Sprintf("[%d]", i)}, valErr.Path...)
				}
				return nil, err
			}

			result = reflect.Append(result, reflect.ValueOf(validatedItem))
		}

		return result.Interface(), nil
	}

	return value, nil
}

// ObjectSchema validates objects
type ObjectSchema struct {
	Properties map[string]Schema
	Required   []string
}

// Validate validates an object
func (s *ObjectSchema) Validate(value any) (any, error) {
	val := reflect.ValueOf(value)
	if val.Kind() != reflect.Map && val.Kind() != reflect.Struct {
		return nil, &ValidationError{
			Message: "value is not an object",
		}
	}

	if val.Kind() == reflect.Map {
		// Validate map
		result := reflect.MakeMap(val.Type())

		// Check required properties
		for _, req := range s.Required {
			if val.MapIndex(reflect.ValueOf(req)).IsNil() {
				return nil, &ValidationError{
					Message: fmt.Sprintf("required property %s is missing", req),
				}
			}
		}

		// Validate properties
		for key, schema := range s.Properties {
			keyVal := reflect.ValueOf(key)
			propVal := val.MapIndex(keyVal)

			if propVal.IsValid() {
				validatedProp, err := schema.Validate(propVal.Interface())
				if err != nil {
					if valErr, ok := err.(*ValidationError); ok {
						valErr.Path = append([]string{key}, valErr.Path...)
					}
					return nil, err
				}

				result.SetMapIndex(keyVal, reflect.ValueOf(validatedProp))
			} else {
				// Property not present, check if required
				for _, req := range s.Required {
					if req == key {
						return nil, &ValidationError{
							Message: fmt.Sprintf("required property %s is missing", key),
						}
					}
				}
			}
		}

		return result.Interface(), nil
	}

	// Validate struct
	result := reflect.New(val.Type()).Elem()

	// Check required properties
	for _, req := range s.Required {
		field := val.FieldByName(req)
		if !field.IsValid() {
			return nil, &ValidationError{
				Message: fmt.Sprintf("required property %s is missing", req),
			}
		}
	}

	// Validate properties
	for key, schema := range s.Properties {
		field := val.FieldByName(key)

		if field.IsValid() {
			validatedField, err := schema.Validate(field.Interface())
			if err != nil {
				if valErr, ok := err.(*ValidationError); ok {
					valErr.Path = append([]string{key}, valErr.Path...)
				}
				return nil, err
			}

			result.FieldByName(key).Set(reflect.ValueOf(validatedField))
		} else {
			// Property not present, check if required
			for _, req := range s.Required {
				if req == key {
					return nil, &ValidationError{
						Message: fmt.Sprintf("required property %s is missing", key),
					}
				}
			}
		}
	}

	return result.Interface(), nil
}

// CustomSchema is a schema that accepts any value
type CustomSchema struct{}

// Validate validates a value against the schema
func (s *CustomSchema) Validate(value any) (any, error) {
	return value, nil
}

// String creates a new string schema
func String() *StringSchema {
	return &StringSchema{}
}

// Number creates a new number schema
func Number() *NumberSchema {
	return &NumberSchema{}
}

// Boolean creates a new boolean schema
func Boolean() *BooleanSchema {
	return &BooleanSchema{}
}

// Array creates a new array schema
func Array(itemSchema Schema) *ArraySchema {
	return &ArraySchema{
		ItemSchema: itemSchema,
	}
}

// Object creates a new object schema
func Object(properties map[string]Schema) *ObjectSchema {
	return &ObjectSchema{
		Properties: properties,
	}
}

// Custom creates a new custom schema
func Custom[T any]() Schema {
	return &CustomSchema{}
}

