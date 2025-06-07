# 🏦 finatext-intern-coding-test
finatext株式会社のサマーインターンの[選考課題](https://finatextgroup.kibe.la/shared/entries/fbebacbe-ab1b-442f-90f5-9573f6ab8a7f)になります。

## 🚀 Getting started
`docker`を起動して、`docker --version`で使えることを確認してください。
以下のコマンドを複数のターミナルで順に実行してください。

```bash
# リポジトリのコピー
git clone https://github.com/bmi921/finatext-intern-coding-test
cd ./finatext-intern-coding-test

# apiとdb起動
make dev/run 

# csvをdbにインポートする
make dev/run/import:

# api鯖を起動する
make dev/run/server

```
## ✅ 概要
docker-composeで`app`と`db`の2つのサービスを立ち上げています。  
`app`はapiサーバーでGo言語で仕様に則って、httpリクエストを返します。
[http://localhost:8080](http://localhost:8080)で立ち上がります。  
`db`はMySQLで2つのテーブルデータを持っています。計7時間ほどで開発しました。  
