package main

import (
	"database/sql"
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"       // 数値変換のため追加
	"time"          // 日付変換のため追加

	_ "github.com/go-sql-driver/mysql" // MySQL ドライバーのインポート
)

const dsn = "user:password@tcp(db:3306)/appdb?parseTime=true"

func main() {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatalf("データベースへの接続に失敗しました: %v", err)
	}
	defer db.Close() // 関数終了時にDB接続を閉じる

	err = db.Ping()
	if err != nil {
		log.Fatalf("データベースへの接続確認 (Ping) に失敗しました: %v", err)
	}
	fmt.Println("データベースに正常に接続しました。")

	// データのインポート
	// /app/data/ にCSVファイルがあることを想定
	err = importTradeHistories(db, "/app/data/trade_history.csv")
	if err != nil {
		log.Fatalf("trade_history.csv のインポートに失敗しました: %v", err)
	}
	fmt.Println("trade_history.csv のインポートが完了しました。")

	err = importReferencePrices(db, "/app/data/reference_prices.csv")
	if err != nil {
		log.Fatalf("reference_prices.csv のインポートに失敗しました: %v", err)
	}
	fmt.Println("reference_prices.csv のインポートが完了しました。")
}

// importTradeHistories は trade_history.csv を読み込み、trade_histories テーブルに挿入します
func importTradeHistories(db *sql.DB, csvFilePath string) error {
	fmt.Printf("trade_histories のインポートを開始: %s\n", csvFilePath)

	file, err := os.Open(csvFilePath)
	if err != nil {
		return fmt.Errorf("CSVファイル '%s' を開けませんでした: %w", csvFilePath, err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1 // レコードごとにフィールド数が異なることを許容
	reader.TrimLeadingSpace = true // フィールドの先頭/末尾の空白をトリム

	// ヘッダー行をスキップ
	_, err = reader.Read()
	if err != nil {
		if err == io.EOF {
			return fmt.Errorf("trade_history.csv が空です")
		}
		return fmt.Errorf("trade_history.csv のヘッダー読み込みに失敗: %w", err)
	}

	tx, err := db.Begin() // トランザクションを開始
	if err != nil {
		return fmt.Errorf("トランザクションの開始に失敗しました: %w", err)
	}
	// エラー発生時にロールバック、成功時にコミット
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			panic(r) // 再パニック
		} else if err != nil {
			tx.Rollback() // エラーがあればロールバック
		} else {
			err = tx.Commit() // エラーがなければコミット
		}
	}()

	stmt, err := tx.Prepare("INSERT INTO trade_histories (user_id, fund_id, quantity, trade_date) VALUES (?, ?, ?, ?)")
	if err != nil {
		return fmt.Errorf("trade_histories のプリペアドステートメント準備に失敗: %w", err)
	}
	defer stmt.Close()

	recordsInserted := 0
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("trade_history.csv のレコード読み込みに失敗: %w", err)
		}

		if len(record) != 4 {
			return fmt.Errorf("trade_history.csv の行の列数が不正です（期待:4, 実際:%d）: %v", len(record), record)
		}

		// データ型の変換
		userID := record[0]
		fundID, err := strconv.Atoi(record[1])
		if err != nil { return fmt.Errorf("trade_history: fund_id '%s' の変換に失敗: %w", record[1], err) }
		quantity, err := strconv.Atoi(record[2])
		if err != nil { return fmt.Errorf("trade_history: quantity '%s' の変換に失敗: %w", record[2], err) }
		
		// 日付形式 "YYYY-MM-DD" を time.Time にパース
		tradeDate, err := time.Parse("2006-01-02", record[3])
		if err != nil { return fmt.Errorf("trade_history: trade_date '%s' のパースに失敗: %w", record[3], err) }

		_, err = stmt.Exec(userID, fundID, quantity, tradeDate)
		if err != nil {
			return fmt.Errorf("trade_histories へのデータ挿入に失敗しました（レコード: %v）: %w", record, err)
		}
		recordsInserted++
	}

	fmt.Printf("trade_histories に %d 件のレコードが挿入されました。\n", recordsInserted)
	return nil
}

// importReferencePrices は reference_prices.csv を読み込み、reference_prices テーブルに挿入します
func importReferencePrices(db *sql.DB, csvFilePath string) error {
	fmt.Printf("reference_prices のインポートを開始: %s\n", csvFilePath)

	file, err := os.Open(csvFilePath)
	if err != nil {
		return fmt.Errorf("CSVファイル '%s' を開けませんでした: %w", csvFilePath, err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1
	reader.TrimLeadingSpace = true

	// ヘッダー行をスキップ
	_, err = reader.Read()
	if err != nil {
		if err == io.EOF {
			return fmt.Errorf("reference_prices.csv が空です")
		}
		return fmt.Errorf("reference_prices.csv のヘッダー読み込みに失敗: %w", err)
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("トランザクションの開始に失敗しました: %w", err)
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			panic(r)
		} else if err != nil {
			tx.Rollback()
		} else {
			err = tx.Commit()
		}
	}()

	stmt, err := tx.Prepare("INSERT INTO reference_prices (fund_id, price, price_date) VALUES (?, ?, ?)")
	if err != nil {
		return fmt.Errorf("reference_prices のプリペアドステートメント準備に失敗: %w", err)
	}
	defer stmt.Close()

	recordsInserted := 0
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("reference_prices.csv のレコード読み込みに失敗: %w", err)
		}

		if len(record) != 3 {
			return fmt.Errorf("reference_prices.csv の行の列数が不正です（期待:3, 実際:%d）: %v", len(record), record)
		}

		// データ型の変換
		fundID, err := strconv.Atoi(record[0])
		if err != nil { return fmt.Errorf("reference_prices: fund_id '%s' の変換に失敗: %w", record[0], err) }
		
		// price は DECIMAL(10,2) なので、Goではstringのまま渡すのが最も安全（精度を保つため）
		price := record[1] 
		
		priceDate, err := time.Parse("2006-01-02", record[2])
		if err != nil { return fmt.Errorf("reference_prices: price_date '%s' のパースに失敗: %w", record[2], err) }

		_, err = stmt.Exec(fundID, price, priceDate)
		if err != nil {
			return fmt.Errorf("reference_prices へのデータ挿入に失敗しました（レコード: %v）: %w", record, err)
		}
		recordsInserted++
	}

	fmt.Printf("reference_prices に %d 件のレコードが挿入されました。\n", recordsInserted)
	return nil
}