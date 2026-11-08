package judge

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

type fakeJudge struct {
	intent     MergedIntent
	evaluation Result

	mergeCalls []mergeCall
	evalCalls  []Input
}

type mergeCall struct {
	prevIntent string
	newMessage string
}

func (j *fakeJudge) MergeIntent(_ context.Context, prevIntent, newMessage string) (MergedIntent, error) {
	j.mergeCalls = append(j.mergeCalls, mergeCall{prevIntent: prevIntent, newMessage: newMessage})
	return j.intent, nil
}

func (j *fakeJudge) Evaluate(_ context.Context, input Input) (Result, error) {
	j.evalCalls = append(j.evalCalls, input)
	return j.evaluation, nil
}

type fakePersister struct {
	savedState  *SessionState
	loadedState *SessionState
	loadErr     error
	loadCalls   []string
}

func (p *fakePersister) Load(sessionID string) (*SessionState, error) {
	p.loadCalls = append(p.loadCalls, sessionID)
	if p.loadErr != nil {
		return nil, p.loadErr
	}
	if p.loadedState != nil {
		return p.loadedState, nil
	}
	return &SessionState{SessionID: sessionID}, nil
}

func (p *fakePersister) Save(state *SessionState) error {
	b, _ := json.Marshal(state)
	var snapshot SessionState
	_ = json.Unmarshal(b, &snapshot)
	p.savedState = &snapshot
	return nil
}

func TestSession_OnUserInput(t *testing.T) {
	ctx := t.Context()
	fj := &fakeJudge{intent: MergedIntent{Content: "do X"}}
	fp := &fakePersister{}
	s := NewSession(&SessionState{}, fj, fp)
	assert.NoError(t, s.OnUserInput(ctx, "do X"))
	assert.Equal(t, "do X", s.state.Intent)
	assert.Equal(t, "do X", fp.savedState.Intent)
	assert.False(t, fp.savedState.LastUpdated.IsZero())
	assert.Len(t, fp.loadCalls, 0, "NewSession should not call Load")
}

func TestSession_OnUserInput2(t *testing.T) {
	ctx := t.Context()
	fj := &fakeJudge{intent: MergedIntent{Content: "do X and Y"}}
	fp := &fakePersister{}
	s := NewSession(&SessionState{Intent: "do X"}, fj, fp)
	assert.NoError(t, s.OnUserInput(ctx, "also Y"))
	assert.Equal(t, 1, len(fj.mergeCalls))
	assert.Equal(t, "do X", fj.mergeCalls[0].prevIntent)
	assert.Equal(t, "also Y", fj.mergeCalls[0].newMessage)
	assert.Equal(t, "do X and Y", s.state.Intent)
	assert.Equal(t, "do X and Y", fp.savedState.Intent)
	assert.False(t, fp.savedState.LastUpdated.IsZero())
	assert.Len(t, fp.loadCalls, 0, "NewSession should not call Load")
}

func TestSession_OnUserInput_SkipSystemTag(t *testing.T) {
	ctx := t.Context()
	fj := &fakeJudge{}
	s := NewSession(&SessionState{}, fj, &fakePersister{})
	assert.NoError(t, s.OnUserInput(ctx, "<system-reminder>do X</system-reminder>"))
	assert.Len(t, fj.mergeCalls, 0)

	fj = &fakeJudge{intent: MergedIntent{Content: "do X"}}
	s = NewSession(&SessionState{}, fj, &fakePersister{})
	assert.NoError(t, s.OnUserInput(ctx, "do X<system-reminder>do X</system-reminder>"))
	assert.Len(t, fj.mergeCalls, 1)
	assert.Equal(t, "do X", s.state.Intent)
	assert.Equal(t, "do X", fj.mergeCalls[0].newMessage, "system tag should be stripped")
}

func TestSession_OnToolCall(t *testing.T) {
	ctx := t.Context()
	fp := &fakePersister{}
	s := NewSession(&SessionState{}, &fakeJudge{}, fp)
	assert.NoError(t, s.OnToolCall(ctx, ToolCall{Name: "toolA", Input: []byte(`{"param":"value"}`)}))
	assert.Equal(t, 1, len(s.state.ToolCalls))
	assert.Equal(t, "toolA", s.state.ToolCalls[0].Name)
	assert.JSONEq(t, `{"param":"value"}`, string(s.state.ToolCalls[0].Input))
	assert.Equal(t, fp.savedState.SessionID, s.state.SessionID)
	assert.Equal(t, fp.savedState.Intent, s.state.Intent)
	assert.Len(t, fp.savedState.ToolCalls, 1)
	assert.Equal(t, fp.savedState.ToolCalls[0].Name, s.state.ToolCalls[0].Name)
	assert.JSONEq(t, string(fp.savedState.ToolCalls[0].Input), string(s.state.ToolCalls[0].Input))
}

func TestSession_OnTurnEnd_WithoutToolCall(t *testing.T) {
	ctx := t.Context()
	fj := &fakeJudge{evaluation: Result{Satisfied: true}}
	fp := &fakePersister{}
	s := NewSession(&SessionState{Intent: "do X"}, fj, fp)
	result, err := s.OnTurnEnd(ctx, "agent response")
	assert.NoError(t, err)
	assert.Equal(t, Result{Satisfied: true}, result)
	assert.Len(t, fj.evalCalls, 0)
	assert.NotNil(t, fp.savedState)
}

func TestSession_OnTurnEnd_WithToolCall(t *testing.T) {
	ctx := t.Context()
	fj := &fakeJudge{evaluation: Result{Satisfied: false, Reason: "issue found"}}
	fp := &fakePersister{}
	s := NewSession(&SessionState{
		Intent: "do X",
		ToolCalls: []ToolCall{
			{Name: "toolA", Input: []byte(`{"param":1}`)},
			{Name: "toolB", Input: []byte(`{"param":2}`)},
		},
	}, fj, fp)
	result, err := s.OnTurnEnd(ctx, "agent response")
	assert.NoError(t, err)
	assert.Equal(t, Result{Satisfied: false, Reason: "issue found"}, result)
	assert.NotNil(t, fp.savedState)
	assert.Nil(t, fp.savedState.ToolCalls) // ToolCalls should be cleared after turn end
	assert.NotEmpty(t, fp.savedState.Intent)
	assert.Len(t, fp.loadCalls, 0, "NewSession should not call Load")
	assert.Equal(t, 1, len(fj.evalCalls))
	assert.Equal(t, "do X", fj.evalCalls[0].UserIntent)
	assert.Equal(t, "agent response", fj.evalCalls[0].AgentResponse)
	assert.Len(t, fj.evalCalls[0].ToolCalls, 2)
}

func TestSession_LoadSession(t *testing.T) {
	fp := &fakePersister{loadedState: &SessionState{SessionID: "s1", Intent: "previous intent"}}
	s := LoadSession("s1", &fakeJudge{}, fp)
	assert.Equal(t, "previous intent", s.state.Intent)
	assert.Equal(t, []string{"s1"}, fp.loadCalls)
	assert.Equal(t, "s1", s.state.SessionID)
}

func TestSession_LoadSession_Fails(t *testing.T) {
	fp := &fakePersister{loadErr: assert.AnError}
	s := LoadSession("s1", &fakeJudge{}, fp)
	assert.Empty(t, s.state.Intent)
	assert.Equal(t, []string{"s1"}, fp.loadCalls)
	assert.Equal(t, "s1", s.state.SessionID)
}
