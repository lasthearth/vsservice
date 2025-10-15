package httpdto

type Role struct {
	Id   string `json:"id"`
	Name string `json:"name"`
}

type User struct {
	Id       string `json:"id"`
	Username string `json:"username"`
}
