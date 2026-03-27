package daemon

import (
	"errors"
	"strings"

	"control/internal/apicode"
)

type codexFileSearchErrorMapper interface {
	Map(err error) error
}

type defaultCodexFileSearchErrorMapper struct{}

func codexFileSearchErrorMapperOrDefault(mapper codexFileSearchErrorMapper) codexFileSearchErrorMapper {
	if mapper != nil {
		return mapper
	}
	return defaultCodexFileSearchErrorMapper{}
}

func (defaultCodexFileSearchErrorMapper) Map(err error) error {
	if err == nil {
		return nil
	}
	var serviceErr *ServiceError
	if errors.As(err, &serviceErr) {
		return err
	}
	var rpcErr *codexRPCError
	if errors.As(err, &rpcErr) && rpcErr != nil {
		switch rpcErr.Code {
		case -32601, -32602:
			return unavailableErrorWithCode("file search endpoint is not available for this provider runtime", apicode.ErrorCodeFileSearchUnsupported, err)
		}
		message := strings.ToLower(strings.TrimSpace(rpcErr.Message))
		if strings.Contains(message, "experimental") || strings.Contains(message, "fuzzyfilesearch") {
			return unavailableErrorWithCode("file search endpoint is not available for this provider runtime", apicode.ErrorCodeFileSearchUnsupported, err)
		}
	}
	return unavailableError("file search failed", err)
}
