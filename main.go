// main.go (例)
package main

import (
	"database/sql"
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"os"
	"strconv" // string to int conversion
	"time"

	_ "github.com/go-sql-driver/mysql" // MySQL ドライバーをインポート
)

// Config struct to hold database connection details
type Config struct {
	DBUser     string
	DBPassword string
	DBHost     string
	DBPort     string
	DBName     string
}

// User struct to match CSV and DB table structure (例)
type User struct {
	ID   int
	Name string
	Age  int
}

func main() {
	// データベース設定を環境変数から読み込む
	cfg := Config{
		DBUser:     os.Getenv("MYSQL_USER"),
		DBPassword: os.Getenv("MYSQL_PASSWORD"),
		DBHost:     os.Getenv("MYSQL_HOST"),
		DBPort:     os.Getenv("MYSQL_PORT"),
		DBName:     os.Getenv("MYSQL_DATABASE"),
	}

	// 環境変数が設定されているか確認
	if cfg.DBUser == "" || cfg.DBPassword == "" || cfg.DBHost == "" || cfg.DBPort == "" || cfg.DBName == "" {
		log.Fatal("Database environment variables are not set. Please set MYSQL_USER, MYSQL_PASSWORD, MYSQL_HOST, MYSQL_PORT, MYSQL_DATABASE")
	}

	// データベース接続文字列の構築
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s", cfg.DBUser, cfg.DBPassword, cfg.DBHost, cfg.DBPort, cfg.DBName)
	log.Printf("Connecting to database: %s", dsn)

	// データベースへの接続
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// 接続確認 (リトライロジックを追加すると、DB起動を待てる)
	for i := 0; i < 5; i++ { // 5回リトライ
		err = db.Ping()
		if err == nil {
			log.Println("Successfully connected to the database!")
			break
		}
		log.Printf("Waiting for database to be ready (attempt %d/5): %v", i+1, err)
		time.Sleep(2 * time.Second) // 2秒待機
	}
	if err != nil {
		log.Fatalf("Database did not become ready after retries: %v", err)
	}

	// テーブルが存在しない場合は作成 (Idempotent: 冪等性)
	createTableSQL := `
	CREATE TABLE IF NOT EXISTS users (
		id INT NOT NULL AUTO_INCREMENT,
		name VARCHAR(255) NOT NULL,
		age INT NOT NULL,
		PRIMARY KEY (id)
	);`
	_, err = db.Exec(createTableSQL)
	if err != nil {
		log.Fatalf("Failed to create table: %v", err)
	}
	log.Println("Table 'users' ensured to exist.")

	// CSVファイルからのインポート処理 (コマンドライン引数でファイルパスを受け取る)
	if len(os.Args) > 1 && os.Args[1] == "import" { // "go run main.go import [csv_file_path]" でインポート
		csvFilePath := "data/users.csv" // デフォルトパス
		if len(os.Args) > 2 {
			csvFilePath = os.Args[2] // 引数でCSVパスが指定されたらそれを使う
		}
		log.Printf("Starting CSV import from: %s", csvFilePath)
		err := importCSVToDB(db, csvFilePath)
		if err != nil {
			log.Fatalf("CSV import failed: %v", err)
		}
		log.Println("CSV import completed successfully!")
	} else {
		// 通常のアプリケーションロジック (例: Webサーバー起動など)
		log.Println("Application started. To import CSV, run with 'import' argument.")
		// ここにWebサーバーや他のアプリケーションロジックを記述
		select {} // アプリケーションを終了させないためのダミー
	}
}

func importCSVToDB(db *sql.DB, filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open CSV file: %w", err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1 // 列数が可変の場合にエラーを回避

	// ヘッダー行を読み飛ばす
	_, err = reader.Read()
	if err != nil {
		return fmt.Errorf("failed to read CSV header: %w", err)
	}

	txn, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer txn.Rollback() // エラー発生時にロールバック

	stmt, err := txn.Prepare("INSERT INTO users(name, age) VALUES(?, ?)")
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break // ファイルの終端
		}
		if err != nil {
			return fmt.Errorf("failed to read CSV record: %w", err)
		}

		// CSVの各列を正しい型に変換 (例: name (string), age (int))
		name := record[0] // 1列目が name だと仮定
		age, err := strconv.Atoi(record[1]) // 2列目が age だと仮定
		if err != nil {
			log.Printf("Skipping row due to age conversion error: %v, record: %v", err, record)
			continue // エラーのある行はスキップ
		}

		_, err = stmt.Exec(name, age)
		if err != nil {
			return fmt.Errorf("failed to insert data: %w", err)
		}
	}

	return txn.Commit()
}