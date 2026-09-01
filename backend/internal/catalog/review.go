package catalog

import "strings"

func EffectiveSyncState(status, syncState string) string {
	syncState = strings.TrimSpace(syncState)
	if syncState != "" {
		return syncState
	}
	switch strings.TrimSpace(status) {
	case SyncPublished, "deprecated":
		return SyncPublished
	default:
		return SyncDraft
	}
}

func FilterAdminModels(items []ModelView, status, syncState, q string) []ModelView {
	status = strings.TrimSpace(status)
	syncState = strings.TrimSpace(syncState)
	q = strings.ToLower(strings.TrimSpace(q))
	out := make([]ModelView, 0, len(items))
	for _, item := range items {
		if status != "" && !strings.EqualFold(item.Status, status) {
			continue
		}
		got := EffectiveSyncState(item.Status, item.SyncState)
		if syncState != "" && !strings.EqualFold(got, syncState) {
			continue
		}
		if q != "" {
			hay := strings.ToLower(item.ID + item.Vendor + item.DisplayName + item.Status + got + item.CreatedByUserID)
			if !strings.Contains(hay, q) {
				continue
			}
		}
		out = append(out, item)
	}
	return out
}

func creatorIsActor(createdBy, actor string) bool {
	createdBy = strings.TrimSpace(createdBy)
	actor = strings.TrimSpace(actor)
	return createdBy != "" && actor != "" && createdBy == actor
}
