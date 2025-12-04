package main

// UserState хранит состояние пользователя в боте.
type UserState struct {
	PeerID            int
	UserID            int
	Step              string
	RecordResultsStep int
	TypeID            int
	Selected          string
	OP                string
	OPOpponent        string
	UserName          string
}

// userStates — карта состояний пользователей.
var userStates = make(map[int]*UserState)
