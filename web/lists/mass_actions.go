package lists

// MassAction describes one bulk operation offered by a list.
type MassAction struct {
	Key    string
	Label  string
	Icon   string
	Danger bool
	URL    string
}

var massActionRegistry = map[string][]MassAction{}

// RegisterMassActions publishes the ordered actions offered by a list.
func RegisterMassActions(listID string, actions []MassAction) {
	seen := make(map[string]struct{}, len(actions))
	for _, action := range actions {
		if action.Key == "" || action.Label == "" || action.URL == "" {
			panic("lists: mass action key, label, and URL are required for list '" + listID + "'")
		}
		if _, exists := seen[action.Key]; exists {
			panic("lists: duplicate mass action key '" + action.Key + "' for list '" + listID + "'")
		}
		seen[action.Key] = struct{}{}
	}
	massActionRegistry[listID] = append([]MassAction(nil), actions...)
}

// MassActionsFor returns the ordered mass actions registered for a list.
func MassActionsFor(listID string) []MassAction {
	return append([]MassAction(nil), massActionRegistry[listID]...)
}

func init() {
	RegisterMassActions(ListIDAssets, []MassAction{
		{Key: "delete", Label: "Delete", Icon: "trash", Danger: true, URL: "/api/assets/bulk"},
		{Key: "restore", Label: "Restore", Icon: "undo", URL: "/api/assets/bulk"},
		{Key: "purge", Label: "Delete permanently", Icon: "trash", Danger: true, URL: "/api/assets/bulk"},
	})
	RegisterMassActions(ListIDIdentities, []MassAction{
		{Key: "delete", Label: "Delete", Icon: "trash", Danger: true, URL: "/api/users/bulk"},
		{Key: "restore", Label: "Restore", Icon: "undo", URL: "/api/users/bulk"},
		{Key: "purge", Label: "Delete permanently", Icon: "trash", Danger: true, URL: "/api/users/bulk"},
	})
	RegisterMassActions(ListIDGroups, []MassAction{
		{Key: "delete", Label: "Delete", Icon: "trash", Danger: true, URL: "/api/groups/bulk"},
		{Key: "restore", Label: "Restore", Icon: "undo", URL: "/api/groups/bulk"},
		{Key: "purge", Label: "Delete permanently", Icon: "trash", Danger: true, URL: "/api/groups/bulk"},
	})
	RegisterMassActions(ListIDConfigGroups, []MassAction{
		{Key: "delete", Label: "Delete", Icon: "trash", Danger: true, URL: "/api/config-groups/bulk"},
		{Key: "restore", Label: "Restore", Icon: "undo", URL: "/api/config-groups/bulk"},
		{Key: "purge", Label: "Delete permanently", Icon: "trash", Danger: true, URL: "/api/config-groups/bulk"},
	})
	RegisterMassActions(ListIDDependencyGroups, []MassAction{
		{Key: "delete", Label: "Delete", Icon: "trash", Danger: true, URL: "/api/dependency-groups/bulk"},
		{Key: "restore", Label: "Restore", Icon: "undo", URL: "/api/dependency-groups/bulk"},
		{Key: "purge", Label: "Delete permanently", Icon: "trash", Danger: true, URL: "/api/dependency-groups/bulk"},
	})
	RegisterMassActions(ListIDPolicyModules, []MassAction{
		{Key: "duplicate", Label: "Duplicate", Icon: "copy", URL: "/api/modules/bulk"},
		{Key: "revoke", Label: "Revoke versions", Icon: "ban", Danger: true, URL: "/api/modules/bulk"},
		{Key: "delete", Label: "Delete", Icon: "trash", Danger: true, URL: "/api/modules/bulk"},
		{Key: "restore", Label: "Restore", Icon: "undo", URL: "/api/modules/bulk"},
		{Key: "purge", Label: "Delete permanently", Icon: "trash", Danger: true, URL: "/api/modules/bulk"},
	})
}
