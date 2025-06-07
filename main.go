package main

import (
	"fmt"
	"os"
	"os/signal" 
    "syscall" 
	_ "github.com/go-sql-driver/mysql"
)

func main() {

	// --- コンテナを起動し続けるための処理 ---
	fmt.Println("appコンテナが起動しました。(Ctrl+C)で終了します...")
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM) // Ctrl+C や docker stop を捕捉
	<-sigs                                               
	fmt.Println("終了シグナルを受信しました。アプリケーションを終了します。")
	fmt.Println("Application exiting.")
}
