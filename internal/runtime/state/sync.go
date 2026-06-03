package state

import (
	"time"

	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// SyncTranscriptMessages persists the current canonical message list.
// It appends only the new suffix when the previous messages are a prefix of the
// current messages, and falls back to a full replace when the runtime rewrote
// history.
func (s *Store) SyncTranscriptMessages(
	sessionID types.SessionID,
	previousMessages []types.Message,
	currentMessages []types.Message,
) error {
	if len(currentMessages) == 0 {
		return s.ReplaceTranscript(sessionID, nil)
	}

	if hasMessagePrefix(currentMessages, previousMessages) {
		delta := currentMessages[len(previousMessages):]
		return s.AppendTranscriptEntries(sessionID, types.TranscriptEntriesFromMessages(delta, ""))
	}

	return s.ReplaceTranscript(sessionID, types.TranscriptEntriesFromMessages(currentMessages, ""))
}

// hasMessagePrefix returns true only when the previous in-memory transcript is a
// canonical prefix of the current one.
func hasMessagePrefix(messages []types.Message, prefix []types.Message) bool {
	return types.CanonicalTranscriptPrefix(messages, prefix)
}

func canonicalTranscriptEntries(messages []types.Message) []types.TranscriptEntry {
	return types.CanonicalTranscriptEntriesFromMessages(messages, "")
}

func canonicalMessages(entries []types.TranscriptEntry) []types.Message {
	return types.CanonicalMessagesFromTranscriptEntries(entries)
}

func canonicalTranscriptMessages(messages []types.Message) []types.Message {
	return types.CanonicalTranscriptMessages(messages)
}

func lastCompactionMetadata(messages []types.Message) *types.CompactionMetadata {
	return types.CompactionMetadataFromMessages(messages)
}

func countCompactionMessages(messages []types.Message) int {
	return types.CountCompactionMessages(messages)
}

func countCanonicalTranscriptMessages(messages []types.Message) int {
	return types.CanonicalTranscriptMessageCount(messages)
}

func countCanonicalTranscriptTurns(messages []types.Message) int {
	return types.CountDistinctTurnIDs(canonicalTranscriptMessages(messages))
}

func countCanonicalTranscriptToolResults(messages []types.Message) int {
	return types.CountToolResultMessages(canonicalTranscriptMessages(messages))
}

// canonicalTranscriptSummary derives the metadata snapshot stored alongside a
// persisted session so restore/list operations can inspect transcript shape
// without reparsing the whole history at each call site.
func canonicalTranscriptSummary(messages []types.Message) map[string]any {
	summary := map[string]any{
		"message_count": countCanonicalTranscriptMessages(messages),
		"turn_count":    countCanonicalTranscriptTurns(messages),
		"tool_results":  countCanonicalTranscriptToolResults(messages),
	}
	if compaction := lastCompactionMetadata(messages); compaction != nil {
		summary["last_compaction_kind"] = compaction.Kind
		summary["last_compaction_target_tokens"] = compaction.TargetTokens
		if compaction.BoundaryVersion != 0 {
			summary["last_compaction_boundary_version"] = compaction.BoundaryVersion
		}
		if compaction.FirstPreservedMessageID != "" {
			summary["last_compaction_first_preserved_message_id"] = compaction.FirstPreservedMessageID.String()
		}
		if compaction.LastPreservedMessageID != "" {
			summary["last_compaction_last_preserved_message_id"] = compaction.LastPreservedMessageID.String()
		}
		if compaction.PreservedTailHash != "" {
			summary["last_compaction_preserved_tail_hash"] = compaction.PreservedTailHash
		}
	}
	if count := countCompactionMessages(messages); count > 0 {
		summary["compaction_message_count"] = count
	}
	return summary
}

func applyCanonicalTranscriptSummary(metadata *types.SessionMetadata, messages []types.Message) {
	if metadata == nil {
		return
	}
	if metadata.Additional == nil {
		metadata.Additional = make(map[string]any)
	}
	metadata.Additional["canonical_transcript"] = canonicalTranscriptSummary(messages)
	metadata.CompactCount = countCompactionMessages(messages)
	if metadata.CompactCount > 0 {
		now := time.Now().UTC()
		metadata.LastCompactedAt = &now
	} else {
		metadata.LastCompactedAt = nil
	}
}
