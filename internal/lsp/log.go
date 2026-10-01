package lsp

import (
	"context"
	"fmt"
	"os"

	"go.lsp.dev/protocol"
)

// logf sends window/logMessage to the client; before a client is attached
// the line goes to stderr, which editors also collect.
func (s *Server) logf(ctx context.Context, kind protocol.MessageType, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	if client, ok := protocol.ClientFromContext(ctx); ok {
		client.LogMessage(ctx, &protocol.LogMessageParams{Type: kind, Message: msg})
		return
	}
	fmt.Fprintln(os.Stderr, msg)
}
