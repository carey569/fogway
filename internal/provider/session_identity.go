package provider

// SessionIdentity is a subscription account's public identity, which a
// local session's records can name. Fogway reads no agent's sign-in, so
// it knows of none.
type SessionIdentity struct {
	Agent, AccountID, UserID, OrganizationID, User string
	// OfficialLogin describes this account's recorded login method, not the
	// route or credentials used for any particular historical request.
	OfficialLogin bool
}

// SessionIdentities are the identities of the accounts local sessions
// may have run under: none.
func SessionIdentities(codexDir string) []SessionIdentity { return nil }
