-- IMPORTANT: このパスはDBコンテナ内のパスです
LOAD DATA INFILE '/app/data/reference_prices.csv'
INTO TABLE reference_prices
FIELDS TERMINATED BY ','
LINES TERMINATED BY '\n'
IGNORE 1 LINES -- ヘッダー行をスキップ
(fund_id, price, price_date);