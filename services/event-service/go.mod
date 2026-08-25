module github.com/alsung/event-ticketing-system/services/event-service

go 1.25.0

replace github.com/alsung/event-ticketing-system/services/pkg => ../pkg

require (
	github.com/alsung/event-ticketing-system/services/pkg v0.0.0-00010101000000-000000000000
	github.com/jackc/pgx/v5 v5.10.0
	github.com/joho/godotenv v1.5.1
)

require (
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/text v0.41.0 // indirect
)
