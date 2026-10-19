package api

import "time"

// SetSeedFetchBudget shortens the seed deadline so the test doesn't wait 25s for a tracker that
// hangs. It returns the function that restores the previous value.
func SetSeedFetchBudget(d time.Duration) (restore func()) {
	old := seedFetchBudget
	seedFetchBudget = d
	return func() { seedFetchBudget = old }
}
