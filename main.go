package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/SevereCloud/vksdk/v3/api"
	"github.com/SevereCloud/vksdk/v3/events"
	longpoll "github.com/SevereCloud/vksdk/v3/longpoll-bot"
	"github.com/SevereCloud/vksdk/v3/object"
)

func main() {

	conn = setupLog() // глобальная переменная conn
	initDB()

	WriteLog("Приложение работает 🚀", 0, "info")

	vk := api.NewVK(vkToken)

	// Получаем группу
	groupResp, err := vk.GroupsGetByID(nil)
	if err != nil {
		WriteLog(fmt.Sprintf("Ошибка получения группы: %v", err), 0, "error_vk")
		log.Fatal("Ошибка получения группы:", err)
	}
	groupID := groupResp.Groups[0].ID
	log.Printf("Группа доступна 2, ID=%d\n", groupID)

	WriteLog(fmt.Sprintf("Группа доступн 2а, ID=%d", groupID), 0, "info")

	lp, err := longpoll.NewLongPoll(vk, groupID)
	if err != nil {
		WriteLog(fmt.Sprintf("Ошибка создания LongPoll: %v", err), 0, "error_vk")
		log.Fatal("Ошибка создания LongPoll:", err)
	}
	WriteLog("LongPoll создан", 0, "info")

	lp.MessageNew(func(ctx context.Context, obj events.MessageNewObject) {

		peerID := obj.Message.PeerID
		fromID := obj.Message.FromID
		msg, err := json.Marshal(obj.Message)
		if err != nil {
			// обработай ошибку
			WriteLog("json marshal error: "+err.Error(), peerID, "VK")
			return
		}

		WriteLog(string(msg), peerID, "UserFromVK")
		log.Printf("UserFromMK", string(msg))

		userName, err := getUserNickname(vk, fromID)
		if err != nil {
			WriteLog("getUserNickname error: "+err.Error(), peerID, "VK")
			userName = ""
		}

		WriteLog(userName, peerID, "UserFromVK")
		text := obj.Message.Text
		payLoad := obj.Message.Payload
		WriteLog(text, peerID, "VK")

		// гарантируем, что userStates[peerID] всегда есть
		if _, ok := userStates[peerID]; !ok {

			userStates[peerID] = &UserState{PeerID: peerID, Step: "start", RecordResultsStep: 0}

			WriteLog("Создан новый userstate", peerID, "VK_states")
		}

		state := userStates[peerID]

		// Пытаемся найти текущего пользователя в таблице users:
		// сначала по username в vk_username, затем по vk_id.
		WriteLog(userName, peerID, "VK")
		log.Println(userName)

		if fromID != 0 || userName != "" {
			u, err := GetUserByVK(int64(fromID), userName)
			if err != nil {
				WriteLog("GetUserByVK error: "+err.Error(), peerID, "db")
			} else if u != nil {
				state.UserID = u.ID
				state.UserName = u.Username
				WriteLog(fmt.Sprintf("GetUserByVK success: user_id=%d, vk_name=%s", u.ID, userName), peerID, "db")
			} else {
				WriteLog(fmt.Sprintf("GetUserByVK: user not found for vkID=%d, vk_name=%s", fromID, userName), peerID, "db")
			}
		}
		WriteLog(
			fmt.Sprintf("User state updated: step=%s, recordStep=%d", state.Step, state.RecordResultsStep),
			peerID,
			"VK_states",
		)

		type Command struct {
			Command string `json:"command"`
		}
		if text == "начать" || text == "start" || text == "/start" {
			if err == nil {
				sendWelcomeMenu(vk, peerID, state)
			}
			return
		}

		var Result string
		var cmd Command

		if payLoad != "" {
			var inner string

			if err := json.Unmarshal([]byte(payLoad), &inner); err != nil {
				WriteLog(fmt.Sprintf("Ошибка первого парсинга payload: %v, payload=%s", err, payLoad), peerID, "error_vk")
				return
			}

			// второй шаг — теперь распарсим внутренний JSON

			if err := json.Unmarshal([]byte(inner), &cmd); err != nil {
				WriteLog(fmt.Sprintf("Ошибка второго парсинга payload: %v, inner=%s", err, inner), peerID, "error_vk")
				return
			}

		}
		if cmd.Command == "" {
			Result = state.Step
		} else {
			Result = cmd.Command
		}
		switch Result {
		case "results":
			if state.UserID == 0 {
				sendText(vk, peerID, "Не удалось определить тебя в БД, результаты недоступны.")
				break
			}

			games, err := GetUserGames(state.UserID)
			if err != nil {
				WriteLog(fmt.Sprintf("Ошибка получения результатов пользователя: %v", err), peerID, "db")
				sendText(vk, peerID, "Ошибка получения результатов, попробуй позже.")
				break
			}
			if len(games) == 0 {
				sendText(vk, peerID, "📊 У тебя пока нет записанных результатов.")
				break
			}

			var b strings.Builder
			opponentNames := make(map[int]string)
			b.WriteString("📊 Твои последние результаты:\n")
			for i, g := range games {
				// ограничим количество записей, чтобы не заспамить
				if i >= 10 {
					break
				}
				dateStr := g.Datetime.Format("02.01.2006 15:04")

				var myOP, oppOP int
				var opponentID int
				if g.FirstUserID == state.UserID {
					myOP = g.FirstUserResult
					oppOP = g.SecondUserResult
					opponentID = g.SecondUserID
				} else {
					myOP = g.SecondUserResult
					oppOP = g.FirstUserResult
					opponentID = g.FirstUserID
				}

				oppName, ok := opponentNames[opponentID]
				if !ok {
					oppUser, err := GetUserByID(opponentID)
					if err != nil {
						WriteLog(fmt.Sprintf("Ошибка получения оппонента по id=%d: %v", opponentID, err), peerID, "db")
						oppName = "неизвестен"
					} else if oppUser == nil {
						oppName = "неизвестен"
					} else {
						oppName = oppUser.Username
					}
					opponentNames[opponentID] = oppName
				}

				b.WriteString(fmt.Sprintf("%s — ты: %d, оппонент (%s): %d\n", dateStr, myOP, oppName, oppOP))
			}

			sendText(vk, peerID, b.String())
		case "recordResults":
			state.Step = "recordResults"

			recordResults(peerID, text, vk)
		case "find_game":
			sendText(vk, peerID, "🔍 Поиск игр пока в разработке.")
		case "create_game":
			sendText(vk, peerID, "🎮 Создание игры...")
		default:
			sendText(vk, peerID, "Неизвестная команда.")
		}

	})

	log.Println("VK LongPoll бот запущен...")
	WriteLog("VK LongPoll бот запущен", 0, "info")
	if err := lp.Run(); err != nil {
		WriteLog(fmt.Sprintf("Ошибка запуска LongPoll: %v", err), 0, "error_vk")
		log.Fatal("Ошибка запуска LongPoll:", err)
	}
}
func recordResults(peerID int, text string, vk *api.VK) {
	state, exists := userStates[peerID]
	if !exists {
		state = &UserState{PeerID: peerID, Step: "recordResults", RecordResultsStep: 0}
		userStates[peerID] = state
	} //fmt.Println("Текущий шаг:", state.Step, "этап записи:", state.RecordResultsStep)

	keyboard := object.NewMessagesKeyboard(false)
	keyboard.AddRow()
	keyboard.AddTextButton("По договорённости", "", "primary")
	keyboard.AddTextButton("Турнир", "", "secondary")
	keyboardJSON, _ := json.Marshal(keyboard)

	switch state.RecordResultsStep {
	case 0:
		WriteLog("recordResults: step 0, запрос типа мероприятия", peerID, "VK_states")
		vk.MessagesSend(api.Params{
			"peer_id":   peerID,
			"message":   "Введите тип мероприятия",
			"random_id": 0,
			"keyboard":  string(keyboardJSON),
		})
		state.RecordResultsStep = 1
		userStates[peerID] = state

	case 1:
		WriteLog(fmt.Sprintf("recordResults: step 1, ввод типа мероприятия: %s", text), peerID, "VK_states")
		vk.MessagesSend(api.Params{
			"peer_id":   peerID,
			"message":   text,
			"random_id": 0,
		})
		if strings.Contains(strings.ToLower(text), "турнир") {
			state.TypeID = 0
		} else if strings.Contains(strings.ToLower(text), "договор") {
			state.TypeID = 1
		} else {
			WriteLog("recordResults: некорректный тип мероприятия", peerID, "VK_states")
			vk.MessagesSend(api.Params{
				"peer_id":   peerID,
				"message":   "Выбери одну из кнопок 👇",
				"random_id": 0,
				"keyboard":  string(keyboardJSON),
			})
			return
		}

		state.RecordResultsStep = 2
		WriteLog(fmt.Sprintf("recordResults: step 1 завершён, TypeID=%d", state.TypeID), peerID, "VK_states")
		vk.MessagesSend(api.Params{
			"peer_id":   peerID,
			"message":   "Тип события сохранён ✅",
			"random_id": 0,
		})

		usernames, err := GetUsernames()
		if err != nil {
			WriteLog(fmt.Sprintf("Ошибка получения пользователей из БД: %v", err), peerID, "db")
			vk.MessagesSend(api.Params{
				"peer_id":   peerID,
				"message":   "Ошибка получения списка пользователей, попробуй позже.",
				"random_id": 0,
			})
			return
		}

		usersKeyboard := object.NewMessagesKeyboard(false)
		for i, u := range usernames {
			if i%3 == 0 {
				usersKeyboard.AddRow()
			}
			usersKeyboard.AddTextButton(u, "", "secondary")
		}
		usersKbJSON, _ := json.Marshal(usersKeyboard)
		vk.MessagesSend(api.Params{
			"peer_id":   peerID,
			"message":   "Выбери пользователя:",
			"keyboard":  string(usersKbJSON),
			"random_id": 0,
		})

	case 2:
		WriteLog(fmt.Sprintf("recordResults: step 2, выбор пользователя: %s", text), peerID, "VK_states")

		usernames, err := GetUsernames()
		if err != nil {
			WriteLog(fmt.Sprintf("Ошибка получения пользователей из БД: %v", err), peerID, "db")
			vk.MessagesSend(api.Params{
				"peer_id":   peerID,
				"message":   "Ошибка получения списка пользователей, попробуй позже.",
				"random_id": 0,
			})
			return
		}

		found := false
		for _, u := range usernames {
			if strings.EqualFold(text, u) {
				state.Selected = u
				found = true
				break
			}
		}
		if !found {
			vk.MessagesSend(api.Params{
				"peer_id":   peerID,
				"message":   "Выбери пользователя с помощью кнопок 👇",
				"random_id": 0,
			})
			return
		}
		WriteLog(fmt.Sprintf("recordResults: выбран пользователь %s", state.Selected), peerID, "VK_states")
		state.RecordResultsStep = 3
		vk.MessagesSend(api.Params{
			"peer_id":   peerID,
			"message":   fmt.Sprintf("Ты выбрал: %s ✅\nТеперь введи набранные OP:", state.Selected),
			"random_id": 0,
		})

	case 3:
		WriteLog(fmt.Sprintf("recordResults: step 3, ввод TP: %s", text), peerID, "VK_states")
		state.OP = text
		state.RecordResultsStep = 4
		vk.MessagesSend(api.Params{
			"peer_id":   peerID,
			"message":   fmt.Sprintf("TP оппонента: %s ✅", state.OP),
			"random_id": 0,
		})
		vk.MessagesSend(api.Params{
			"peer_id":   peerID,
			"message":   "Введи набранные OP оппонентом",
			"random_id": 0,
		})

	case 4:
		WriteLog(fmt.Sprintf("recordResults: step 4, ввод TP: %s", text), peerID, "VK_states")
		state.OPOpponent = text
		state.RecordResultsStep = 5
		WriteLog("recordResults: шаги завершены", peerID, "VK_states")

		// Сохраняем результат в БД:
		// первый игрок — текущий пользователь (state.UserID),
		// второй игрок — выбранный оппонент (state.Selected).

		if state.UserID == 0 {
			WriteLog("recordResults: UserID в состоянии не установлен, пропускаем запись в БД", peerID, "db")
			vk.MessagesSend(api.Params{
				"peer_id":   peerID,
				"message":   "Не удалось определить тебя в БД, результат не сохранён.",
				"random_id": 0,
			})
			return
		}

		// Ищем оппонента по имени в users (vk_username/username).
		opponent, err := GetUserByUsername(state.Selected)
		if err != nil {
			WriteLog(fmt.Sprintf("recordResults: ошибка поиска оппонента '%s': %v", state.Selected, err), peerID, "db")
			vk.MessagesSend(api.Params{
				"peer_id":   peerID,
				"message":   "Ошибка поиска оппонента в БД, результат не сохранён.",
				"random_id": 0,
			})
			return
		}
		if opponent == nil {
			WriteLog(fmt.Sprintf("recordResults: оппонент '%s' не найден в БД", state.Selected), peerID, "db")
			vk.MessagesSend(api.Params{
				"peer_id":   peerID,
				"message":   "Оппонент не найден в БД, результат не сохранён.",
				"random_id": 0,
			})
			return
		}

		// Парсим результаты (OP) как числа.
		firstResult, err := strconv.Atoi(state.OP)
		if err != nil {
			WriteLog(fmt.Sprintf("recordResults: неверный формат результата игрока: %s", state.OP), peerID, "VK_states")
			vk.MessagesSend(api.Params{
				"peer_id":   peerID,
				"message":   "Неверный формат результата, используй только числа.",
				"random_id": 0,
			})
			return
		}

		secondResult, err := strconv.Atoi(state.OPOpponent)
		if err != nil {
			WriteLog(fmt.Sprintf("recordResults: неверный формат результата оппонента: %s", state.OPOpponent), peerID, "VK_states")
			vk.MessagesSend(api.Params{
				"peer_id":   peerID,
				"message":   "Неверный формат результата оппонента, используй только числа.",
				"random_id": 0,
			})
			return
		}

		// Пока TP и ростер не запрашиваем — сохраняем 0 и пустые строки.
		err = InsertGameResult(
			state.TypeID, // тип события
			time.Now(),   // дата/время
			state.UserID, // первый игрок — текущий пользователь
			opponent.ID,  // второй игрок — выбранный оппонент
			firstResult,  // результат первого игрока
			secondResult, // результат второго игрока
			0,            // TP первого игрока
			0,            // TP второго игрока
			"",           // ростер первого игрока
			"",           // ростер второго игрока
		)
		if err != nil {
			WriteLog(fmt.Sprintf("recordResults: ошибка записи результата в БД: %v", err), peerID, "db")
			vk.MessagesSend(api.Params{
				"peer_id":   peerID,
				"message":   "Ошибка сохранения результата в БД.",
				"random_id": 0,
			})
			return
		}

		vk.MessagesSend(api.Params{
			"peer_id":   peerID,
			"message":   fmt.Sprintf("Результат сохранён ✅\nТвой результат: %d\nРезультат оппонента (%s): %d", firstResult, state.Selected, secondResult),
			"random_id": 0,
		})
	}
}

func sendWelcomeMenu(vk *api.VK, peerID int, state *UserState) {

	keyboard := object.NewMessagesKeyboardInline()
	keyboard.AddRow()
	keyboard.AddTextButton("🏆 Мои результаты", `{"command":"results"}`, "primary")
	keyboard.AddRow()
	keyboard.AddTextButton("✍️ Занести результаты", `{"command":"recordResults"}`, "positive")
	keyboard.AddRow()
	keyboard.AddTextButton("🔍 Найти игру", `{"command":"find_game"}`, "secondary")
	keyboard.AddRow()
	keyboard.AddTextButton("🎮 Создать игру", `{"command":"create_game"}`, "positive")

	_, err := vk.MessagesSend(api.Params{
		"peer_id":   peerID,
		"message":   "👋 Привет! " + state.UserName + " Что хочешь сделать?",
		"keyboard":  keyboard,
		"random_id": 0,
	})
	if err != nil {
		WriteLog(fmt.Sprintf("Ошибка отправки меню: %v", err), peerID, "error_vk")
	}
}

// Отправка простого текста
func sendText(vk *api.VK, peerID int, text string) {
	_, err := vk.MessagesSend(api.Params{
		"peer_id":   peerID,
		"message":   text,
		"random_id": 0,
	})
	if err != nil {
		WriteLog(fmt.Sprintf("Ошибка отправки сообщения: %v", err), peerID, "error_vk")
	}
}
