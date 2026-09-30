package gtasks

import "context"

// ResolveTaskList resolves the bridge's single fixed default task list,
// creating it on first use if it doesn't exist yet (see EnsureTaskList).
// Unlike gcal's split read-any/write-restricted model, every gtasks
// operation targets this one list — there is no per-request list picking.
func ResolveTaskList(ctx context.Context, api API) (string, error) {
	return api.EnsureTaskList(ctx, BotTaskListName)
}
