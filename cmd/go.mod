module main

go 1.22.0

toolchain go1.24.2

require (
	github.com/integrii/flaggy v1.5.2
	github.com/tadhunt/papertool v0.0.0-00010101000000-000000000000
)

require (
	github.com/tadhunt/logger v0.0.0-20250303180812-6aad7c71b986 // indirect
	golang.org/x/mod v0.23.0 // indirect
	golang.org/x/sync v0.11.0 // indirect
	golang.org/x/text v0.11.0 // indirect
	golang.org/x/tools v0.30.0 // indirect
	gopkg.in/vansante/go-dl-stream.v2 v2.0.1 // indirect
)

replace github.com/tadhunt/papertool => ./..
