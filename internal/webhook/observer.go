package webhook

type Observer interface {
	OnUserSignedIn(payload LogtoPayload) error
}
