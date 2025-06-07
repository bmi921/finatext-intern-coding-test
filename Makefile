dev/run/import:
	docker-compose build --no-cache db
	docker-compose build --no-cache app
	docker-compose up -d db
	docker-compose up -d app
	@sleep 5
	docker exec -it app sh -c "go build -o db_init /app/db_init.go"
	@sleep 5
	docker exec -it app sh -c "ls"
	docker exec -it app sh -c "/app/db_init"