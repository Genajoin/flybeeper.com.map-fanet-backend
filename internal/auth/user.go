package auth

import (
	"encoding/json"
	"time"
)

// User представляет пользователя из Laravel API
type User struct {
	ID              int                    `json:"id"`
	Name            string                 `json:"name"`
	Email           string                 `json:"email"`
	EmailVerifiedAt *time.Time            `json:"email_verified_at"`
	Role            string                 `json:"role"`
	Settings        map[string]interface{} `json:"settings"`
}

// ToJSON сериализует пользователя в JSON для кеширования
func (u *User) ToJSON() ([]byte, error) {
	return json.Marshal(u)
}

// FromJSON десериализует пользователя из JSON
func UserFromJSON(data []byte) (*User, error) {
	var user User
	err := json.Unmarshal(data, &user)
	return &user, err
}

// IsEmailVerified проверяет, подтвержден ли email пользователя
func (u *User) IsEmailVerified() bool {
	return u.EmailVerifiedAt != nil
}

// IsAdmin проверяет, является ли пользователь администратором
func (u *User) IsAdmin() bool {
	return u.Role == "admin"
}

// GetSetting возвращает значение настройки пользователя
func (u *User) GetSetting(key string) interface{} {
	if u.Settings == nil {
		return nil
	}
	return u.Settings[key]
}