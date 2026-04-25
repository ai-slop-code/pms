package permissions

const (
	PropertySettings = "property_settings"
	Occupancy        = "occupancy"
	NukiAccess       = "nuki_access"
	CleaningLog      = "cleaning_log"
	Finance          = "finance"
	Invoices         = "invoices"
	Messages         = "messages"
	Analytics        = "analytics"
	UsersPermissions = "users_permissions"
)

const (
	LevelRead   = "read"
	LevelWrite  = "write"
	LevelAdmin  = "admin"
)

func LevelRank(level string) int {
	switch level {
	case LevelRead:
		return 1
	case LevelWrite:
		return 2
	case LevelAdmin:
		return 3
	default:
		return 0
	}
}

func Meets(required, actual string) bool {
	return LevelRank(actual) >= LevelRank(required)
}
