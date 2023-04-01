package user

import (
	"encoding/json"
	"time"
)

type User struct {
	ID        string
	FirstName string
	Details   interface{}
	BirthDate time.Time
}

func (u *User) Serialize() []byte {
	bytes, err := json.Marshal(u)
	if err != nil {
		panic(err)
	}
	return bytes
}
