package auth

// OnIdentityMiss installs the seam that lets a test play a concurrent sign-in at the exact moment
// the race can happen.
func (s *Service) OnIdentityMiss(f func()) { s.onIdentityMiss = f }
