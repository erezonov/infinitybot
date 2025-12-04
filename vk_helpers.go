package main

import (
	"fmt"

	"github.com/SevereCloud/vksdk/v3/api"
)

// getUserNickname получает "ник" пользователя ВК по его ID (peerID для лички).
// Возвращает строку вида "Имя Фамилия" или screen_name, если он есть.
func getUserNickname(vk *api.VK, userID int) (string, error) {
	users, err := vk.UsersGet(api.Params{
		"user_ids": []int{userID},
		"fields":   "screen_name",
	})
	if err != nil {
		return "", fmt.Errorf("vk.UsersGet error: %w", err)
	}
	if len(users) == 0 {
		return "", fmt.Errorf("user not found for id=%d", userID)
	}

	u := users[0]

	if u.ScreenName != "" {
		return u.ScreenName, nil
	}

	name := u.FirstName
	if u.LastName != "" {
		name += " " + u.LastName
	}

	return name, nil
}
