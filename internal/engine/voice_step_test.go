package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/mule-ai/mule/internal/primitive"
	"github.com/mule-ai/mule/pkg/job"
)

// stubVoiceCall is a controllable VoiceCall for engine-level tests of
// the voice_call step: Run blocks until the test ends the call.
type stubVoiceCall struct {
	runDone chan struct{}
	killErr error
	output  map[string]interface{}
}

func newStubVoiceCall(output map[string]interface{}) *stubVoiceCall {
	return &stubVoiceCall{runDone: make(chan struct{}), output: output}
}

func (s *stubVoiceCall) Run() error {
	<-s.runDone
	if s.killErr != nil {
		return s.killErr
	}
	return nil
}
func (s *stubVoiceCall) Output() map[string]interface{} { return s.output }
func (s *stubVoiceCall) FeedIn(pcm []byte)              {}
func (s *stubVoiceCall) FeedText(text string)           {}
func (s *stubVoiceCall) Out() <-chan VoiceOutChunk      { return nil }
func (s *stubVoiceCall) MarkOutClosed()                 {}
func (s *stubVoiceCall) HangUp()                        { close(s.runDone) }
func (s *stubVoiceCall) Kill(err error) {
	if s.killErr == nil {
		s.killErr = err
	}
	close(s.runDone)
}

// stubVoiceExecutor implements VoiceExecutor and records calls.
type stubVoiceExecutor struct {
	calls     map[string]*stubVoiceCall
	started   []string
	forgotten []string
	startErr  error
}

func newStubVoiceExecutor() *stubVoiceExecutor {
	return &stubVoiceExecutor{calls: map[string]*stubVoiceCall{}}
}

func (e *stubVoiceExecutor) StartCall(_ context.Context, jobID string, stepConfig map[string]interface{}, _ map[string]interface{}) (VoiceCall, error) {
	if e.startErr != nil {
		return nil, e.startErr
	}
	call := newStubVoiceCall(map[string]interface{}{"status": "hung_up", "turns": 3})
	e.calls[jobID] = call
	e.started = append(e.started, jobID)
	return call, nil
}
func (e *stubVoiceExecutor) ForgetCall(jobID string) {
	delete(e.calls, jobID)
	e.forgotten = append(e.forgotten, jobID)
}

func newVoiceStepEngine(t *testing.T, voice VoiceExecutor) (*Engine, *MockJobStore, *MockPrimitiveStore) {
	store := &MockPrimitiveStore{
		Workflows: []*primitive.Workflow{{ID: "wf-voice", Name: "voice-loop", Description: "d"}},
		WorkflowSteps: []*primitive.WorkflowStep{
			{ID: "step-v1", WorkflowID: "wf-voice", StepType: "voice_call", StepOrder: 1,
				Config: map[string]interface{}{"forge_url": "http://f", "api_key": "k", "agent_id": "a"}},
		},
	}
	jobStore := &MockJobStore{Jobs: make(map[string]*job.Job)}
	eng := NewEngine(store, jobStore, nil, nil, Config{Workers: 1, Voice: voice})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	if err := eng.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(eng.Stop)
	return eng, jobStore, store
}

// waitJobStatus blocks until the job leaves QUEUED/RUNNING or times out.
func waitJobStatus(t *testing.T, js *MockJobStore, jobID string, timeout time.Duration) *job.Job {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		j, err := js.GetJob(jobID)
		if err == nil && j != nil && j.Status != job.StatusQueued && j.Status != job.StatusRunning {
			return j
		}
		time.Sleep(50 * time.Millisecond)
	}
	return nil
}

func TestVoiceCallStepCompletesOnHangUp(t *testing.T) {
	exec := newStubVoiceExecutor()
	eng, jobStore, _ := newVoiceStepEngine(t, exec)

	j, err := eng.SubmitJob(context.Background(), "wf-voice", map[string]interface{}{})
	assert.NoError(t, err)

	// The step's StartCall fired: the executor has the call under the
	// job id; end it to hang up.
	deadline := time.Now().Add(5 * time.Second)
	for len(exec.started) == 0 && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	call, ok := exec.calls[j.ID]
	assert.True(t, ok, "no call registered for the voice job")
	call.HangUp()

	finished := waitJobStatus(t, jobStore, j.ID, 10*time.Second)
	if finished == nil {
		t.Fatal("job did not reach a terminal state")
	}
	assert.Equal(t, job.StatusCompleted, finished.Status, "hang-up must complete the job: %v", finished.Status)
	assert.Equal(t, "hung_up", finished.OutputData["status"])
	assert.Equal(t, 3, finished.OutputData["turns"])
	assert.Equal(t, []string{j.ID}, exec.forgotten, "the engine must forget the call on completion")
}

func TestVoiceCallStepFailsWhenCallKilled(t *testing.T) {
	exec := newStubVoiceExecutor()
	eng, jobStore, _ := newVoiceStepEngine(t, exec)
	jobStore.Errors = map[string]string{}

	j, err := eng.SubmitJob(context.Background(), "wf-voice", map[string]interface{}{})
	assert.NoError(t, err)

	deadline := time.Now().Add(5 * time.Second)
	var call *stubVoiceCall
	for {
		if c, ok := exec.calls[j.ID]; ok {
			call = c
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("call never registered for the voice job")
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Simulate the orchestrator dying mid-call.
	killErr := errors.New("orchestrator crashed")
	call.Kill(killErr)

	finished := waitJobStatus(t, jobStore, j.ID, 10*time.Second)
	if finished == nil {
		t.Fatal("job did not reach a terminal state after the kill")
	}
	assert.Equal(t, job.StatusFailed, finished.Status, "killed call must fail the job: %v", finished.Status)
	assert.Contains(t, jobStore.Errors[j.ID], "orchestrator crashed",
		"job failure should carry the kill reason, got: %q", jobStore.Errors[j.ID])
	assert.Equal(t, []string{j.ID}, exec.forgotten, "the engine must forget the call on failure too")
}

func TestVoiceCallStepStartFailureFailsJob(t *testing.T) {
	exec := newStubVoiceExecutor()
	exec.startErr = errors.New("invalid voice_call config: forge_url is required")
	eng, jobStore, _ := newVoiceStepEngine(t, exec)
	jobStore.Errors = map[string]string{}

	j, err := eng.SubmitJob(context.Background(), "wf-voice", map[string]interface{}{})
	assert.NoError(t, err)
	finished := waitJobStatus(t, jobStore, j.ID, 10*time.Second)
	if finished == nil {
		t.Fatal("job did not reach a terminal state")
	}
	assert.Equal(t, job.StatusFailed, finished.Status)
	assert.Contains(t, jobStore.Errors[j.ID], "invalid voice_call config")
}

func TestVoiceCallStepWithoutExecutor(t *testing.T) {
	// An engine built without a voice executor must fail the step with
	// a clear message instead of panicking.
	store := &MockPrimitiveStore{
		Workflows: []*primitive.Workflow{{ID: "wf-v", Name: "v", Description: "d"}},
		WorkflowSteps: []*primitive.WorkflowStep{
			{ID: "s1", WorkflowID: "wf-v", StepType: "voice_call", StepOrder: 1},
		},
	}
	jobStore := &MockJobStore{Jobs: make(map[string]*job.Job)}
	eng := NewEngine(store, jobStore, nil, nil, Config{Workers: 1})
	_, err := eng.processStepWithWorkingDir(context.Background(), "j1", store.WorkflowSteps[0], map[string]interface{}{}, "")
	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "voice executor"), fmt.Sprintf("got: %v", err))
}
