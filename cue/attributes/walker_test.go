package attributes

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
)

// walkerMockProcessor extends mockProcessor with additional tracking for walker tests.
type walkerMockProcessor struct {
	mockProcessor
	lastAttr *Attribute
}

func (m *walkerMockProcessor) Process(ctx context.Context, attr Attribute) (cue.Value, error) {
	m.lastAttr = &attr
	return m.mockProcessor.Process(ctx, attr)
}

func TestNewWalker(t *testing.T) {
	registry := NewRegistry()
	cueCtx := cuecontext.New()
	
	walker := NewWalker(registry, cueCtx)
	
	if walker == nil {
		t.Fatal("NewWalker returned nil")
	}
	if walker.registry != registry {
		t.Error("Walker registry not set correctly")
	}
	if walker.cueCtx != cueCtx {
		t.Error("Walker cueCtx not set correctly")
	}
}

func TestWalk_SimpleValue_NoAttributes(t *testing.T) {
	ctx := context.Background()
	cueCtx := cuecontext.New()
	registry := NewRegistry()
	walker := NewWalker(registry, cueCtx)
	
	// Simple value with no attributes
	value := cueCtx.CompileString(`42`)
	
	result, err := walker.Walk(ctx, value)
	if err != nil {
		t.Fatalf("Walk failed: %v", err)
	}
	
	// Result should be the same value
	var num int
	if err := result.Decode(&num); err != nil {
		t.Fatalf("Failed to decode result: %v", err)
	}
	if num != 42 {
		t.Errorf("Expected 42, got %d", num)
	}
}

func TestWalk_RegisteredAttribute(t *testing.T) {
	ctx := context.Background()
	cueCtx := cuecontext.New()
	registry := NewRegistry()
	
	// Create a mock processor that replaces with "replaced"
	mock := &mockProcessor{
		name: "test",
		processFunc: func(_ context.Context, _ Attribute) (cue.Value, error) {
			return cueCtx.CompileString(`"replaced"`), nil
		},
	}
	
	if err := registry.Register(mock); err != nil {
		t.Fatalf("Failed to register processor: %v", err)
	}
	
	walker := NewWalker(registry, cueCtx)
	
	// Value with @test attribute on a field
	value := cueCtx.CompileString(`{value: "original" @test()}`)
	
	result, err := walker.Walk(ctx, value)
	if err != nil {
		t.Fatalf("Walk failed: %v", err)
	}
	
	// Check that processor was called
	if mock.processCalled != 1 {
		t.Errorf("Expected processor to be called once, got %d calls", mock.processCalled)
	}
	
	// Check result
	resultValue := result.LookupPath(cue.ParsePath("value"))
	var str string
	if err := resultValue.Decode(&str); err != nil {
		t.Fatalf("Failed to decode result: %v", err)
	}
	if str != "replaced" {
		t.Errorf("Expected 'replaced', got %q", str)
	}
}

func TestWalk_ProcessorCalledWithCorrectAttribute(t *testing.T) {
	ctx := context.Background()
	cueCtx := cuecontext.New()
	registry := NewRegistry()
	
	// Create a walker mock processor that captures the attribute
	mock := &walkerMockProcessor{
		mockProcessor: mockProcessor{
			name: "artifact",
			processFunc: func(_ context.Context, _ Attribute) (cue.Value, error) {
				return cueCtx.CompileString(`"processed"`), nil
			},
		},
	}
	
	if err := registry.Register(mock); err != nil {
		t.Fatalf("Failed to register processor: %v", err)
	}
	
	walker := NewWalker(registry, cueCtx)
	
	// Value with @artifact attribute and arguments on a field
	value := cueCtx.CompileString(`{value: "test" @artifact(name="api-server", field="uri")}`)
	
	_, err := walker.Walk(ctx, value)
	if err != nil {
		t.Fatalf("Walk failed: %v", err)
	}
	
	// Verify the attribute passed to processor
	if mock.lastAttr == nil {
		t.Fatal("Processor was not called")
	}
	
	if mock.lastAttr.Name != "artifact" {
		t.Errorf("Expected attribute name 'artifact', got %q", mock.lastAttr.Name)
	}
	
	if mock.lastAttr.Args["name"] != "api-server" {
		t.Errorf("Expected arg name='api-server', got %q", mock.lastAttr.Args["name"])
	}
	
	if mock.lastAttr.Args["field"] != "uri" {
		t.Errorf("Expected arg field='uri', got %q", mock.lastAttr.Args["field"])
	}
}

func TestWalk_UnknownAttribute_Ignored(t *testing.T) {
	ctx := context.Background()
	cueCtx := cuecontext.New()
	registry := NewRegistry()
	
	// Register a processor for "known" but not "unknown"
	mock := &mockProcessor{
		name: "known",
		processFunc: func(_ context.Context, _ Attribute) (cue.Value, error) {
			return cueCtx.CompileString(`"processed"`), nil
		},
	}
	
	if err := registry.Register(mock); err != nil {
		t.Fatalf("Failed to register processor: %v", err)
	}
	
	walker := NewWalker(registry, cueCtx)
	
	// Value with unknown attribute on a field
	value := cueCtx.CompileString(`{value: "test" @unknown(arg="value")}`)
	
	result, err := walker.Walk(ctx, value)
	if err != nil {
		t.Fatalf("Walk failed: %v", err)
	}
	
	// Should not have called our processor
	if mock.processCalled != 0 {
		t.Errorf("Processor should not have been called for unknown attribute")
	}
	
	// Result should be original value
	resultValue := result.LookupPath(cue.ParsePath("value"))
	var str string
	if err := resultValue.Decode(&str); err != nil {
		t.Fatalf("Failed to decode result: %v", err)
	}
	if str != "test" {
		t.Errorf("Expected 'test', got %q", str)
	}
}

func TestWalk_ProcessorError_FillsErrorValue(t *testing.T) {
	ctx := context.Background()
	cueCtx := cuecontext.New()
	registry := NewRegistry()
	
	// Create a processor that returns an error
	mock := &mockProcessor{
		name: "failing",
		processFunc: func(_ context.Context, _ Attribute) (cue.Value, error) {
			return cue.Value{}, fmt.Errorf("processor failed")
		},
	}
	
	if err := registry.Register(mock); err != nil {
		t.Fatalf("Failed to register processor: %v", err)
	}
	
	walker := NewWalker(registry, cueCtx)
	
	// Value with failing attribute on a field
	value := cueCtx.CompileString(`{value: "test" @failing()}`)
	
	result, err := walker.Walk(ctx, value)
	// Walk should NOT return an error - it continues
	if err != nil {
		t.Fatalf("Walk should not return error, got: %v", err)
	}
	
	// Check that the processor was called
	if mock.processCalled != 1 {
		t.Error("Expected processor to be called once")
	}
	
	// Result field should be an error value (bottom)
	resultValue := result.LookupPath(cue.ParsePath("value"))
	if resultValue.Err() == nil {
		t.Error("Expected result field to be an error value")
	}
	
	// We just verify that an error occurred - the exact error message format
	// depends on how CUE marshals and unmarshals the error value
}

func TestWalk_NestedStruct_ProcessesAllLevels(t *testing.T) {
	ctx := context.Background()
	cueCtx := cuecontext.New()
	registry := NewRegistry()
	
	callPaths := []string{}
	
	// Create a processor that tracks which paths it's called on
	mock := &mockProcessor{
		name: "replace",
		processFunc: func(_ context.Context, attr Attribute) (cue.Value, error) {
			callPaths = append(callPaths, attr.Path.String())
			return cueCtx.CompileString(`"replaced"`), nil
		},
	}
	
	if err := registry.Register(mock); err != nil {
		t.Fatalf("Failed to register processor: %v", err)
	}
	
	walker := NewWalker(registry, cueCtx)
	
	// Nested struct with attributes at different levels
	value := cueCtx.CompileString(`{
		a: "val1" @replace()
		nested: {
			b: "val2" @replace()
		}
	}`)
	
	result, err := walker.Walk(ctx, value)
	if err != nil {
		t.Fatalf("Walk failed: %v", err)
	}
	
	// Should have been called twice
	if mock.processCalled != 2 {
		t.Errorf("Expected processor to be called twice, got %d calls", mock.processCalled)
	}
	
	// Check that both paths were processed
	if len(callPaths) != 2 {
		t.Fatalf("Expected 2 paths, got %d", len(callPaths))
	}
	
	// Verify the result has replaced values
	var output struct {
		A      string
		Nested struct {
			B string
		}
	}
	
	if err := result.Decode(&output); err != nil {
		t.Fatalf("Failed to decode result: %v", err)
	}
	
	if output.A != "replaced" {
		t.Errorf("Expected a='replaced', got %q", output.A)
	}
	if output.Nested.B != "replaced" {
		t.Errorf("Expected nested.b='replaced', got %q", output.Nested.B)
	}
}

func TestWalk_List_ProcessesElements(t *testing.T) {
	ctx := context.Background()
	cueCtx := cuecontext.New()
	registry := NewRegistry()
	
	callCount := 0
	
	// Create a processor that increments a counter
	mock := &mockProcessor{
		name: "process",
		processFunc: func(_ context.Context, _ Attribute) (cue.Value, error) {
			callCount++
			return cueCtx.CompileString(fmt.Sprintf(`"item%d"`, callCount)), nil
		},
	}
	
	if err := registry.Register(mock); err != nil {
		t.Fatalf("Failed to register processor: %v", err)
	}
	
	walker := NewWalker(registry, cueCtx)
	
	// List with struct elements that have attributes
	value := cueCtx.CompileString(`{
		items: [
			{val: "a" @process()},
			{val: "b" @process()},
			{val: "c" @process()}
		]
	}`)
	
	result, err := walker.Walk(ctx, value)
	if err != nil {
		t.Fatalf("Walk failed: %v", err)
	}
	
	// Should have been called 3 times
	if mock.processCalled != 3 {
		t.Errorf("Expected processor to be called 3 times, got %d calls", mock.processCalled)
	}
	
	// Verify the result
	items := result.LookupPath(cue.ParsePath("items"))
	iter, _ := items.List()
	
	index := 0
	expected := []string{"item1", "item2", "item3"}
	for iter.Next() {
		elem := iter.Value()
		valField := elem.LookupPath(cue.ParsePath("val"))
		var str string
		if err := valField.Decode(&str); err != nil {
			t.Fatalf("Failed to decode element %d: %v", index, err)
		}
		if str != expected[index] {
			t.Errorf("Element %d: expected %q, got %q", index, expected[index], str)
		}
		index++
	}
	
	if index != len(expected) {
		t.Errorf("Expected %d elements, got %d", len(expected), index)
	}
}

func TestWalk_ComplexStructure(t *testing.T) {
	ctx := context.Background()
	cueCtx := cuecontext.New()
	registry := NewRegistry()
	
	// Create a processor that replaces with uppercase version
	mock := &mockProcessor{
		name: "upper",
		processFunc: func(_ context.Context, attr Attribute) (cue.Value, error) {
			var str string
			if err := attr.Value.Decode(&str); err != nil {
				return cue.Value{}, fmt.Errorf("decode failed: %w", err)
			}
			return cueCtx.CompileString(fmt.Sprintf(`"%s"`, strings.ToUpper(str))), nil
		},
	}
	
	if err := registry.Register(mock); err != nil {
		t.Fatalf("Failed to register processor: %v", err)
	}
	
	walker := NewWalker(registry, cueCtx)
	
	// Complex nested structure
	value := cueCtx.CompileString(`{
		name: "test" @upper()
		items: [
			{value: "item1" @upper()},
			{value: "item2" @upper()}
		]
		config: {
			key: "key1" @upper()
			nested: {
				deep: "deep1" @upper()
			}
		}
	}`)
	
	result, err := walker.Walk(ctx, value)
	if err != nil {
		t.Fatalf("Walk failed: %v", err)
	}
	
	// Should have been called 5 times
	if mock.processCalled != 5 {
		t.Errorf("Expected processor to be called 5 times, got %d calls", mock.processCalled)
	}
	
	// Verify the result structure
	var output map[string]interface{}
	if err := result.Decode(&output); err != nil {
		t.Fatalf("Failed to decode result: %v", err)
	}
	
	// Check name
	if name, ok := output["name"].(string); !ok || name != "TEST" {
		t.Errorf("Expected name='TEST', got %v", output["name"])
	}
}

func TestWalk_MultipleProcessorsOnSameValue(t *testing.T) {
	ctx := context.Background()
	cueCtx := cuecontext.New()
	registry := NewRegistry()
	
	// Register two different processors
	proc1 := &mockProcessor{
		name: "first",
		processFunc: func(_ context.Context, _ Attribute) (cue.Value, error) {
			return cueCtx.CompileString(`"first"`), nil
		},
	}
	
	proc2 := &mockProcessor{
		name: "second",
		processFunc: func(_ context.Context, _ Attribute) (cue.Value, error) {
			return cueCtx.CompileString(`"second"`), nil
		},
	}
	
	if err := registry.Register(proc1); err != nil {
		t.Fatalf("Failed to register processor 1: %v", err)
	}
	if err := registry.Register(proc2); err != nil {
		t.Fatalf("Failed to register processor 2: %v", err)
	}
	
	walker := NewWalker(registry, cueCtx)
	
	// Value with multiple attributes on a field
	value := cueCtx.CompileString(`{value: "original" @first() @second()}`)
	
	result, err := walker.Walk(ctx, value)
	if err != nil {
		t.Fatalf("Walk failed: %v", err)
	}
	
	// One of the processors should have been called
	// (The exact behavior depends on CUE's attribute ordering)
	totalCalls := proc1.processCalled + proc2.processCalled
	if totalCalls == 0 {
		t.Error("At least one processor should have been called")
	}
	
	// Result should be from one of the processors
	resultValue := result.LookupPath(cue.ParsePath("value"))
	var str string
	if err := resultValue.Decode(&str); err != nil {
		t.Fatalf("Failed to decode result: %v", err)
	}
	if str != "first" && str != "second" {
		t.Errorf("Expected 'first' or 'second', got %q", str)
	}
}

func TestWalk_ErrorInNestedStruct_ContinuesWalking(t *testing.T) {
	ctx := context.Background()
	cueCtx := cuecontext.New()
	registry := NewRegistry()
	
	// Create a processor that fails for certain values
	mock := &mockProcessor{
		name: "conditional",
		processFunc: func(_ context.Context, attr Attribute) (cue.Value, error) {
			var str string
			if err := attr.Value.Decode(&str); err != nil {
				return cue.Value{}, fmt.Errorf("decode failed: %w", err)
			}
			
			if str == "fail" {
				return cue.Value{}, fmt.Errorf("intentional failure")
			}
			
			return cueCtx.CompileString(`"success"`), nil
		},
	}
	
	if err := registry.Register(mock); err != nil {
		t.Fatalf("Failed to register processor: %v", err)
	}
	
	walker := NewWalker(registry, cueCtx)
	
	// Struct with one failing and one succeeding value
	value := cueCtx.CompileString(`{
		good: "ok" @conditional()
		bad: "fail" @conditional()
		alsogood: "ok" @conditional()
	}`)
	
	result, err := walker.Walk(ctx, value)
	if err != nil {
		t.Fatalf("Walk should not return error, got: %v", err)
	}
	
	// Should have been called 3 times
	if mock.processCalled != 3 {
		t.Errorf("Expected processor to be called 3 times, got %d calls", mock.processCalled)
	}
	
	// The result should have errors in the "bad" field but continue
	// We can check that the overall result has partial success
	goodVal := result.LookupPath(cue.ParsePath("good"))
	if goodVal.Err() != nil {
		t.Error("'good' field should not have error")
	}
	
	// Note: The "bad" field will have an error value
	badVal := result.LookupPath(cue.ParsePath("bad"))
	if badVal.Err() == nil {
		t.Error("'bad' field should have error")
	}
}
