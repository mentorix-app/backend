package auth

import "time"

// SetLockTimeoutForTest shortens the family lock wait so a test does not sit through it.
func (s *Store) SetLockTimeoutForTest(d time.Duration) { s.lockTimeout = d }
