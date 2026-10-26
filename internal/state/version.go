package state

// MaxSchemaVersion reports the highest migration this binary can apply. A
// backup from a newer binary must not be restored over this one.
func MaxSchemaVersion() int {
	migs, err := loadMigrations()
	if err != nil || len(migs) == 0 {
		return 0
	}
	return migs[len(migs)-1].version
}
