dev/run:
	docker-compose down -v
	docker-compose up --build

dev/run/import:
	docker exec -it app sh -c "go run /app/db_init.go"

dev/run/server:
	docker exec -it app sh -c "go run /app/server.go"
