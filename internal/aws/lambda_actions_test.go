package aws_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/lambda"
	awspkg "github.com/bryanl/lazyaws/internal/aws"
)

// stubLambdaWrite extends stubLambda with write operations, satisfying LambdaActionsAPI.
type stubLambdaWrite struct {
	stubLambda
	invokeErr      error
	deleteFuncErr  error
	invokeCalled   bool
	deleteCalled   bool
}

func (s *stubLambdaWrite) Invoke(_ context.Context, _ *lambda.InvokeInput, _ ...func(*lambda.Options)) (*lambda.InvokeOutput, error) {
	s.invokeCalled = true
	return &lambda.InvokeOutput{}, s.invokeErr
}

func (s *stubLambdaWrite) DeleteFunction(_ context.Context, _ *lambda.DeleteFunctionInput, _ ...func(*lambda.Options)) (*lambda.DeleteFunctionOutput, error) {
	s.deleteCalled = true
	return &lambda.DeleteFunctionOutput{}, s.deleteFuncErr
}

// stubActionContext records ActionContext calls for assertions.
type stubActionContext struct {
	confirmed      bool
	confirmDeleted bool
	refreshed      bool
	errorShown     error
	onConfirm      func()
	errorCh        chan error // non-nil to receive ShowError calls asynchronously
}

func (s *stubActionContext) Confirm(_ string, onConfirm func()) {
	s.confirmed = true
	s.onConfirm = onConfirm
}

func (s *stubActionContext) ConfirmDelete(_ string, onConfirm func()) {
	s.confirmDeleted = true
	s.onConfirm = onConfirm
}

func (s *stubActionContext) PromptInput(_ string, _ string, _ func(string)) {}

func (s *stubActionContext) ShowError(err error) {
	s.errorShown = err
	if s.errorCh != nil {
		s.errorCh <- err
	}
}

func (s *stubActionContext) ShowInfo(_ string) {}

func (s *stubActionContext) Refresh() { s.refreshed = true }

func (s *stubActionContext) OpenMultiGroupPicker(_ func([]string)) {}

func TestLambdaProvider_Actions_noItem(t *testing.T) {
	p := awspkg.NewLambdaProviderWithClient(newStubLambda())
	actions := p.Actions(awspkg.Item{})
	if len(actions) != 0 {
		t.Errorf("expected no actions for empty item, got %d", len(actions))
	}
}

func TestLambdaProvider_Actions_readOnlyClient(t *testing.T) {
	// stubLambda does NOT implement LambdaActionsAPI (no write methods).
	p := awspkg.NewLambdaProviderWithClient(newStubLambda())
	actions := p.Actions(awspkg.Item{ID: "my-function", Name: "my-function"})
	if len(actions) != 0 {
		t.Errorf("expected no actions with read-only client, got %d", len(actions))
	}
}

func TestLambdaProvider_Actions_invoke(t *testing.T) {
	stub := &stubLambdaWrite{stubLambda: *newStubLambda()}
	p := awspkg.NewLambdaProviderWithClient(stub)
	item := awspkg.Item{ID: "my-function", Name: "my-function"}

	actions := p.Actions(item)
	if len(actions) != 2 {
		t.Fatalf("expected 2 actions, got %d", len(actions))
	}

	cases := []struct {
		name  string
		label string
		key   rune
	}{
		{"invoke", "Invoke function", 'i'},
		{"delete", "Delete function", 'd'},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if actions[i].Label != tc.label {
				t.Errorf("action[%d].Label = %q, want %q", i, actions[i].Label, tc.label)
			}
			if actions[i].Key != tc.key {
				t.Errorf("action[%d].Key = %q, want %q", i, actions[i].Key, tc.key)
			}
		})
	}
}

func TestLambdaProvider_Actions_invokeFunc_callsPromptInput(t *testing.T) {
	stub := &stubLambdaWrite{stubLambda: *newStubLambda()}
	p := awspkg.NewLambdaProviderWithClient(stub)
	item := awspkg.Item{ID: "my-function", Name: "my-function"}
	ac := &stubActionContext{}

	actions := p.Actions(item)
	if err := actions[0].Func(context.Background(), item, ac); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// PromptInput is a no-op in the stub; just verify no error and no Confirm call
	if ac.confirmed {
		t.Error("invoke should use PromptInput, not Confirm")
	}
}

func TestLambdaProvider_Actions_deleteFunc_callsConfirmDelete(t *testing.T) {
	stub := &stubLambdaWrite{stubLambda: *newStubLambda()}
	p := awspkg.NewLambdaProviderWithClient(stub)
	item := awspkg.Item{ID: "my-function", Name: "my-function"}
	ac := &stubActionContext{}

	actions := p.Actions(item)
	if err := actions[1].Func(context.Background(), item, ac); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ac.confirmDeleted {
		t.Error("expected ConfirmDelete to be called")
	}
}

func TestLambdaProvider_Actions_deleteFunc_sdkError(t *testing.T) {
	sdkErr := errors.New("function not found")
	stub := &stubLambdaWrite{stubLambda: *newStubLambda(), deleteFuncErr: sdkErr}
	p := awspkg.NewLambdaProviderWithClient(stub)
	item := awspkg.Item{ID: "my-function", Name: "my-function"}
	ac := &stubActionContext{errorCh: make(chan error, 1)}

	actions := p.Actions(item)
	actions[1].Func(context.Background(), item, ac) //nolint:errcheck

	// Simulate user confirming deletion — onConfirm spawns a goroutine internally.
	if ac.onConfirm != nil {
		ac.onConfirm()
	}

	// Wait for the inner goroutine to call ShowError.
	select {
	case err := <-ac.errorCh:
		if !errors.Is(err, sdkErr) {
			t.Errorf("expected errorShown=%v, got %v", sdkErr, err)
		}
	case <-time.After(time.Second):
		t.Error("timed out waiting for ShowError to be called")
	}
}
