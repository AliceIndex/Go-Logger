package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	_ "github.com/lib/pq"
)

// Response : 書き込み完了時のメッセージ用
type Response struct {
	Message  string `json:"message"`
	DBStatus string `json:"db_status"`
}

// LogEntry : 読み出し用（DBのテーブル構造に合わせる）
type LogEntry struct {
	ID        int       `json:"id"`
	UserAgent string    `json:"user_agent"`
	CreatedAt time.Time `json:"created_at"`
}

var db *sql.DB

func main() {
	// ==========================================
	// 1. データベース接続設定
	// ==========================================
	connStr := fmt.Sprintf("host=%s user=%s password=%s dbname=%s sslmode=disable",
		os.Getenv("DB_HOST"), os.Getenv("DB_USER"), os.Getenv("DB_PASSWORD"), os.Getenv("DB_NAME"))

	var err error
	// DBが起動するまでリトライする（最大10回 / 20秒待機）
	for i := 0; i < 10; i++ {
		fmt.Println("Connecting to database...")
		db, err = sql.Open("postgres", connStr)
		if err == nil {
			if err = db.Ping(); err == nil {
				fmt.Println("Success: Connected to Database!")
				break
			}
		}
		fmt.Printf("Waiting for database... (Attempt %d/10)\n", i+1)
		time.Sleep(2 * time.Second)
	}

	if err != nil {
		log.Fatal("Failed to connect to database after retries:", err)
	}

	// ==========================================
	// 2. テーブル作成（初回のみ）
	// ==========================================
	createTableSQL := `
	CREATE TABLE IF NOT EXISTS access_logs (
		id SERIAL PRIMARY KEY,
		user_agent TEXT,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);`
	if _, err := db.Exec(createTableSQL); err != nil {
		log.Fatal("Failed to create table:", err)
	}

	// ==========================================
	// 3. ルーティング設定
	// ==========================================
	
	// A. ログ書き込み用API (curlなどでアクセスすると記録＆通知)
	// 例: https://dev.aliceindex.jp/go/api/
	http.HandleFunc("/api/", writeHandler)

	// B. ログ読み出し用API (JSからfetchしてデータを取得)
	// 例: https://dev.aliceindex.jp/go/api/logs
	http.HandleFunc("/api/logs", readHandler)

	// C. ダッシュボード画面 (staticフォルダ内のHTMLを配信)
	// 例: https://dev.aliceindex.jp/go/
	fs := http.FileServer(http.Dir("./static"))
	http.Handle("/", fs)

	// サーバー起動
	fmt.Println("Server starting on port 8081...")
	log.Fatal(http.ListenAndServe(":8081", nil))
}

// ==========================================
// ハンドラ関数定義
// ==========================================

// writeHandler : アクセスをDBに保存し、Discordに通知を送る
func writeHandler(w http.ResponseWriter, r *http.Request) {
	// 1. DBへの書き込み (INSERT)
	_, err := db.Exec("INSERT INTO access_logs (user_agent) VALUES ($1)", r.UserAgent())
	
	status := "OK"
	if err != nil {
		status = "Error: " + err.Error()
		fmt.Println("DB Insert Error:", err)
	} else {
		// 2. 成功したら非同期でDiscordへ通知
		go sendDiscordNotification("🚀 New Access Detected! UA: " + r.UserAgent())
	}

	// 3. クライアントへJSONレスポンス
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(Response{
		Message:  "Logged successfully!",
		DBStatus: status,
	})
}

// readHandler : 保存されたログをDBから取得して返す
func readHandler(w http.ResponseWriter, r *http.Request) {
	// 1. DBからデータ取得 (SELECT) 最新50件
	rows, err := db.Query("SELECT id, user_agent, created_at FROM access_logs ORDER BY id DESC LIMIT 50")
	if err != nil {
		http.Error(w, "Database error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	// 2. 構造体のリストに変換
	var logs []LogEntry
	for rows.Next() {
		var l LogEntry
		if err := rows.Scan(&l.ID, &l.UserAgent, &l.CreatedAt); err != nil {
			continue
		}
		logs = append(logs, l)
	}

	// 3. JSONとして返す
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(logs)
}

// sendDiscordNotification : Discord WebhookにPOSTリクエストを送る
func sendDiscordNotification(message string) {
	url := os.Getenv("DISCORD_WEBHOOK_URL")
	if url == "" {
		return // URL設定がなければ何もしない
	}

	// Discord用JSON作成
	jsonBody := []byte(fmt.Sprintf(`{"content": "%s"}`, message))
	
	// HTTPリクエスト作成
	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")

	// 送信
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("Failed to send Discord notification:", err)
		return
	}
	defer resp.Body.Close()
}