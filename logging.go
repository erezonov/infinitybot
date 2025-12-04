package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"time"
)

// conn — глобальное TCP‑подключение к Logstash.
var conn net.Conn

// setupLog устанавливает соединение с Logstash, с ретраями.
func setupLog() net.Conn {
	host := os.Getenv("LOGSTASH_HOST")
	if host == "" {
		log.Println("LOGSTASH_HOST not set, running without Logstash")
		return nil
	}
	var (
		c   net.Conn
		err error
	)
	for i := 0; i < 10; i++ {
		c, err = net.Dial("tcp", host)
		if err == nil {
			break
		}
		WriteLog(fmt.Sprintf("Logstash not ready (%v), retrying in 5s...", err), 0, "error_logstash")
		time.Sleep(5 * time.Second)
	}
	if err != nil {
		log.Printf("Cannot connect to Logstash after retries: %v; continuing without it", err)
		return nil
	}
	return c
}

// WriteLog пишет JSON‑лог в Logstash.
// level по умолчанию "VK".
func WriteLog(message string, user int, levelOpt ...string) {
	level := "VK"
	if len(levelOpt) > 0 {
		level = levelOpt[0]
	}

	msg := map[string]interface{}{
		"timestamp": time.Now().Format(time.RFC3339),
		"level":     level,
		"message":   message,
	}
	if user != 0 {
		msg["user"] = user
	}

	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("Ошибка сериализации лога: %v", err)
		return
	}
	if conn != nil {
		fmt.Fprintln(conn, string(data))
	} else {
		// резервный вывод, если подключения к Logstash ещё нет
		log.Printf("LOG(no conn): %s", string(data))
	}
}
