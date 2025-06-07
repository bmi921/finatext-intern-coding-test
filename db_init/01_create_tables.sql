CREATE TABLE IF NOT EXISTS trade_histories (
    user_id VARCHAR(255) NOT NULL,
    fund_id INT NOT NULL,
    quantity INT NOT NULL,
    trade_date DATE NOT NULL,
    PRIMARY KEY (user_id, fund_id, trade_date)
);

CREATE TABLE IF NOT EXISTS reference_prices (
    fund_id INT NOT NULL,
    price DECIMAL(10, 2) NOT NULL,
    price_date DATE NOT NULL,
    PRIMARY KEY (fund_id, price_date)
);

-- IMPORTANT: このパスはDBコンテナ内のパスです
LOAD DATA INFILE '/app/data/trade_history.csv'
INTO TABLE trade_histories
FIELDS TERMINATED BY ','
LINES TERMINATED BY '\n'
IGNORE 1 LINES -- ヘッダー行をスキップ
(user_id, fund_id, quantity, trade_date);

-- IMPORTANT: このパスはDBコンテナ内のパスです
LOAD DATA INFILE '/app/data/reference_prices.csv'
INTO TABLE reference_prices
FIELDS TERMINATED BY ','
LINES TERMINATED BY '\n'
IGNORE 1 LINES -- ヘッダー行をスキップ
(fund_id, price, price_date);