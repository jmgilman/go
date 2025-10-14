// Package attributes provides attribute parsing infrastructure for CUE values.
package attributes

import (
	"fmt"
	"regexp"
	"strings"

	"cuelang.org/go/cue"
)

// Attribute represents a parsed CUE attribute with its context.
type Attribute struct {
	Name  string            // Attribute name (e.g., "artifact")
	Args  map[string]string // Parsed arguments (e.g., {"name": "api-server", "field": "uri"})
	Path  cue.Path          // Location in CUE tree
	Value cue.Value         // Original value with attribute
}

// ParseAttribute extracts attribute information from a CUE value.
// Returns (Attribute, true) if attribute found, (Attribute{}, false) if not found.
func ParseAttribute(value cue.Value, attrName string) (Attribute, bool) {
	// Get the attribute from the value
	attr := value.Attribute(attrName)
	
	// Check if attribute exists using Err()
	if attr.Err() != nil {
		return Attribute{}, false
	}
	
	// Get attribute text (may be empty for attributes with no arguments)
	attrText := attr.Contents()
	
	// Parse the arguments from the attribute text
	args, err := ParseArgs(attrText)
	if err != nil {
		// If parsing fails, return false (malformed attribute)
		return Attribute{}, false
	}
	
	// Build and return the Attribute struct
	return Attribute{
		Name:  attrName,
		Args:  args,
		Path:  value.Path(),
		Value: value,
	}, true
}

// ParseArgs parses attribute arguments from CUE attribute syntax.
// Example: @artifact(name="api-server", field="uri")
// The input string should be the contents within the parentheses: name="api-server", field="uri".
// Returns: map[string]string{"name": "api-server", "field": "uri"}.
func ParseArgs(attrText string) (map[string]string, error) {
	// Handle empty attribute (no arguments)
	attrText = strings.TrimSpace(attrText)
	if attrText == "" {
		return make(map[string]string), nil
	}
	
	// Regular expression to match key="value" patterns
	// Supports:
	// - Simple values: key="value"
	// - Values with spaces: key="value with spaces"
	// - Values with special characters: key="value-with_special.chars"
	// - Escaped quotes: key="value with \"quotes\""
	keyValuePattern := regexp.MustCompile(`(\w+)="([^"\\]*(?:\\.[^"\\]*)*)"`)
	
	matches := keyValuePattern.FindAllStringSubmatch(attrText, -1)
	if len(matches) == 0 {
		return nil, fmt.Errorf("malformed attribute syntax: no valid key=\"value\" pairs found in %q", attrText)
	}
	
	args := make(map[string]string)
	for _, match := range matches {
		if len(match) != 3 {
			continue
		}
		key := match[1]
		value := match[2]
		
		// Unescape any escaped quotes in the value
		value = strings.ReplaceAll(value, `\"`, `"`)
		
		args[key] = value
	}
	
	if len(args) == 0 {
		return nil, fmt.Errorf("malformed attribute syntax: could not parse arguments from %q", attrText)
	}
	
	return args, nil
}
