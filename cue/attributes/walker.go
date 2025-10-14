package attributes

import (
	"context"
	"fmt"

	"cuelang.org/go/cue"
)

// Walker traverses CUE values and applies registered processors.
type Walker struct {
	registry *Registry
	cueCtx   *cue.Context
}

// NewWalker creates a walker with a registry and CUE context.
func NewWalker(registry *Registry, cueCtx *cue.Context) *Walker {
	return &Walker{
		registry: registry,
		cueCtx:   cueCtx,
	}
}

// Walk traverses a CUE value, finds attributes, and applies processors.
// Unknown attributes are ignored (forward compatibility).
// Processor errors fill the path with an error value and continue walking.
func (w *Walker) Walk(ctx context.Context, value cue.Value) (cue.Value, error) {
	return w.walkValue(ctx, value)
}

// walkValue recursively processes a CUE value and its children.
func (w *Walker) walkValue(ctx context.Context, value cue.Value) (cue.Value, error) {
	// Based on the value's kind, walk appropriately
	switch value.Kind() {
	case cue.StructKind:
		return w.walkStruct(ctx, value)
	case cue.ListKind:
		return w.walkList(ctx, value)
	default:
		// For scalar values, process attributes
		return w.processValue(ctx, value)
	}
}

// processValue processes attributes on a value and then walks its children.
func (w *Walker) processValue(ctx context.Context, value cue.Value) (cue.Value, error) {
	// Check for attributes on this value
	attrs := value.Attributes(cue.ValueAttr)
	
	// Process each attribute
	for _, attr := range attrs {
		attrName := attr.Name()
		
		// Check if we have a processor for this attribute
		processor, ok := w.registry.Get(attrName)
		if !ok {
			// Unknown attribute - ignore it (forward compatibility)
			continue
		}
		
		// Parse the attribute
		parsedAttr, ok := ParseAttribute(value, attrName)
		if !ok {
			// Malformed attribute - skip it
			continue
		}
		
		// Call the processor
		newValue, err := processor.Process(ctx, parsedAttr)
		if err != nil {
			// Processor error - return an error value
			errorMsg := fmt.Sprintf("attribute processing failed: %v", err)
			return w.cueCtx.CompileString(fmt.Sprintf("_|_ // %s", errorMsg)), nil
		}
		
		// Replace the value with the processed result
		// Only process the first registered attribute
		value = newValue
		break
	}
	
	return value, nil
}

// walkStruct walks through a struct's fields.
func (w *Walker) walkStruct(ctx context.Context, value cue.Value) (cue.Value, error) {
	// Build a map to collect processed fields
	processedFields := make(map[string]string)
	
	// Iterate through all fields
	iter, err := value.Fields(cue.All())
	if err != nil {
		return value, nil // Return original value if we can't iterate
	}
	
	for iter.Next() {
		fieldName := iter.Selector().String()
		fieldValue := iter.Value()
		
		// First check for attributes on the field itself
		processedField, err := w.processValue(ctx, fieldValue)
		if err != nil {
			return cue.Value{}, err
		}
		
		// Then recursively walk the field's children
		finalField, err := w.walkValue(ctx, processedField)
		if err != nil {
			return cue.Value{}, err
		}
		
		// Marshal the processed field value to CUE syntax
		// This is a simplified approach
		bytes, err := finalField.MarshalJSON()
		if err == nil {
			processedFields[fieldName] = string(bytes)
		}
	}
	
	// Build a new struct from the processed fields
	structStr := "{"
	first := true
	for name, jsonValue := range processedFields {
		if !first {
			structStr += ", "
		}
		first = false
		structStr += fmt.Sprintf("%s: %s", name, jsonValue)
	}
	structStr += "}"
	
	return w.cueCtx.CompileString(structStr), nil
}

// walkList walks through list elements.
func (w *Walker) walkList(ctx context.Context, value cue.Value) (cue.Value, error) {
	// Build a list of processed elements
	var processedElements []string
	
	// Get list iterator
	iter, err := value.List()
	if err != nil {
		return value, nil // Return original if we can't iterate
	}
	
	for iter.Next() {
		elem := iter.Value()
		
		// Recursively walk this element
		processedElem, err := w.walkValue(ctx, elem)
		if err != nil {
			return cue.Value{}, err
		}
		
		// Marshal to JSON
		bytes, err := processedElem.MarshalJSON()
		if err == nil {
			processedElements = append(processedElements, string(bytes))
		}
	}
	
	// Build a new list
	listStr := "["
	for i, elem := range processedElements {
		if i > 0 {
			listStr += ", "
		}
		listStr += elem
	}
	listStr += "]"
	
	return w.cueCtx.CompileString(listStr), nil
}
