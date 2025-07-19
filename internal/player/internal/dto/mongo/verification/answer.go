package verificationdto

import "time"

type Answer struct {
	Id        string
	Question  string
	Answer    string
	CreatedAt time.Time
}
