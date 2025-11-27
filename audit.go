package agendadistribuida

import (
	"context"
	"encoding/json"
	"sync"
	"time"
)

// AuditLevel represents the severity recorded in the audit table.
type AuditLevel string

const (
	AuditLevelInfo  AuditLevel = "info"
	AuditLevelWarn  AuditLevel = "warn"
	AuditLevelError AuditLevel = "error"
)

var (
	auditRepoMu sync.RWMutex
	auditRepo   AuditRepository

	nodeMetaMu sync.RWMutex
	nodeID     string
)

// SetAuditRepository installs the repository that will store audit events.
func SetAuditRepository(repo AuditRepository) {
	auditRepoMu.Lock()
	defer auditRepoMu.Unlock()
	auditRepo = repo
}

// SetNodeMetadata stores the node identifier used in audit entries.
func SetNodeMetadata(id string) {
	nodeMetaMu.Lock()
	defer nodeMetaMu.Unlock()
	nodeID = id
}

func getNodeID() string {
	nodeMetaMu.RLock()
	defer nodeMetaMu.RUnlock()
	return nodeID
}

// RecordAudit persists a structured audit log and mirrors it to the structured logger.
func RecordAudit(ctx context.Context, level AuditLevel, component, action, message string, fields map[string]any) {
	auditRepoMu.RLock()
	repo := auditRepo
	auditRepoMu.RUnlock()
	if repo == nil {
		Logger().Debug("audit_disabled", "component", component, "action", action)
		return
	}

	if ctx == nil {
		ctx = context.Background()
	}
	ctx, reqID := WithRequestID(ctx)
	payload := ""
	if len(fields) > 0 {
		if data, err := json.Marshal(fields); err == nil {
			payload = string(data)
		}
	}

	entry := &AuditLog{
		Component:  component,
		Action:     action,
		Level:      string(level),
		Message:    message,
		Payload:    payload,
		RequestID:  reqID,
		NodeID:     getNodeID(),
		OccurredAt: time.Now(),
	}
	if actorID, ok := GetUserIDFromContext(ctx); ok {
		entry.ActorID = &actorID
	}
	if err := repo.AppendAudit(entry); err != nil {
		Logger().Warn("audit_append_failed", "err", err, "component", component, "action", action)
	}
	Logger().Info("audit", "component", component, "action", action, "level", level, "message", message, "request_id", reqID, "fields", fields)
}
