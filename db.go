package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	_ "github.com/lib/pq"
)

// dsn по умолчанию, соответствует настройкам в docker-compose.yml:
// POSTGRES_USER: postgres
// POSTGRES_PASSWORD: postgres
// POSTGRES_DB: gamebot
// host: db, port: 5432
const defaultDSN = "postgres://postgres:postgres@db:5432/gamebot?sslmode=disable"

var db *sql.DB

func initDB() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = defaultDSN
		WriteLog("DATABASE_URL не задан, используем dsn по умолчанию", 0, "info_db")
	}

	conn, err := sql.Open("postgres", dsn)
	if err != nil {
		WriteLog(fmt.Sprintf("Ошибка открытия подключения к БД: %v", err), 0, "error_db")
		log.Fatalf("Ошибка открытия подключения к БД: %v", err)
	}
	if err := conn.Ping(); err != nil {
		WriteLog(fmt.Sprintf("Ошибка подключения к БД: %v", err), 0, "error_db")
		log.Fatalf("Ошибка подключения к БД: %v", err)
	}

	db = conn

	if err := ensureSchema(); err != nil {
		WriteLog(fmt.Sprintf("Ошибка инициализации схемы БД: %v", err), 0, "error_db")
		log.Fatalf("Ошибка инициализации схемы БД: %v", err)
	}

	WriteLog("Подключение к БД установлено", 0, "db")
}

// GameResult описывает запись о сыгранной партии.
type GameResult struct {
	ID               int
	GameType         int
	Datetime         time.Time
	FirstUserID      int
	SecondUserID     int
	FirstUserResult  int
	SecondUserResult int
	FirstUserTP      int
	SecondUserTP     int
	FirstUserRoster  string
	SecondUserRoster string
}

// User описывает запись в таблице users.
type User struct {
	ID         int
	Username   string
	VKID       sql.NullInt64
	VKUsername sql.NullString
}

// ensureSchema создаёт необходимые таблицы, если их ещё нет.
func ensureSchema() error {
	_, err := db.Exec(`
CREATE TABLE IF NOT EXISTS users (
    id       SERIAL PRIMARY KEY,
    username TEXT NOT NULL UNIQUE
);
`)
	if err != nil {
		return fmt.Errorf("create users table: %w", err)
	}

	if err := ensureUsersTable(); err != nil {
		return err
	}

	if err := ensureResultsTable(); err != nil {
		return err
	}

	// начальные пользователи
	_, err = db.Exec(`
INSERT INTO users (username) VALUES
  ('rezonov'),
  ('mishka'),
  ('andrew'),
  ('sergey'),
  ('danya')
ON CONFLICT (username) DO NOTHING;
`)
	if err != nil {
		return fmt.Errorf("seed users: %w", err)
	}

	return nil
}

// ensureUsersTable обновляет таблицу users, добавляя поля для VK.
func ensureUsersTable() error {
	if _, err := db.Exec(`ALTER TABLE users ADD COLUMN IF NOT EXISTS vk_id BIGINT`); err != nil {
		return fmt.Errorf("alter users add vk_id: %w", err)
	}

	if _, err := db.Exec(`ALTER TABLE users ADD COLUMN IF NOT EXISTS vk_username TEXT`); err != nil {
		return fmt.Errorf("alter users add vk_username: %w", err)
	}

	return nil
}

// ensureResultsTable создаёт/обновляет таблицу results под нужные поля.
// В таблице храним:
// - тип события (game_type)
// - дату
// - id первого и второго игрока
// - результат (OP) первого и второго игрока
// - полученные TP обоих игроков
// - ростер первого и второго игрока
func ensureResultsTable() error {
	_, err := db.Exec(`
CREATE TABLE IF NOT EXISTS results (
    id               SERIAL PRIMARY KEY,
    datetime         TIMESTAMP NOT NULL,
    first_user_id    INTEGER NOT NULL REFERENCES users(id),
    first_user_tp    INTEGER NOT NULL,
    first_user_op    INTEGER NOT NULL,
    second_user_id   INTEGER NOT NULL REFERENCES users(id),
    second_user_tp   INTEGER NOT NULL,
    second_user_op   INTEGER NOT NULL
);
`)
	if err != nil {
		return fmt.Errorf("create results table: %w", err)
	}

	// Удаляем устаревший столбец type, если он остался от старой схемы.
	if _, err := db.Exec(`ALTER TABLE results DROP COLUMN IF EXISTS type`); err != nil {
		return fmt.Errorf("alter results drop legacy type column: %w", err)
	}

	// дополнительные поля, которые могли отсутствовать в старой схеме
	if _, err := db.Exec(`ALTER TABLE results ADD COLUMN IF NOT EXISTS game_type INTEGER NOT NULL DEFAULT 0`); err != nil {
		return fmt.Errorf("alter results add game_type: %w", err)
	}

	if _, err := db.Exec(`ALTER TABLE results ADD COLUMN IF NOT EXISTS first_user_roster TEXT NOT NULL DEFAULT ''`); err != nil {
		return fmt.Errorf("alter results add first_user_roster: %w", err)
	}

	if _, err := db.Exec(`ALTER TABLE results ADD COLUMN IF NOT EXISTS second_user_roster TEXT NOT NULL DEFAULT ''`); err != nil {
		return fmt.Errorf("alter results add second_user_roster: %w", err)
	}

	return nil
}

func closeDB() {
	if db != nil {
		if err := db.Close(); err != nil {
			WriteLog(fmt.Sprintf("Ошибка закрытия соединения с БД: %v", err), 0, "error_db")
		}
	}
}

// InsertResult записывает результат игры в таблицу results.
func InsertResult(
	dt time.Time,
	firstUserID int,
	firstUserTP int,
	firstUserOP int,
	secondUserID int,
	secondUserTP int,
	secondUserOP int,
) error {
	if db == nil {
		return fmt.Errorf("db не инициализировано")
	}

	if err := ensureSchema(); err != nil {
		WriteLog(fmt.Sprintf("Ошибка инициализации схемы перед записью результата: %v", err), 0, "db")
		return err
	}

	const query = `
INSERT INTO results (
    datetime,
    first_user_id,
    first_user_tp,
    first_user_op,
    second_user_id,
    second_user_tp,
    second_user_op
) VALUES ($1, $2, $3, $4, $5, $6, $7);
`

	if _, err := db.Exec(
		query,
		dt,
		firstUserID,
		firstUserTP,
		firstUserOP,
		secondUserID,
		secondUserTP,
		secondUserOP,
	); err != nil {
		WriteLog(fmt.Sprintf("Ошибка записи результата: %v", err), 0, "error_db")
		return err
	}

	return nil
}

// InsertGameResult записывает расширенный результат игры в таблицу results.
// Параметры соответствуют полям:
//   - тип
//   - дата
//   - id игрока
//   - id второго игрока
//   - результат первого игрока
//   - результат второго игрока
//   - полученные TP первого игрока
//   - полученные TP второго игрока
//   - ростер первого игрока
//   - ростер второго игрока
func InsertGameResult(
	gameType int,
	dt time.Time,
	firstUserID int,
	secondUserID int,
	firstUserResult int,
	secondUserResult int,
	firstUserTP int,
	secondUserTP int,
	firstUserRoster string,
	secondUserRoster string,
) error {
	if db == nil {
		return fmt.Errorf("db не инициализировано")
	}

	if err := ensureSchema(); err != nil {
		WriteLog(fmt.Sprintf("Ошибка инициализации схемы перед записью расширенного результата: %v", err), 0, "db")
		return err
	}

	const query = `
INSERT INTO results (
    game_type,
    datetime,
    first_user_id,
    second_user_id,
    first_user_op,
    second_user_op,
    first_user_tp,
    second_user_tp,
    first_user_roster,
    second_user_roster
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10);
`

	if _, err := db.Exec(
		query,
		gameType,
		dt,
		firstUserID,
		secondUserID,
		firstUserResult,
		secondUserResult,
		firstUserTP,
		secondUserTP,
		firstUserRoster,
		secondUserRoster,
	); err != nil {
		WriteLog(fmt.Sprintf("Ошибка записи расширенного результата: %v", err), 0, "error_db")
		return err
	}

	return nil
}

// GetUsernames возвращает список username из таблицы users.
func GetUsernames() ([]string, error) {
	if db == nil {
		return nil, fmt.Errorf("db не инициализировано")
	}

	// На всякий случай убеждаемся, что схема есть
	if err := ensureSchema(); err != nil {
		WriteLog(fmt.Sprintf("Ошибка инициализации схемы перед выборкой пользователей: %v", err), 0, "db")
		return nil, err
	}

	rows, err := db.Query(`SELECT username FROM users`)
	if err != nil {
		WriteLog(fmt.Sprintf("Ошибка выборки пользователей: %v", err), 0, "db")
		return nil, err
	}
	defer rows.Close()

	var result []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		result = append(result, name)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return result, nil
}

// GetUserGames выбирает все партии пользователя, где он мог быть как первым, так и вторым игроком.
func GetUserGames(userID int) ([]GameResult, error) {
	if db == nil {
		return nil, fmt.Errorf("db не инициализировано")
	}

	if err := ensureSchema(); err != nil {
		WriteLog(fmt.Sprintf("Ошибка инициализации схемы перед выборкой партий: %v", err), 0, "db")
		return nil, err
	}

	const query = `
SELECT
    id,
    game_type,
    datetime,
    first_user_id,
    second_user_id,
    first_user_op,
    second_user_op,
    first_user_tp,
    second_user_tp,
    first_user_roster,
    second_user_roster
FROM results
WHERE first_user_id = $1 OR second_user_id = $1
ORDER BY datetime DESC;
`

	rows, err := db.Query(query, userID)
	if err != nil {
		WriteLog(fmt.Sprintf("Ошибка выборки партий пользователя: %v", err), 0, "error_db")
		return nil, err
	}
	defer rows.Close()

	var games []GameResult
	for rows.Next() {
		var g GameResult
		if err := rows.Scan(
			&g.ID,
			&g.GameType,
			&g.Datetime,
			&g.FirstUserID,
			&g.SecondUserID,
			&g.FirstUserResult,
			&g.SecondUserResult,
			&g.FirstUserTP,
			&g.SecondUserTP,
			&g.FirstUserRoster,
			&g.SecondUserRoster,
		); err != nil {
			return nil, err
		}
		games = append(games, g)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return games, nil
}

// GetUserByID ищет пользователя по его id.
// Возвращает (*User, nil), если найден, (nil, nil), если не найден, и ошибку в остальных случаях.
func GetUserByID(id int) (*User, error) {
	if db == nil {
		return nil, fmt.Errorf("db не инициализировано")
	}

	if err := ensureSchema(); err != nil {
		WriteLog(fmt.Sprintf("Ошибка инициализации схемы перед поиском пользователя по id: %v", err), 0, "db")
		return nil, err
	}

	const query = `
SELECT
    id,
    username,
    vk_id,
    vk_username
FROM users
WHERE id = $1
LIMIT 1;
`

	var u User
	row := db.QueryRow(query, id)
	err := row.Scan(&u.ID, &u.Username, &u.VKID, &u.VKUsername)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		WriteLog(fmt.Sprintf("Ошибка поиска пользователя по id: %v", err), 0, "error_db")
		return nil, err
	}

	return &u, nil
}

// GetUserByUsername ищет пользователя по полю username.
// Возвращает (*User, nil), если найден, (nil, nil), если не найден, и ошибку в остальных случаях.
func GetUserByUsername(username string) (*User, error) {
	if db == nil {
		return nil, fmt.Errorf("db не инициализировано")
	}

	if err := ensureSchema(); err != nil {
		WriteLog(fmt.Sprintf("Ошибка инициализации схемы перед поиском пользователя по username: %v", err), 0, "db")
		return nil, err
	}

	const query = `
SELECT
    id,
    username,
    vk_id,
    vk_username
FROM users
WHERE username = $1
LIMIT 1;
`

	var u User
	row := db.QueryRow(query, username)
	err := row.Scan(&u.ID, &u.Username, &u.VKID, &u.VKUsername)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		WriteLog(fmt.Sprintf("Ошибка поиска пользователя по username: %v", err), 0, "error_db")
		return nil, err
	}

	return &u, nil
}

// GetUserByVK ищет пользователя по vkUsername (vk_username/username), а если не найден — по vkID.
// Сначала пытаемся найти по vkUsername, если оно задано.
// Если пользователь не найден и есть vkID, ищем по vk_id.
// Возвращает (*User, nil), если найден, (nil, nil), если нет, и ошибку в остальных случаях.
func GetUserByVK(vkID int64, vkUsername string) (*User, error) {
	if db == nil {
		return nil, fmt.Errorf("db не инициализировано")
	}

	if err := ensureSchema(); err != nil {
		WriteLog(fmt.Sprintf("Ошибка инициализации схемы перед поиском пользователя по VK: %v", err), 0, "db")
		return nil, err
	}

	const baseSelect = `
SELECT
    id,
    username,
    vk_id,
    vk_username
FROM users
`

	var u User
	if vkUsername != "" {
		// Ищем и в vk_username, и в username — на случай старых записей.
		// Сравниваем без учёта регистра и лишних пробелов.
		query := `
SELECT id, username, vk_id, vk_username
FROM users
WHERE vk_username = $1
LIMIT 1;
`
		row := db.QueryRow(query, vkUsername)
		err := row.Scan(&u.ID, &u.Username, &u.VKID, &u.VKUsername)

		if err != nil && err != sql.ErrNoRows {
			WriteLog(fmt.Sprintf("Ошибка поиска пользователя по vkUsername: %v", err), 0, "error_db")
			return nil, err
		}
		if err == nil {

			return &u, nil
		}
	}

	// Если по имени не нашли и есть vkID — ищем по vk_id.
	if vkID != 0 {
		row := db.QueryRow(baseSelect+`WHERE vk_id = $1 LIMIT 1;`, vkID)
		err := row.Scan(&u.ID, &u.Username, &u.VKID, &u.VKUsername)
		if err == sql.ErrNoRows {
			return nil, nil
		}
		if err != nil {
			WriteLog(fmt.Sprintf("Ошибка поиска пользователя по vkID: %v", err), 0, "error_db")
			return nil, err
		}

		return &u, nil
	}

	return nil, nil
}
