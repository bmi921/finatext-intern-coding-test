package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"math" // math.Floor のために追加
	"net/http"
	"os"
	"os/signal"
	"sort"   // スライスソートのために追加
	// "strconv" // 文字列と数値の変換のために追加
	"syscall"
	"time"

	_ "github.com/go-sql-driver/mysql" // MySQL ドライバーのインポート
	"github.com/gorilla/mux"           // ルーティングのために追加
)

// --- 定数 ---
const (
	UNIT_PER_PRICE_BASE = 10000.0 // 基準価額あたりの口数 (計算のためにfloat64)
	DB_RETRY_ATTEMPTS   = 10      // DB接続リトライ回数
	DB_RETRY_INTERVAL   = 2 * time.Second // DB接続リトライ間隔
)

// --- 設定構造体 ---
type Config struct {
	DBUser     string
	DBPassword string
	DBHost     string
	DBPort     string
	DBName     string
}

// --- グローバルなDB接続変数 ---
var db *sql.DB

// --- データ構造体 (内部使用) ---
// TradeHistory はAPIからは直接使われないが、DBからの取得やロジックで利用する
type TradeHistory struct {
	UserID    string
	FundID    int
	Quantity  int
	TradeDate time.Time
}

// ReferencePrice も同様
type ReferencePrice struct {
	FundID    int
	Price     float64
	PriceDate time.Time
}

// Position はユーザーの特定のファンドの保有状況を表す
type Position struct {
	FundID        int
	TotalQuantity int       // 総保有口数
	TotalBuyCost  float64   // 総買付金額 (正確な計算のためfloat64)
	TradeDate     time.Time // 取引日（年ごとの集計で使用）
}

// --- APIレスポンス構造体 ---

// TradesResponse はStep 3のレスポンス
type TradesResponse struct {
	Count int `json:"count"`
}

// AssetData はStep 4, 5, 6の資産評価額と評価損益のレスポンス
type AssetData struct {
	Date        string `json:"date"`
	CurrentValue int64 `json:"current_value"` // 整数に切り捨て
	CurrentPL    int64 `json:"current_pl"`    // 整数に切り捨て
}

// AssetsByYearResponse はStep 6の買付年ごとの評価額・評価損益のレスポンス
type AssetsByYearResponse struct {
	Date   string        `json:"date"`
	Assets []YearlyAsset `json:"assets"`
}

// YearlyAsset はStep 6の年ごとの資産評価額・評価損益の詳細
type YearlyAsset struct {
	Year        int   `json:"year"`
	CurrentValue int64 `json:"current_value"`
	CurrentPL    int64 `json:"current_pl"`
}

// --- メイン関数 ---
func main() {
	// --- データベース接続設定 ---
	cfg := Config{
		DBUser:     os.Getenv("DB_USER"),
		DBPassword: os.Getenv("DB_PASSWORD"),
		DBHost:     os.Getenv("DB_HOST"),
		DBPort:     os.Getenv("DB_PORT"),
		DBName:     os.Getenv("DB_NAME"),
	}

	if cfg.DBUser == "" || cfg.DBPassword == "" || cfg.DBHost == "" || cfg.DBPort == "" || cfg.DBName == "" {
		log.Fatal("環境変数の読み込みに失敗しました: DB_USER, DB_PASSWORD, DB_HOST, DB_PORT, DB_NAME が設定されている必要があります。")
	}

	// parseTime=true は MySQL ドライバーで time.Time 型を正しく扱うために重要
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true", cfg.DBUser, cfg.DBPassword, cfg.DBHost, cfg.DBPort, cfg.DBName)
	log.Printf("データベースに接続を試行中: %s", cfg.DBHost)

	var err error
	db, err = sql.Open("mysql", dsn)
	if err != nil {
		log.Fatalf("データベース接続のオープンに失敗しました: %v", err)
	}
	defer db.Close() // 関数終了時にDB接続を閉じる

	// データベース接続のリトライロジック
	for i := 0; i < DB_RETRY_ATTEMPTS; i++ {
		err = db.Ping()
		if err == nil {
			log.Println("データベースに正常に接続しました！")
			break
		}
		log.Printf("データベースの準備を待機中 (試行 %d/%d): %v", i+1, DB_RETRY_ATTEMPTS, err)
		time.Sleep(DB_RETRY_INTERVAL)
	}
	if err != nil {
		log.Fatalf("リトライ後もデータベースが準備できませんでした: %v", err)
	}

	// --- データベーステーブルの初期化 ---
	// CSVインポートをしない場合でも、テーブル構造は必要なのでこの処理は残します。
	log.Println("データベーステーブルが存在することを確認しています...")
	err = setupDatabaseTables(db)
	if err != nil {
		log.Fatalf("データベーステーブルの設定に失敗しました: %v", err)
	}
	log.Println("データベーステーブルは準備完了です。")

	// --- APIサーバー設定 ---
	router := mux.NewRouter()

	// 基本的なヘルスチェック
	router.HandleFunc("/hello", helloHandler).Methods("GET")

	// Step 3: ユーザーの取引回数を取得
	router.HandleFunc("/{user_id}/trades", getTradesCountHandler).Methods("GET")

	// Step 4 & 5: ユーザーの資産評価額と評価損益を取得 (オプションの日付パラメータあり)
	router.HandleFunc("/{user_id}/assets", getAssetsHandler).Methods("GET")

	// Step 6: ユーザーの資産評価額と評価損益を年ごとに取得
	router.HandleFunc("/{user_id}/assets/byYear", getAssetsByYearHandler).Methods("GET")

	// HTTPサーバーを起動
	port := "8080"
	fmt.Printf("APIサーバー :https://localhost:%s で起動中\n", port)

	// サーバーを起動し、エラーがあればログに出力して終了
	go func() {
		log.Fatal(http.ListenAndServe(":"+port, router))
	}()

	// --- コンテナを起動し続けるための処理 ---
	fmt.Println("APIサーバーが起動しました。終了シグナルを待機中...")
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM) // Ctrl+C や docker stop を捕捉
	<-sigs                                               // シグナルが来るまでブロック
	fmt.Println("終了シグナルを受信しました。アプリケーションを終了します。")
	fmt.Println("Application exiting.")
}

// --- ヘルパー関数: データベーステーブルのセットアップ ---
// CSVインポートが行われない場合でも、APIがDBを参照するためにテーブルは必要なので残します。
func setupDatabaseTables(db *sql.DB) error {
	createTradeHistoriesSQL := `
	CREATE TABLE IF NOT EXISTS trade_histories (
		user_id VARCHAR(255) NOT NULL,
		fund_id INT NOT NULL,
		quantity INT NOT NULL,
		trade_date DATE NOT NULL,
		PRIMARY KEY (user_id, fund_id, trade_date)
	);`

	createReferencePricesSQL := `
	CREATE TABLE IF NOT EXISTS reference_prices (
		fund_id INT NOT NULL,
		price DECIMAL(10, 2) NOT NULL,
		price_date DATE NOT NULL,
		PRIMARY KEY (fund_id, price_date)
	);`

	_, err := db.Exec(createTradeHistoriesSQL)
	if err != nil {
		return fmt.Errorf("trade_histories テーブルの作成に失敗しました: %w", err)
	}
	log.Println("trade_histories テーブルは作成済み、または既に存在します。")

	_, err = db.Exec(createReferencePricesSQL)
	if err != nil {
		return fmt.Errorf("reference_prices テーブルの作成に失敗しました: %w", err)
	}
	log.Println("reference_prices テーブルは作成済み、または既に存在します。")
	return nil
}

// --- APIハンドラ ---

// helloHandler: 基本的なヘルスチェック
func helloHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "Hello from Go API!"})
}

// getTradesCountHandler: Step 3 - 特定のuser_idの取引回数を取得
func getTradesCountHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	userID := vars["user_id"]

	var count int
	// user_idごとのtrade_dateのユニークな数を数える
	// もし「取引を行った回数」が `trade_histories` テーブルの行数と等しいなら COUNT(*) でOK
	// 厳密に「取引を行った日」のユニーク数を数えるなら DISTINCT trade_date を使う
	query := "SELECT COUNT(*) FROM trade_histories WHERE user_id = ?"
	err := db.QueryRow(query, userID).Scan(&count)
	if err != nil {
		http.Error(w, fmt.Sprintf("取引回数の取得に失敗しました: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(TradesResponse{Count: count})
}

// getAssetsHandler: Step 4 & 5 - ユーザーの資産評価額と評価損益を取得 (オプションの日付パラメータあり)
func getAssetsHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	userID := vars["user_id"]
	dateStr := r.URL.Query().Get("date") // クエリパラメータからdateを取得

	var targetDate time.Time
	if dateStr != "" {
		// 指定された日付を使用
		parsedDate, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			http.Error(w, "日付フォーマットが不正です。YYYY-MM-DD 形式を使用してください。", http.StatusBadRequest)
			return
		}
		targetDate = parsedDate
	} else {
		// 日付が指定されていない場合は現在の日付を使用
		// Goのtime.Now()はタイムゾーン情報を持つため、DBのDATE型に合わせるために日付部分のみにする
		targetDate = time.Now().In(time.Local) // 日本のタイムゾーンで現在時刻を取得
		targetDate = time.Date(targetDate.Year(), targetDate.Month(), targetDate.Day(), 0, 0, 0, 0, time.Local)
	}

	// 資産評価額と買付金額の合計を計算するためのSQLクエリ
	// 各ファンドIDごとの最終的な保有口数と、その口数に対する買付金額の合計を算出
	// 指定された日付以前の取引のみを考慮する
	rows, err := db.Query(`
		SELECT
			th.fund_id,
			SUM(th.quantity) AS total_quantity,
			-- 買付金額の合計: (買付時の基準価額 / 基準価額あたりの口数 * 買付口数) を合計
			-- MySQLのDECIMAL型はそのままfloat64にスキャンされる
			SUM(th.quantity * rp_buy.price / ?) AS total_buy_cost
		FROM
			trade_histories th
		JOIN
			reference_prices rp_buy ON th.fund_id = rp_buy.fund_id AND th.trade_date = rp_buy.price_date
		WHERE
			th.user_id = ? AND th.trade_date <= ?
		GROUP BY
			th.fund_id
		HAVING
			total_quantity > 0;
	`, UNIT_PER_PRICE_BASE, userID, targetDate.Format("2006-01-02")) // DATE型に合わせるためフォーマット
	if err != nil {
		log.Printf("ユーザー %s のポジション取得中にエラーが発生しました（日付 %s）: %v", userID, targetDate.Format("2006-01-02"), err)
		http.Error(w, "資産データの取得に失敗しました。", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	// 各ファンドIDごとの保有状況と買付金額を格納
	positions := make(map[int]Position)
	for rows.Next() {
		var fundID int
		var totalQuantity int
		var totalBuyCost float64
		err := rows.Scan(&fundID, &totalQuantity, &totalBuyCost)
		if err != nil {
			log.Printf("ポジション行のスキャン中にエラーが発生しました: %v", err)
			continue
		}
		positions[fundID] = Position{
			FundID:        fundID,
			TotalQuantity: totalQuantity,
			TotalBuyCost:  totalBuyCost,
		}
	}
	if rows.Err() != nil {
		log.Printf("行のイテレーション中にエラーが発生しました: %v", rows.Err())
	}

	var totalCurrentValue float64 = 0
	var totalBuyAmount float64 = 0

	for _, pos := range positions {
		// 基準価額（評価日時点の最新の基準価額）を取得
		// 指定日以前で最も新しい price_date を持つレコードを取得
		var currentPrice float64
		err := db.QueryRow(`
			SELECT price FROM reference_prices
			WHERE fund_id = ? AND price_date <= ?
			ORDER BY price_date DESC
			LIMIT 1
		`, pos.FundID, targetDate.Format("2006-01-02")).Scan(&currentPrice)
		if err == sql.ErrNoRows {
			// そのファンドIDの基準価額が指定日以前で見つからない場合、その銘柄は評価対象外
			log.Printf("ファンドID %d の参照価格が %s 以前で見つかりません。計算をスキップします。", pos.FundID, targetDate.Format("2006-01-02"))
			continue
		}
		if err != nil {
			log.Printf("ファンドID %d の現在価格の取得中にエラーが発生しました: %v", pos.FundID, err)
			continue // エラーが発生した場合はその銘柄の計算をスキップ
		}

		// 資産評価額: (基準価額 * 所持口数) / 基準価額あたりの口数
		currentValue := (currentPrice * float64(pos.TotalQuantity)) / UNIT_PER_PRICE_BASE
		totalCurrentValue += currentValue

		// 買付金額の合計は Position の TotalBuyCost をそのまま使う
		totalBuyAmount += pos.TotalBuyCost
	}

	// 整数に切り捨て
	finalCurrentValue := int64(math.Floor(totalCurrentValue))
	finalCurrentPL := int64(math.Floor(totalCurrentValue - totalBuyAmount))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(AssetData{
		Date:        targetDate.Format("2006-01-02"),
		CurrentValue: finalCurrentValue,
		CurrentPL:    finalCurrentPL,
	})
}

// getAssetsByYearHandler: Step 6 - ユーザーの資産評価額・評価損益を年ごとに取得
func getAssetsByYearHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	userID := vars["user_id"]
	
	// 現在の日付を取得（基準価額の取得に使用）
	currentDate := time.Now().In(time.Local)
	currentDate = time.Date(currentDate.Year(), currentDate.Month(), currentDate.Day(), 0, 0, 0, 0, time.Local)
	currentDateStr := currentDate.Format("2006-01-02")

	// 買付年、ファンドIDごとの総保有口数と総買付金額を取得
	// current_value, current_pl の計算は Go側で行うため、買付時の情報のみ取得
	rows, err := db.Query(`
		SELECT
			YEAR(th.trade_date) AS trade_year,
			th.fund_id,
			SUM(th.quantity) AS total_quantity,
			SUM(th.quantity * rp_buy.price / ?) AS total_buy_cost
		FROM
			trade_histories th
		JOIN
			reference_prices rp_buy ON th.fund_id = rp_buy.fund_id AND th.trade_date = rp_buy.price_date
		WHERE
			th.user_id = ? AND th.trade_date <= ? -- 現在時刻までの取引を対象
		GROUP BY
			trade_year, th.fund_id
		HAVING
			total_quantity > 0; -- 1口以上の残高をもつ銘柄
	`, UNIT_PER_PRICE_BASE, userID, currentDateStr)
	if err != nil {
		log.Printf("ユーザー %s の年別資産取得中にエラーが発生しました: %v", userID, err)
		http.Error(w, "年別資産データの取得に失敗しました。", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	// 年ごとの集計マップ
	// Key: 年 (int), Value: その年の合計評価額と合計買付金額
	// さらに、その年に購入したファンドごとの保有口数と買付コストを保持する
	type yearlyFundData struct {
		CurrentValueSum float64
		BuyAmountSum    float64
	}
	yearlySummary := make(map[int]yearlyFundData)
    
	// ファンドごとの現在価格のキャッシュ (複数回クエリを打つのを避けるため)
	priceCache := make(map[int]float64)

	for rows.Next() {
		var tradeYear int
		var fundID int
		var totalQuantity int
		var totalBuyCost float64
		err := rows.Scan(&tradeYear, &fundID, &totalQuantity, &totalBuyCost)
		if err != nil {
			log.Printf("年別資産行のスキャン中にエラーが発生しました: %v", err)
			continue
		}

		// 現在時刻の基準価額を取得 (キャッシュ利用)
		currentPrice, ok := priceCache[fundID]
		if !ok {
			err := db.QueryRow(`
				SELECT price FROM reference_prices
				WHERE fund_id = ? AND price_date <= ?
				ORDER BY price_date DESC
				LIMIT 1
			`, fundID, currentDateStr).Scan(&currentPrice)

			if err == sql.ErrNoRows {
				log.Printf("ファンドID %d の現在参照価格が %s 以前で見つかりません。年別計算をスキップします。", fundID, currentDateStr)
				continue
			}
			if err != nil {
				log.Printf("ファンドID %d の現在価格の取得中にエラーが発生しました (年別資産): %v", fundID, err)
				continue
			}
			priceCache[fundID] = currentPrice // キャッシュに保存
		}

		// 資産評価額 (その買付年の口数のみで計算)
		currentValueForFund := (currentPrice * float64(totalQuantity)) / UNIT_PER_PRICE_BASE

		// マップの値を更新
		data := yearlySummary[tradeYear]
		data.CurrentValueSum += currentValueForFund
		data.BuyAmountSum += totalBuyCost
		yearlySummary[tradeYear] = data
	}
	if rows.Err() != nil {
		log.Printf("年別資産の行イテレーション中にエラーが発生しました: %v", rows.Err())
	}

	// 結果をAssetsByYearResponseの形式に変換
	var yearlyAssets []YearlyAsset
	for year, data := range yearlySummary {
		yearlyAssets = append(yearlyAssets, YearlyAsset{
			Year:        year,
			CurrentValue: int64(math.Floor(data.CurrentValueSum)),
			CurrentPL:    int64(math.Floor(data.CurrentValueSum - data.BuyAmountSum)),
		})
	}

	// Yearの降順でソート
	sort.Slice(yearlyAssets, func(i, j int) bool {
		return yearlyAssets[i].Year > yearlyAssets[j].Year
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(AssetsByYearResponse{
		Date:   currentDateStr,
		Assets: yearlyAssets,
	})
}