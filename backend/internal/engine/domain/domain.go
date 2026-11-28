package domain

import (
	"encoding/json"
	"errors"
	"slices"
	"time"
	"uuid"
)

var (
	ErrNotFound          = errors.New("not found")
	ErrInvalidTransition = errors.New("invalid status transition")
)

func NewID() string {
	return uuid.NewV7().String()
}

func Now() time.Time {
	return time.Now().UTC().Truncate(time.Millisecond)
}

type MergeMode string

const (
	MergePR    MergeMode = "pr"
	MergeLocal MergeMode = "local"
)

type CodeRemote string

const (
	CodeRemoteProject CodeRemote = "project"
	CodeRemoteTower   CodeRemote = "tower"
)

type Project struct {
	ID            string
	Name          string
	RemoteURL     string
	DefaultBranch string
	SetupScript   string
	RunScript     string
	SandboxImage  string
	MergeMode     MergeMode
	CodeRemote    CodeRemote
	CreatedAt     time.Time
}

type WorkspaceStatus string

const (
	WorkspaceCreated  WorkspaceStatus = "created"
	WorkspaceActive   WorkspaceStatus = "active"
	WorkspaceIdle     WorkspaceStatus = "idle"
	WorkspaceArchived WorkspaceStatus = "archived"
)

var transitions = map[WorkspaceStatus][]WorkspaceStatus{
	WorkspaceCreated: {WorkspaceActive, WorkspaceArchived},
	WorkspaceActive:  {WorkspaceIdle},
	WorkspaceIdle:    {WorkspaceActive, WorkspaceArchived},
}

func (s WorkspaceStatus) CanTransition(to WorkspaceStatus) bool {
	return slices.Contains(transitions[s], to)
}

type Workspace struct {
	ID        string
	ProjectID string
	Name      string
	Slug      string
	Status    WorkspaceStatus
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (w Workspace) Branch() string {
	return "hangar/" + w.Slug
}

type Chat struct {
	ID                string
	WorkspaceID       string
	Title             string
	ProviderSessionID string
	CreatedAt         time.Time
}

type Event struct {
	ID        string
	ChatID    string
	Seq       *int64
	Author    string
	MachineID string
	Type      string
	Payload   json.RawMessage
	CreatedAt time.Time
}
