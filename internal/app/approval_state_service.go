package app

import "strings"

type ApprovalSessionState struct {
	Requests    []*ApprovalRequest
	Resolutions []*ApprovalResolution
}

type ApprovalStateService interface {
	SetRequests(sessionID string, state ApprovalSessionState, requests []*ApprovalRequest) ApprovalSessionState
	UpsertRequest(sessionID string, state ApprovalSessionState, request *ApprovalRequest) (ApprovalSessionState, bool)
	RemoveRequest(state ApprovalSessionState, requestID int) (ApprovalSessionState, bool)
	UpsertResolution(sessionID string, state ApprovalSessionState, resolution *ApprovalResolution) (ApprovalSessionState, bool)
	RemoveResolution(state ApprovalSessionState, requestID int) (ApprovalSessionState, bool)
	LatestRequest(requests []*ApprovalRequest) *ApprovalRequest
}

type defaultApprovalStateService struct{}

func NewDefaultApprovalStateService() ApprovalStateService {
	return defaultApprovalStateService{}
}

func (defaultApprovalStateService) SetRequests(sessionID string, state ApprovalSessionState, requests []*ApprovalRequest) ApprovalSessionState {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return state
	}
	normalized := normalizeApprovalRequests(requests)
	for i := range normalized {
		if normalized[i] == nil {
			continue
		}
		if strings.TrimSpace(normalized[i].SessionID) == "" {
			normalized[i].SessionID = sessionID
		}
	}
	resolutions := state.Resolutions
	for _, req := range normalized {
		if req == nil {
			continue
		}
		updated, _ := removeApprovalResolution(resolutions, req.RequestID)
		resolutions = updated
	}
	return ApprovalSessionState{
		Requests:    normalized,
		Resolutions: resolutions,
	}
}

func (defaultApprovalStateService) UpsertRequest(sessionID string, state ApprovalSessionState, request *ApprovalRequest) (ApprovalSessionState, bool) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return state, false
	}
	if request != nil && strings.TrimSpace(request.SessionID) == "" {
		copy := *request
		copy.SessionID = sessionID
		request = &copy
	}
	updatedRequests, changed := upsertApprovalRequest(state.Requests, request)
	updatedResolutions := state.Resolutions
	if request != nil {
		nextResolutions, removed := removeApprovalResolution(updatedResolutions, request.RequestID)
		updatedResolutions = nextResolutions
		if removed {
			changed = true
		}
	}
	return ApprovalSessionState{
		Requests:    updatedRequests,
		Resolutions: updatedResolutions,
	}, changed
}

func (defaultApprovalStateService) RemoveRequest(state ApprovalSessionState, requestID int) (ApprovalSessionState, bool) {
	updated, changed := removeApprovalRequest(state.Requests, requestID)
	state.Requests = updated
	return state, changed
}

func (defaultApprovalStateService) UpsertResolution(sessionID string, state ApprovalSessionState, resolution *ApprovalResolution) (ApprovalSessionState, bool) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return state, false
	}
	if resolution != nil && strings.TrimSpace(resolution.SessionID) == "" {
		copy := *resolution
		copy.SessionID = sessionID
		resolution = &copy
	}
	updated, changed := upsertApprovalResolution(state.Resolutions, resolution)
	state.Resolutions = updated
	return state, changed
}

func (defaultApprovalStateService) RemoveResolution(state ApprovalSessionState, requestID int) (ApprovalSessionState, bool) {
	updated, changed := removeApprovalResolution(state.Resolutions, requestID)
	state.Resolutions = updated
	return state, changed
}

func (defaultApprovalStateService) LatestRequest(requests []*ApprovalRequest) *ApprovalRequest {
	return latestApprovalRequest(requests)
}

func WithApprovalStateService(service ApprovalStateService) ModelOption {
	return func(m *Model) {
		if m == nil {
			return
		}
		m.approvalStateService = service
	}
}

func (m *Model) approvalStateServiceOrDefault() ApprovalStateService {
	if m == nil || m.approvalStateService == nil {
		return NewDefaultApprovalStateService()
	}
	return m.approvalStateService
}
