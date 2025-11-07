package webhook

type Observer interface {
	OnUserSignedIn(user User) error
}
