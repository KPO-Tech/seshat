package message

import (
	"context"

	"github.com/EngineerProjects/nexus-engine/internal/nexustui/pubsub"
)

// CreateMessageParams are the parameters for creating a new message.
type CreateMessageParams struct {
	Role             MessageRole
	Parts            []ContentPart
	Model            string
	Provider         string
	IsSummaryMessage bool
}

// Service is the public interface to the message store.
// The actual implementation is provided by the workspace adapter.
type Service interface {
	pubsub.Subscriber[Message]
	Create(ctx context.Context, sessionID string, params CreateMessageParams) (Message, error)
	Update(ctx context.Context, message Message) error
	Get(ctx context.Context, id string) (Message, error)
	List(ctx context.Context, sessionID string) ([]Message, error)
	ListUserMessages(ctx context.Context, sessionID string) ([]Message, error)
	ListAllUserMessages(ctx context.Context) ([]Message, error)
	Delete(ctx context.Context, id string) error
	DeleteSessionMessages(ctx context.Context, sessionID string) error
	Flush(ctx context.Context, id string) error
	FlushAll(ctx context.Context) error
}
