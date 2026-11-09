package facet

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ironsweet/hookfacet"
	"github.com/ironsweet/judge"
	"github.com/stretchr/testify/assert"
)

type fakeJudge struct {
	intent     judge.MergedIntent
	evaluation judge.Result

	mergeCalls []mergeCall
	evalCalls  []judge.Input
}

type mergeCall struct {
	prevIntent string
	newMessage string
}

func (j *fakeJudge) MergeIntent(_ context.Context, prevIntent, newMessage string) (judge.MergedIntent, error) {
	j.mergeCalls = append(j.mergeCalls, mergeCall{prevIntent: prevIntent, newMessage: newMessage})
	return j.intent, nil
}

func (j *fakeJudge) Evaluate(_ context.Context, input judge.Input) (judge.Result, error) {
	j.evalCalls = append(j.evalCalls, input)
	return j.evaluation, nil
}

var _ judge.Interface = (*fakeJudge)(nil)

type fakePersister struct {
	savedState  *judge.SessionState
	saveErr     error
	loadedState *judge.SessionState
	loadErr     error
	loadCalls   []string
}

func (p *fakePersister) Load(sessionID string) (*judge.SessionState, error) {
	p.loadCalls = append(p.loadCalls, sessionID)
	if p.loadErr != nil {
		return nil, p.loadErr
	}
	if p.loadedState != nil {
		return p.loadedState, nil
	}
	return &judge.SessionState{SessionID: sessionID}, nil
}

func (p *fakePersister) Save(state *judge.SessionState) error {
	if p.saveErr != nil {
		return p.saveErr
	}
	b, _ := json.Marshal(state)
	var snapshot judge.SessionState
	_ = json.Unmarshal(b, &snapshot)
	p.savedState = &snapshot
	return nil
}

var _ judge.Persister = (*fakePersister)(nil)

func TestHandler_OnUserInput(t *testing.T) {
	fj := &fakeJudge{intent: judge.MergedIntent{Content: "do X"}}
	fp := &fakePersister{}
	h := New(fj, fp)
	ctx := t.Context()
	st := hookfacet.NewSessionState()
	d, err := h.OnEvent(ctx, hookfacet.Event{
		SessionID: "s1",
		Kind:      hookfacet.KindUserInput,
		Payload: hookfacet.Payload{
			Prompt: "do X",
		},
	}, st)
	assert.NoError(t, err)
	assert.False(t, d.Block)
	assert.NotNil(t, fp.savedState)
	assert.Equal(t, "do X", fp.savedState.Intent)

	d, err = h.OnEvent(ctx, hookfacet.Event{
		SessionID: "s1",
		Kind:      hookfacet.KindUserInput,
		Payload: hookfacet.Payload{
			Prompt: "do Y",
		},
	}, st)
	assert.NoError(t, err)
	assert.False(t, d.Block)
	assert.Len(t, fj.mergeCalls, 2)
	assert.Equal(t, "do Y", fj.mergeCalls[1].newMessage)
	assert.Len(t, fp.loadCalls, 1)
}

func TestHandler_OnToolCall(t *testing.T) {
	fj := &fakeJudge{}
	fp := &fakePersister{}
	h := New(fj, fp)
	ctx := t.Context()
	st := hookfacet.NewSessionState()
	d, err := h.OnEvent(ctx, hookfacet.Event{
		SessionID: "s1",
		Kind:      hookfacet.KindToolResult,
		Payload: hookfacet.Payload{
			ToolName:     "toolA",
			ToolInput:    json.RawMessage(`{"param": 1}`),
			ToolResponse: json.RawMessage(`{"result": "ok"}`),
		},
	}, st)
	assert.NoError(t, err)
	assert.False(t, d.Block)
	assert.NotNil(t, fp.savedState)
	assert.Len(t, fp.savedState.ToolCalls, 1)
	assert.Equal(t, "toolA", fp.savedState.ToolCalls[0].Name)
	assert.JSONEq(t, `{"param":1}`, string(fp.savedState.ToolCalls[0].Input))
	assert.JSONEq(t, `{"result":"ok"}`, string(fp.savedState.ToolCalls[0].Output))
	assert.Len(t, fp.loadCalls, 1)
}

func TestHandler_OnTurnEnd_WithToolCall(t *testing.T) {
	fj := &fakeJudge{evaluation: judge.Result{Satisfied: false, Reason: "issue"}}
	fp := &fakePersister{}
	h := New(fj, fp)
	ctx := t.Context()
	st := hookfacet.NewSessionState()
	hookfacet.Set[*judge.Session](st, judge.NewSession(&judge.SessionState{
		SessionID: "s1",
		Intent:    "do X",
		ToolCalls: []judge.ToolCall{{
			Name:   "toolA",
			Input:  json.RawMessage(`{"param": 1}`),
			Output: json.RawMessage(`{"result": "ok"}`),
		}},
	}, fj, fp))
	d, err := h.OnEvent(ctx, hookfacet.Event{
		SessionID: "s1",
		Kind:      hookfacet.KindTurnEnd,
		Payload: hookfacet.Payload{
			LastAssistantMessage: "agent summary",
		},
	}, st)
	assert.NoError(t, err)
	assert.True(t, d.Block)
	assert.Equal(t, "issue", d.Reason)
	assert.Len(t, fp.loadCalls, 0)
}

func TestHandler_OnTurnEnd_WithoutToolCall(t *testing.T) {
	fj := &fakeJudge{evaluation: judge.Result{Satisfied: false, Reason: "issue"}}
	fp := &fakePersister{}
	h := New(fj, fp)
	ctx := t.Context()
	st := hookfacet.NewSessionState()
	hookfacet.Set[*judge.Session](st, judge.NewSession(&judge.SessionState{
		SessionID: "s1",
		Intent:    "do X",
	}, fj, fp))
	d, err := h.OnEvent(ctx, hookfacet.Event{
		SessionID: "s1",
		Kind:      hookfacet.KindTurnEnd,
		Payload: hookfacet.Payload{
			LastAssistantMessage: "agent summary",
		},
	}, st)
	assert.NoError(t, err)
	assert.False(t, d.Block)
	assert.Len(t, fj.evalCalls, 0)
	assert.Len(t, fp.loadCalls, 0)
}
