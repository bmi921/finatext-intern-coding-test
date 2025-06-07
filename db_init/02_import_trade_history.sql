-- IMPORTANT: このパスはDBコンテナ内のパスです
LOAD DATA INFILE '/app/data/trade_history.csv'
INTO TABLE trade_histories
FIELDS TERMINATED BY ','
LINES TERMINATED BY '\n'
IGNORE 1 LINES -- ヘッダー行をスキップ
(user_id, fund_id, quantity, trade_date);