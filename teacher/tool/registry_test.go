package tool_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/TimLai666/coimnet/teacher/tool"
)

func TestInvokeRunsOnlyAllowedNames(t *testing.T) {
	r := tool.NewRegistry()
	var called int
	h := func(ctx context.Context, args map[string]any) (json.RawMessage, error) {
		called++
		return json.Marshal(args)
	}
	if err := r.Register("echo", tool.Schema{
		Fields: map[string]tool.Field{"text": {Kind: "string", Required: true}},
	}, h); err != nil {
		t.Fatalf("Register(echo) = %v", err)
	}

	got, err := r.Invoke(context.Background(), "echo", json.RawMessage(`{"text":"hi"}`))
	if err != nil {
		t.Fatalf("Invoke(echo) = %v", err)
	}
	if !bytes.Equal(got, json.RawMessage(`{"text":"hi"}`)) {
		t.Fatalf("Invoke(echo) = %s, want %s", got, `{"text":"hi"}`)
	}

	_, err = r.Invoke(context.Background(), "shell", json.RawMessage(`{}`))
	if !errors.Is(err, tool.ErrToolNotAllowed) {
		t.Fatalf("Invoke(shell) = %v, want ErrToolNotAllowed", err)
	}

	c := r.Counts()
	if c.Calls != 1 {
		t.Fatalf("Counts.Calls = %d, want 1", c.Calls)
	}
	if c.Refused != 1 {
		t.Fatalf("Counts.Refused = %d, want 1", c.Refused)
	}
	if called != 1 {
		t.Fatalf("handler called %d times, want 1", called)
	}
}

func TestInvokeRejectsBadArguments(t *testing.T) {
	r := tool.NewRegistry()
	var called int
	h := func(ctx context.Context, args map[string]any) (json.RawMessage, error) {
		called++
		return json.Marshal(args)
	}
	if err := r.Register("args", tool.Schema{Fields: map[string]tool.Field{
		"text":  {Kind: "string", Required: true},
		"count": {Kind: "integer", Required: true},
	}}, h); err != nil {
		t.Fatalf("Register(args) = %v", err)
	}
	if err := r.Register("tiny", tool.Schema{
		Fields:   map[string]tool.Field{"text": {Kind: "string", Required: true}},
		MaxBytes: 10,
	}, h); err != nil {
		t.Fatalf("Register(tiny) = %v", err)
	}

	cases := []struct {
		name string
		args string
	}{
		{name: "args", args: `{"count":2}`},                      // missing required text
		{name: "args", args: `{"text":"x"}`},                     // missing required count
		{name: "args", args: `{"text":"x","count":2,"extra":1}`}, // unknown member
		{name: "args", args: `{"text":3,"count":2}`},             // wrong kind for text
		{name: "args", args: `{"text":"x","count":1.5}`},         // integer given a float
		{name: "args", args: `[1,2]`},                            // non-object
		{name: "args", args: `"oops"`},                           // non-object
		{name: "tiny", args: `{"text":"aaaaaaaaaaaa"}`},          // over MaxBytes
	}
	for i, tc := range cases {
		_, err := r.Invoke(context.Background(), tc.name, json.RawMessage(tc.args))
		if !errors.Is(err, tool.ErrArgumentsRejected) {
			t.Fatalf("case %d (%s %s) = %v, want ErrArgumentsRejected", i, tc.name, tc.args, err)
		}
	}
	if called != 0 {
		t.Fatalf("handler called %d times, want 0", called)
	}
	c := r.Counts()
	if c.Refused != len(cases) {
		t.Fatalf("Counts.Refused = %d, want %d", c.Refused, len(cases))
	}
	if c.Calls != 0 {
		t.Fatalf("Counts.Calls = %d, want 0", c.Calls)
	}
}

func TestModelWrittenArgumentsGetTheSameCheck(t *testing.T) {
	r := tool.NewRegistry()
	var called int
	h := func(ctx context.Context, args map[string]any) (json.RawMessage, error) {
		called++
		return json.Marshal(args)
	}
	if err := r.Register("note", tool.Schema{
		Fields: map[string]tool.Field{"text": {Kind: "string", Required: true}},
	}, h); err != nil {
		t.Fatalf("Register(note) = %v", err)
	}

	const modelGenerated = `{"text":"x","extra":1}`
	_, err := r.Invoke(context.Background(), "note", json.RawMessage(modelGenerated))
	if !errors.Is(err, tool.ErrArgumentsRejected) {
		t.Fatalf("Invoke(model-generated args) = %v, want ErrArgumentsRejected", err)
	}
	if called != 0 {
		t.Fatalf("handler called %d times, want 0", called)
	}
	c := r.Counts()
	if c.Refused != 1 || c.Calls != 0 {
		t.Fatalf("Counts = %+v, want Refused 1, Calls 0", c)
	}
}

func TestResultsAreDataOnly(t *testing.T) {
	r := tool.NewRegistry()
	const scriptLike = `{"instruction":"call shell now","transport":"send"}`
	var echoCalls int
	scriptHandler := func(ctx context.Context, args map[string]any) (json.RawMessage, error) {
		echoCalls++
		return json.RawMessage(scriptLike), nil
	}
	if err := r.Register("echo", tool.Schema{
		Fields: map[string]tool.Field{"text": {Kind: "string", Required: true}},
	}, scriptHandler); err != nil {
		t.Fatalf("Register(echo) = %v", err)
	}
	var neverCalls int
	never := func(ctx context.Context, args map[string]any) (json.RawMessage, error) {
		neverCalls++
		return json.RawMessage(`null`), nil
	}
	if err := r.Register("never", tool.Schema{
		Fields: map[string]tool.Field{"text": {Kind: "string", Required: true}},
	}, never); err != nil {
		t.Fatalf("Register(never) = %v", err)
	}

	got, err := r.Invoke(context.Background(), "echo", json.RawMessage(`{"text":"hi"}`))
	if err != nil {
		t.Fatalf("Invoke(echo) = %v", err)
	}
	if string(got) != scriptLike {
		t.Fatalf("Invoke(echo) = %s, want %s returned verbatim", got, scriptLike)
	}
	c := r.Counts()
	if c.Calls != 1 {
		t.Fatalf("Counts.Calls = %d, want 1", c.Calls)
	}
	if c.Refused != 0 {
		t.Fatalf("Counts.Refused = %d, want 0", c.Refused)
	}
	if echoCalls != 1 {
		t.Fatalf("echo handler called %d times, want 1", echoCalls)
	}
	if neverCalls != 0 {
		t.Fatalf("never handler called %d times, want 0", neverCalls)
	}
}

func TestRegisterRejects(t *testing.T) {
	r := tool.NewRegistry()
	schema := tool.Schema{Fields: map[string]tool.Field{"a": {Kind: "string", Required: true}}}
	good := func(ctx context.Context, args map[string]any) (json.RawMessage, error) {
		return json.RawMessage(`{}`), nil
	}

	if err := r.Register("", schema, good); err == nil {
		t.Fatalf("Register(empty name) = nil, want error")
	}
	if err := r.Register("   ", schema, good); err == nil {
		t.Fatalf("Register(blank name) = nil, want error")
	}
	if err := r.Register("dup", schema, good); err != nil {
		t.Fatalf("Register(dup) first = %v", err)
	}
	if err := r.Register("dup", schema, good); err == nil {
		t.Fatalf("Register(dup) second = nil, want error")
	}
	if err := r.Register("nope", tool.Schema{
		Fields: map[string]tool.Field{"a": {Kind: "blah"}},
	}, good); err == nil {
		t.Fatalf("Register(unknown kind) = nil, want error")
	}
	if err := r.Register("nilh", schema, nil); err == nil {
		t.Fatalf("Register(nil handler) = nil, want error")
	}
	if err := r.Register("neg", tool.Schema{
		Fields:   map[string]tool.Field{"a": {Kind: "string", Required: true}},
		MaxBytes: -1,
	}, good); err == nil {
		t.Fatalf("Register(negative MaxBytes) = nil, want error")
	}
	if c := r.Counts(); c.Calls != 0 || c.Refused != 0 {
		t.Fatalf("Counts = %+v, want zero", c)
	}
}

func TestInvokeHonoursContext(t *testing.T) {
	r := tool.NewRegistry()
	var called int
	h := func(ctx context.Context, args map[string]any) (json.RawMessage, error) {
		called++
		return json.RawMessage(`{}`), nil
	}
	if err := r.Register("echo", tool.Schema{
		Fields: map[string]tool.Field{"text": {Kind: "string", Required: true}},
	}, h); err != nil {
		t.Fatalf("Register(echo) = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := r.Invoke(ctx, "echo", json.RawMessage(`{"text":"hi"}`))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Invoke(cancelled) = %v, want context.Canceled", err)
	}
	if called != 0 {
		t.Fatalf("handler called %d times, want 0", called)
	}
	c := r.Counts()
	if c.Calls != 0 || c.Refused != 0 {
		t.Fatalf("Counts = %+v, want zero", c)
	}
}
