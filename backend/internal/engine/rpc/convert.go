package rpc

import (
	"encoding/json"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	v1 "github.com/dnlaglr/openhangar/backend/gen/hangar/v1"
	"github.com/dnlaglr/openhangar/backend/internal/engine/domain"
)

var (
	mergeModes = map[domain.MergeMode]v1.MergeMode{
		domain.MergePR:    v1.MergeMode_MERGE_MODE_PR,
		domain.MergeLocal: v1.MergeMode_MERGE_MODE_LOCAL,
	}
	codeRemotes = map[domain.CodeRemote]v1.CodeRemote{
		domain.CodeRemoteProject: v1.CodeRemote_CODE_REMOTE_PROJECT,
		domain.CodeRemoteTower:   v1.CodeRemote_CODE_REMOTE_TOWER,
	}
	workspaceStatuses = map[domain.WorkspaceStatus]v1.WorkspaceStatus{
		domain.WorkspaceCreated:  v1.WorkspaceStatus_WORKSPACE_STATUS_CREATED,
		domain.WorkspaceActive:   v1.WorkspaceStatus_WORKSPACE_STATUS_ACTIVE,
		domain.WorkspaceIdle:     v1.WorkspaceStatus_WORKSPACE_STATUS_IDLE,
		domain.WorkspaceArchived: v1.WorkspaceStatus_WORKSPACE_STATUS_ARCHIVED,
	}
	outcomes = map[domain.Outcome]v1.Outcome{
		domain.OutcomeCompleted:   v1.Outcome_OUTCOME_COMPLETED,
		domain.OutcomeInterrupted: v1.Outcome_OUTCOME_INTERRUPTED,
		domain.OutcomeFailed:      v1.Outcome_OUTCOME_FAILED,
	}
)

func reverse[K, V comparable](m map[K]V) map[V]K {
	r := make(map[V]K, len(m))
	for k, v := range m {
		r[v] = k
	}

	return r
}

var (
	mergeModesFromProto  = reverse(mergeModes)
	codeRemotesFromProto = reverse(codeRemotes)
)

func projectToProto(p domain.Project) *v1.Project {
	return &v1.Project{
		Id:            p.ID,
		Name:          p.Name,
		RemoteUrl:     p.RemoteURL,
		DefaultBranch: p.DefaultBranch,
		SetupScript:   p.SetupScript,
		RunScript:     p.RunScript,
		SandboxImage:  p.SandboxImage,
		MergeMode:     mergeModes[p.MergeMode],
		CodeRemote:    codeRemotes[p.CodeRemote],
		CreatedAt:     timestamppb.New(p.CreatedAt),
	}
}

func projectFromProto(r *v1.AddProjectRequest) domain.Project {
	return domain.Project{
		Name:          r.GetName(),
		RemoteURL:     r.GetRemoteUrl(),
		DefaultBranch: r.GetDefaultBranch(),
		SetupScript:   r.GetSetupScript(),
		RunScript:     r.GetRunScript(),
		SandboxImage:  r.GetSandboxImage(),
		MergeMode:     mergeModesFromProto[r.GetMergeMode()],
		CodeRemote:    codeRemotesFromProto[r.GetCodeRemote()],
	}
}

func workspaceToProto(w domain.Workspace) *v1.Workspace {
	return &v1.Workspace{
		Id:        w.ID,
		ProjectId: w.ProjectID,
		Name:      w.Name,
		Slug:      w.Slug,
		Branch:    w.Branch(),
		Status:    workspaceStatuses[w.Status],
		CreatedAt: timestamppb.New(w.CreatedAt),
		UpdatedAt: timestamppb.New(w.UpdatedAt),
	}
}

func agentToProto(a domain.Agent) *v1.Agent {
	return &v1.Agent{Provider: a.Provider, Model: a.Model, Effort: a.Effort}
}

func agentFromProto(a *v1.Agent) domain.Agent {
	return domain.Agent{Provider: a.GetProvider(), Model: a.GetModel(), Effort: a.GetEffort()}
}

func chatToProto(c domain.Chat) *v1.Chat {
	return &v1.Chat{
		Id:          c.ID,
		WorkspaceId: c.WorkspaceID,
		Title:       c.Title,
		Agent:       agentToProto(c.Agent),
		CreatedAt:   timestamppb.New(c.CreatedAt),
	}
}

func toProtos[T, P any](items []T, fn func(T) P) []P {
	out := make([]P, len(items))
	for i, item := range items {
		out[i] = fn(item)
	}

	return out
}

// eventToProto leaves the payload unset for event types this schema does not know, so newer events never break a stream.
func eventToProto(e domain.Event) (*v1.Event, error) {
	pe := &v1.Event{
		Id:        e.ID,
		ChatId:    e.ChatID,
		Seq:       e.Seq,
		Author:    e.Author,
		MachineId: e.MachineID,
		CreatedAt: timestamppb.New(e.CreatedAt),
	}

	var err error
	switch e.Type {
	case domain.EventUserMessage:
		var p domain.UserMessage
		err = json.Unmarshal(e.Payload, &p)
		pe.Payload = &v1.Event_UserMessage{UserMessage: &v1.UserMessage{Text: p.Text}}
	case domain.EventTurnStarted:
		var p domain.TurnStarted
		err = json.Unmarshal(e.Payload, &p)
		pe.Payload = &v1.Event_TurnStarted{TurnStarted: &v1.TurnStarted{TurnId: p.TurnID, MessageIds: p.MessageIDs, Agent: agentToProto(p.Agent)}}
	case domain.EventAgentText:
		var p domain.AgentText
		err = json.Unmarshal(e.Payload, &p)
		pe.Payload = &v1.Event_AgentText{AgentText: &v1.AgentText{TurnId: p.TurnID, Text: p.Text}}
	case domain.EventToolCall:
		var p domain.ToolCall
		err = json.Unmarshal(e.Payload, &p)
		pe.Payload = &v1.Event_ToolCall{ToolCall: &v1.ToolCall{
			TurnId:       p.TurnID,
			CallId:       p.CallID,
			ParentCallId: p.ParentCallID,
			Name:         p.Name,
			Input:        toolInput(p.Input),
		}}
	case domain.EventToolResult:
		var p domain.ToolResult
		err = json.Unmarshal(e.Payload, &p)
		pe.Payload = &v1.Event_ToolResult{ToolResult: &v1.ToolResult{TurnId: p.TurnID, CallId: p.CallID, Output: p.Output, IsError: p.IsError}}
	case domain.EventTurnEnded:
		var p domain.TurnEnded
		err = json.Unmarshal(e.Payload, &p)
		pe.Payload = &v1.Event_TurnEnded{TurnEnded: &v1.TurnEnded{
			TurnId:     p.TurnID,
			Outcome:    outcomes[p.Outcome],
			Error:      p.Error,
			DurationMs: p.DurationMS,
			Usage: &v1.Usage{
				InputTokens:      p.Usage.InputTokens,
				OutputTokens:     p.Usage.OutputTokens,
				CacheReadTokens:  p.Usage.CacheReadTokens,
				CacheWriteTokens: p.Usage.CacheWriteTokens,
			},
			CostUsd: p.CostUSD,
			Commit:  p.Commit,
		}}
	}

	return pe, err
}

// toolInput drops inputs that are not JSON objects, which tool inputs always are.
func toolInput(raw json.RawMessage) *structpb.Struct {
	s := &structpb.Struct{}
	if protojson.Unmarshal(raw, s) != nil {
		return nil
	}

	return s
}
