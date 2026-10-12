package service

// SetNoticesForTest swaps the site-notification sender, which production wires
// from the notification use case.
func (s *Service) SetNoticesForTest(n UserNotifier) { s.notices = n }
