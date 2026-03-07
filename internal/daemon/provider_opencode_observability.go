package daemon

import (
	"context"
	"errors"
	"io"
	"net"
	"strings"

	"control/internal/logging"
	"control/internal/types"
)

func openCodeSessionLogFields(session *types.Session, meta *types.SessionMeta) []logging.Field {
	fields := []logging.Field{}
	if session != nil {
		fields = append(fields,
			logging.F("session_id", session.ID),
			logging.F("provider", session.Provider),
			logging.F("cwd", strings.TrimSpace(session.Cwd)),
			logging.F("session_status", session.Status),
		)
	}
	providerSessionID := ""
	if meta != nil {
		providerSessionID = strings.TrimSpace(meta.ProviderSessionID)
	}
	fields = append(fields, logging.F("provider_session_id", providerSessionID))
	return fields
}

func openCodeErrorLogFields(err error) []logging.Field {
	if err == nil {
		return nil
	}
	fields := []logging.Field{
		logging.F("error", err),
	}
	errorKind := "unknown"
	timeout := false
	if errors.Is(err, io.EOF) {
		errorKind = "eof"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		errorKind = "deadline_exceeded"
		timeout = true
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		timeout = timeout || netErr.Timeout()
		fields = append(fields,
			logging.F("network_timeout", netErr.Timeout()),
		)
		if netErr.Timeout() {
			errorKind = "network_timeout"
		}
	}
	var reqErr *openCodeRequestError
	if errors.As(err, &reqErr) && reqErr != nil {
		errorKind = "request_error"
		fields = append(fields,
			logging.F("request_method", reqErr.Method),
			logging.F("request_path", reqErr.Path),
			logging.F("request_status", reqErr.StatusCode),
		)
	}
	fields = append(fields,
		logging.F("error_kind", errorKind),
		logging.F("timeout", timeout),
	)
	return fields
}
